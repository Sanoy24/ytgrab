package ytdlp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Sanoy24/ytgrab/internal/app/deps"
	"github.com/Sanoy24/ytgrab/internal/config"
	"github.com/Sanoy24/ytgrab/internal/domain"
)

// EvidenceSuffix names an evidence download's capture record: "<name>.evidence.json".
const EvidenceSuffix = ".evidence.json"

// Evidence is the capture record of an evidence download, following the practice of the
// Berkeley Protocol on Digital Open Source Investigations: where it came from, when, with
// which tools, and a checksum of every file, so anyone can later check nothing changed.
type Evidence struct {
	Note       string            `json:"note"`
	SourceURL  string            `json:"source_url"`
	PageURL    string            `json:"page_url,omitempty"`
	Site       string            `json:"site"`
	VideoID    string            `json:"video_id"`
	Title      string            `json:"title,omitempty"`
	Channel    string            `json:"channel,omitempty"`
	ChannelURL string            `json:"channel_url,omitempty"`
	UploadDate string            `json:"upload_date,omitempty"` // YYYY-MM-DD, as the site reports it
	CapturedAt string            `json:"captured_at"`           // UTC, when the download finished
	Tools      map[string]string `json:"tools"`
	Files      []EvidenceFile    `json:"files"`
}

// EvidenceFile is one file of the download with its SHA-256.
type EvidenceFile struct {
	Name   string `json:"name"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
}

// evidenceConfig turns off everything that would change what the site served: no embedded
// metadata or cover art, no sponsor cuts, no loudness changes, subtitles as separate files.
func evidenceConfig(cfg config.Config) config.Config {
	cfg.SponsorBlock = ""
	cfg.NormalizeAudio = false
	if cfg.SubtitlesMode == "embed" {
		cfg.SubtitlesMode = "file"
	}
	cfg.SaveMetadata = true
	return cfg
}

// writeEvidence hashes every file of a finished evidence download (the main file, the
// streams as served, and the metadata yt-dlp saved) and writes the capture record beside
// them.
func writeEvidence(ctx context.Context, cfg config.Config, job domain.Job, output string, finished time.Time) error {
	base := strings.TrimSuffix(output, filepath.Ext(output))
	record := Evidence{
		Note:       "Files ending in .f<number>.<ext> are the streams exactly as the site served them; the main file joins them without re-encoding. Check a file with any SHA-256 tool.",
		SourceURL:  job.URL,
		Site:       siteName(job.Site),
		VideoID:    job.VideoID,
		CapturedAt: finished.UTC().Format(time.RFC3339),
		Tools:      map[string]string{"ytgrab": cfg.Version},
	}
	if path, err := deps.Find(cfg, "yt-dlp"); err == nil {
		if version, err := deps.Version(ctx, path); err == nil {
			record.Tools["yt-dlp"] = version
		}
	}
	if path, err := deps.Find(cfg, "ffmpeg"); err == nil {
		if version, err := deps.FFmpegVersion(ctx, path); err == nil {
			record.Tools["ffmpeg"] = version
		}
	}
	if data, err := os.ReadFile(base + ".info.json"); err == nil {
		var info struct {
			Title      string `json:"title"`
			Channel    string `json:"channel"`
			Uploader   string `json:"uploader"`
			ChannelURL string `json:"channel_url"`
			UploadDate string `json:"upload_date"`
			PageURL    string `json:"webpage_url"`
		}
		if json.Unmarshal(data, &info) == nil {
			record.Title, record.Channel, record.ChannelURL, record.PageURL = info.Title, info.Channel, info.ChannelURL, info.PageURL
			if record.Channel == "" {
				record.Channel = info.Uploader
			}
			if d := info.UploadDate; len(d) == 8 {
				record.UploadDate = d[:4] + "-" + d[4:6] + "-" + d[6:]
			}
		}
	}
	dir, prefix := filepath.Dir(output), filepath.Base(base)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		if !entry.Type().IsRegular() || !strings.HasPrefix(name, prefix) || strings.HasSuffix(name, EvidenceSuffix) || strings.HasSuffix(name, ".part") {
			continue
		}
		rest := name[len(prefix):]
		if !strings.HasPrefix(rest, ".") {
			continue // another file whose name merely starts the same
		}
		file, err := hashFile(filepath.Join(dir, name))
		if err != nil {
			return err
		}
		record.Files = append(record.Files, file)
	}
	sort.Slice(record.Files, func(i, j int) bool { return record.Files[i].Name < record.Files[j].Name })
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(base+EvidenceSuffix, append(data, '\n'), 0o644)
}

func hashFile(path string) (EvidenceFile, error) {
	file, err := os.Open(path)
	if err != nil {
		return EvidenceFile{}, err
	}
	defer file.Close()
	hash := sha256.New()
	n, err := io.Copy(hash, file)
	if err != nil {
		return EvidenceFile{}, err
	}
	return EvidenceFile{Name: filepath.Base(path), Bytes: n, SHA256: hex.EncodeToString(hash.Sum(nil))}, nil
}

func siteName(site string) string {
	if site == "" {
		return "youtube"
	}
	return site
}
