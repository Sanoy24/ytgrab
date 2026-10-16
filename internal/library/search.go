package library

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode"

	"github.com/Sanoy24/ytgrab/internal/domain"
	sqlitestore "github.com/Sanoy24/ytgrab/internal/store/sqlite"
)

// IndexStore keeps the search index (the SQLite store).
type IndexStore interface {
	CompletedJobs(context.Context) ([]domain.Job, error)
	IndexedStamps(context.Context) (map[string]string, error)
	IndexJob(ctx context.Context, jobID, stamp string, chunks []sqlitestore.SearchChunk) error
	Unindex(context.Context, string) error
}

var indexing sync.Mutex

// RefreshIndex brings the search index up to date: finished downloads whose files changed
// (or are new) are indexed again, and removed ones are dropped. Downloads already indexed
// cost one look at their folder each.
func RefreshIndex(ctx context.Context, store IndexStore) error {
	indexing.Lock()
	defer indexing.Unlock()
	jobs, err := store.CompletedJobs(ctx)
	if err != nil {
		return err
	}
	stamps, err := store.IndexedStamps(ctx)
	if err != nil {
		return err
	}
	current := map[string]bool{}
	for _, job := range jobs {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		current[job.ID] = true
		subtitles := subtitleFiles(job)
		stamp := indexStamp(job, subtitles)
		if stamps[job.ID] == stamp {
			continue
		}
		chunks := []sqlitestore.SearchChunk{{Start: -1, Text: jobTitle(job)}}
		for _, path := range subtitles {
			chunks = append(chunks, readSubtitles(path)...)
		}
		if err := store.IndexJob(ctx, job.ID, stamp, chunks); err != nil {
			return err
		}
	}
	for id := range stamps {
		if !current[id] {
			if err := store.Unindex(ctx, id); err != nil {
				return err
			}
		}
	}
	return nil
}

func jobTitle(job domain.Job) string {
	if job.Title != nil && *job.Title != "" {
		return *job.Title
	}
	return job.VideoID
}

// indexStamp changes when the title or any of the download's subtitle files does.
func indexStamp(job domain.Job, subtitles []string) string {
	parts := []string{jobTitle(job)}
	for _, path := range subtitles {
		if info, err := os.Stat(path); err == nil {
			parts = append(parts, fmt.Sprintf("%s:%d:%d", filepath.Base(path), info.Size(), info.ModTime().UnixNano()))
		}
	}
	return strings.Join(parts, "|")
}

// subtitleFile matches what yt-dlp saves beside a download: "Title [id] 720p.en.srt".
var subtitleFile = regexp.MustCompile(`^(\.[A-Za-z0-9_-]{1,20})?\.(srt|vtt)$`)

// subtitleFiles lists the subtitle files saved beside a finished download.
func subtitleFiles(job domain.Job) []string {
	path, owned := Owned(job)
	if !owned {
		return nil
	}
	dir, base := filepath.Dir(path), strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var files []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.Type().IsRegular() && strings.HasPrefix(name, base) && subtitleFile.MatchString(name[len(base):]) {
			files = append(files, filepath.Join(dir, name))
		}
	}
	sort.Strings(files)
	return files
}

// Chunks gather a few subtitle lines: long enough to match a phrase, short enough that
// its start time takes you near the words.
const (
	chunkWords   = 24
	chunkSeconds = 30
)

var (
	cueTiming = regexp.MustCompile(`^((?:\d+:)?\d{1,2}:\d{2})[.,](\d{1,3})\s+-->`)
	cueMarkup = regexp.MustCompile(`<[^>]*>|\{\\[^}]*\}`)
)

// readSubtitles reads an SRT or WebVTT file as chunks of text with their start times.
// YouTube's automatic captions repeat each line as the next one rolls in; repeats are kept
// once. A file that can't be read gives no chunks.
func readSubtitles(path string) []sqlitestore.SearchChunk {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()
	var chunks []sqlitestore.SearchChunk
	var words []string
	chunkStart, inCue := -1.0, false
	last := ""
	flush := func() {
		if len(words) > 0 {
			chunks = append(chunks, sqlitestore.SearchChunk{Start: chunkStart, Text: strings.Join(words, " ")})
		}
		words, chunkStart = nil, -1
	}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64<<10), 1<<20)
	for scanner.Scan() {
		line := strings.TrimSpace(strings.TrimPrefix(scanner.Text(), bom))
		if m := cueTiming.FindStringSubmatch(line); m != nil {
			start := clockSeconds(m[1], m[2])
			if chunkStart >= 0 && (len(words) >= chunkWords || start-chunkStart >= chunkSeconds) {
				flush()
			}
			if chunkStart < 0 {
				chunkStart = start
			}
			inCue = true
			continue
		}
		if line == "" {
			inCue = false
			continue
		}
		if !inCue {
			continue // a cue number, the WEBVTT header, or a note
		}
		text := strings.Join(strings.Fields(cueMarkup.ReplaceAllString(line, "")), " ")
		if text == "" || text == last {
			continue
		}
		last = text
		words = append(words, strings.Fields(text)...)
	}
	flush()
	return chunks
}

// bom is the byte-order mark some subtitle files start with.
const bom = "\xef\xbb\xbf"

// clockSeconds reads "1:02:03" or "02:03" plus milliseconds.
func clockSeconds(clock, millis string) float64 {
	total := 0.0
	for _, part := range strings.Split(clock, ":") {
		n, _ := strconv.Atoi(part)
		total = total*60 + float64(n)
	}
	ms, _ := strconv.Atoi((millis + "00")[:3])
	return total + float64(ms)/1000
}

// SearchQuery turns typed words into an FTS5 query: every word must appear, and the last
// may be unfinished ("binary tre" finds "binary tree"). Only letters and digits get
// through, so nothing typed is read as query syntax. "" when nothing is left.
func SearchQuery(text string) string {
	words := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	if len(words) > 8 {
		words = words[:8]
	}
	for i, word := range words {
		words[i] = `"` + word + `"`
	}
	if len(words) > 0 {
		words[len(words)-1] += "*"
	}
	return strings.Join(words, " ")
}
