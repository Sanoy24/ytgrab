// Package captions makes subtitles for downloads that have none, on this computer, with
// whisper.cpp: no audio leaves it. whisper.cpp and its speech model are installed on
// request into YTGrab's data folder (on macOS, whisper comes from Homebrew), each checked
// against a checksum pinned here. Captions are made one at a time, in the background, and
// saved beside the download as "<name>.whisper.srt", which players load and the Library
// search indexes.
package captions

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/Sanoy24/ytgrab/internal/domain"
	"github.com/Sanoy24/ytgrab/internal/library"
	"github.com/Sanoy24/ytgrab/internal/process"
)

// Captions are saved as "<name>.whisper-<language>.srt", like "Talk [id].whisper-es.srt";
// English translations as "<name>.whisper-en.srt".
const Suffix = ".whisper"

// Translated marks the state of a download's English translation: "<job id>/en".
const Translated = "/en"

// detectedLanguage is how whisper reports the language it heard.
var detectedLanguage = regexp.MustCompile(`auto-detected language: ([a-z]{2,3})\b`)

// Job states as the page shows them.
const (
	Queued = "queued"
	Making = "making"
	Done   = "done"
	Failed = "failed"
)

// JobState is one download's captions.
type JobState struct {
	State string `json:"state"`
	Error string `json:"error,omitempty"`
}

// Status is what the settings page and the Library show.
type Status struct {
	Supported  bool                `json:"supported"`            // this system can make captions
	Installed  bool                `json:"installed"`            // whisper and the model are here
	Installing bool                `json:"installing,omitempty"` // a download is running
	Progress   float64             `json:"progress,omitempty"`   // 0–1 while installing
	Error      string              `json:"error,omitempty"`      // why the last install failed
	HowTo      string              `json:"how_to,omitempty"`     // what to install by hand (macOS)
	Size       string              `json:"size"`                 // the download, for the button
	Jobs       map[string]JobState `json:"jobs"`
}

// Service installs whisper and makes captions.
type Service struct {
	dir    string // <data>/captions
	ffmpeg func() (string, error)
	job    func(context.Context, string) (domain.Job, error)
	done   func() // after captions are saved, e.g. to update the search index
	client *http.Client

	mu         sync.Mutex
	installing bool
	progress   float64
	installErr string
	states     map[string]JobState
	queue      chan string

	// Automatic captions: whether they're on and since when, and the finished downloads.
	auto      func() (bool, time.Time)
	completed func(context.Context) ([]domain.Job, error)

	// Replaced in tests.
	goos, goarch string
	lookPath     func(string) (string, error)
}

// New prepares captions for downloads; job looks up a download by ID.
func New(dataDir string, ffmpeg func() (string, error), job func(context.Context, string) (domain.Job, error), done func()) *Service {
	return &Service{
		dir: filepath.Join(dataDir, "captions"), ffmpeg: ffmpeg, job: job, done: done,
		client: &http.Client{Timeout: 30 * time.Minute},
		states: map[string]JobState{}, queue: make(chan string, 500),
		goos: runtime.GOOS, goarch: runtime.GOARCH, lookPath: exec.LookPath,
	}
}

// Status reports whether captions can be made, and each requested download's.
func (s *Service) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	status := Status{Installing: s.installing, Progress: s.progress, Error: s.installErr, Size: "about 160 MB", Jobs: map[string]JobState{}}
	for id, state := range s.states {
		status.Jobs[id] = state
	}
	_, buildable := builds[s.goos+"/"+s.goarch]
	_, whisperErr := s.whisper()
	switch {
	case buildable:
		status.Supported = true
	case s.goos == "darwin":
		status.Supported = true
		if whisperErr != nil {
			status.HowTo = "Install whisper.cpp with Homebrew first: brew install whisper-cpp"
		}
		status.Size = "about 150 MB"
	}
	status.Installed = whisperErr == nil && s.modelReady()
	return status
}

// homebrewWhisper is where Homebrew puts whisper-cli on Apple silicon and Intel Macs.
var homebrewWhisper = []string{"/opt/homebrew/bin/whisper-cli", "/usr/local/bin/whisper-cli"}

// whisper finds whisper-cli: the copy YTGrab installed, or on macOS Homebrew's.
func (s *Service) whisper() (string, error) {
	name := "whisper-cli"
	if s.goos == "windows" {
		name += ".exe"
	}
	if path := filepath.Join(s.dir, "whisper", name); fileExists(path) {
		return path, nil
	}
	if s.goos == "darwin" {
		for _, path := range homebrewWhisper {
			if fileExists(path) {
				return path, nil
			}
		}
		if path, err := s.lookPath("whisper-cli"); err == nil {
			return path, nil
		}
	}
	return "", errors.New("whisper.cpp isn't installed")
}

func (s *Service) modelPath() string { return filepath.Join(s.dir, model.name) }

func (s *Service) modelReady() bool {
	info, err := os.Stat(s.modelPath())
	return err == nil && info.Size() == model.size
}

// Request queues captions for a finished download that has no subtitles yet, or with
// translate, English captions of its speech in another language.
func (s *Service) Request(ctx context.Context, jobID string, translate bool) error {
	if !s.Status().Installed {
		return ErrNotInstalled
	}
	job, err := s.job(ctx, jobID)
	if err != nil {
		return err
	}
	if _, owned := library.Owned(job); !owned {
		return ErrNoFile
	}
	key := jobID
	if translate {
		key += Translated
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if state := s.states[key].State; state == Queued || state == Making {
		return nil
	}
	select {
	case s.queue <- key:
		s.states[key] = JobState{State: Queued}
		return nil
	default:
		return ErrBusy
	}
}

var (
	ErrNotInstalled = errors.New("Install local captions in Settings first.")
	ErrNoFile       = errors.New("Only finished downloads whose file is still there can get captions.")
	ErrBusy         = errors.New("Too many captions are waiting; try again later.")
)

// Automatic makes captions by itself for downloads that finish without subtitles while
// on() reports it on; completed lists the finished downloads.
func (s *Service) Automatic(on func() (bool, time.Time), completed func(context.Context) ([]domain.Job, error)) {
	s.auto, s.completed = on, completed
}

// queueNew queues captions for downloads finished since automatic captions were turned
// on that have no subtitles and haven't been tried yet.
func (s *Service) queueNew(ctx context.Context) {
	if s.auto == nil || s.completed == nil {
		return
	}
	on, since := s.auto()
	if !on || !s.Status().Installed {
		return
	}
	jobs, err := s.completed(ctx)
	if err != nil {
		return
	}
	for _, job := range jobs {
		if job.UpdatedAt.Before(since) || library.HasSubtitles(job) {
			continue
		}
		s.mu.Lock()
		_, tried := s.states[job.ID]
		s.mu.Unlock()
		if !tried {
			_ = s.Request(ctx, job.ID, false)
		}
	}
}

// Run makes queued captions one at a time until ctx ends, and looks for new downloads to
// caption every minute while automatic captions are on.
func (s *Service) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.queueNew(ctx)
		case key := <-s.queue:
			s.set(key, JobState{State: Making})
			id, translate := strings.CutSuffix(key, Translated)
			if err := s.make(ctx, id, translate); err != nil {
				if ctx.Err() != nil {
					return
				}
				s.set(key, JobState{State: Failed, Error: err.Error()})
				continue
			}
			s.set(key, JobState{State: Done})
			if s.done != nil {
				s.done()
			}
		}
	}
}

func (s *Service) set(id string, state JobState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.states[id] = state
}

// make turns a download's sound into 16 kHz mono WAV, which whisper reads, and has whisper
// write "<name>.whisper-<language>.srt" beside it, in the language it detects; with
// translate, "<name>.whisper-en.srt" in English.
func (s *Service) make(ctx context.Context, jobID string, translate bool) error {
	job, err := s.job(ctx, jobID)
	if err != nil {
		return errors.New("The download is no longer in the Library.")
	}
	video, owned := library.Owned(job)
	if !owned || !fileExists(video) {
		return ErrNoFile
	}
	whisper, err := s.whisper()
	if err != nil {
		return ErrNotInstalled
	}
	ffmpeg, err := s.ffmpeg()
	if err != nil {
		return errors.New("FFmpeg is needed to read the sound.")
	}
	work, err := os.MkdirTemp("", "ytgrab-captions-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	wav := filepath.Join(work, "audio.wav")
	if _, err := s.run(ctx, ffmpeg, nil, "-nostdin", "-v", "error", "-y", "-i", video, "-vn", "-ar", "16000", "-ac", "1", "-c:a", "pcm_s16le", wav); err != nil {
		return fmt.Errorf("Couldn't read the sound of this download: %w", err)
	}
	// whisper writes <of>.srt; keep it in the work folder until it is complete.
	out := filepath.Join(work, "captions")
	var env []string
	if s.goos == "linux" {
		env = append(os.Environ(), "LD_LIBRARY_PATH="+filepath.Dir(whisper)) // its libraries sit beside it
	}
	args := []string{"-m", s.modelPath(), "-f", wav, "-l", "auto", "-osrt", "-of", out, "-t", fmt.Sprint(threads())}
	if translate {
		args = append(args, "-tr")
	}
	log, err := s.run(ctx, whisper, env, args...)
	if err != nil {
		return fmt.Errorf("whisper couldn't make captions: %w", err)
	}
	language := "en"
	if !translate {
		language = ""
		if m := detectedLanguage.FindStringSubmatch(log); m != nil {
			language = m[1]
		}
	}
	target := strings.TrimSuffix(video, filepath.Ext(video)) + Suffix
	if language != "" {
		target += "-" + language
	}
	target += ".srt"
	data, err := os.ReadFile(out + ".srt")
	if err != nil {
		return errors.New("whisper finished without writing captions.")
	}
	return os.WriteFile(target, data, 0o644)
}

// threads leaves half the processor for everything else (at least 2, at most 8).
func threads() int {
	return max(2, min(8, runtime.NumCPU()/2))
}

// run runs a tool and returns what it logged (the first 256 KB), or its last line as the
// error when it fails.
func (s *Service) run(ctx context.Context, program string, env []string, args ...string) (string, error) {
	cmd := exec.Command(program, args...)
	cmd.Dir = filepath.Dir(program)
	cmd.Env = env
	var stderr strings.Builder
	cmd.Stderr = &limited{&stderr, 256 << 10}
	stop, err := process.Start(ctx, cmd)
	if err != nil {
		return "", err
	}
	err = cmd.Wait()
	stop()
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if err != nil {
		if last := lastLine(stderr.String()); last != "" {
			return "", errors.New(last)
		}
		return "", err
	}
	return stderr.String(), nil
}

type limited struct {
	b     *strings.Builder
	limit int
}

func (l *limited) Write(p []byte) (int, error) {
	if room := l.limit - l.b.Len(); room > 0 {
		if len(p) > room {
			l.b.Write(p[:room])
		} else {
			l.b.Write(p)
		}
	}
	return len(p), nil
}

func lastLine(text string) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}
