package ytdlp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Sanoy24/ytgrab/internal/config"
	"github.com/Sanoy24/ytgrab/internal/domain"
)

func TestEvidenceKeepsWhatWasServed(t *testing.T) {
	job, _ := domain.NewJob("https://youtu.be/jNQXAC9IVRw", domain.Video720)
	job.Evidence = true
	cfg := evidenceConfig(config.Config{SponsorBlock: "remove", NormalizeAudio: true, SubtitlesMode: "embed", SubtitlesLang: "en"})
	joined := " " + strings.Join(buildArgs(job, cfg), " ") + " "
	for _, want := range []string{" -k ", " --write-info-json ", " --write-description ", " --write-thumbnail ", " --write-subs "} {
		if !strings.Contains(joined, want) {
			t.Errorf("args lack %q: %s", want, joined)
		}
	}
	for _, unwanted := range []string{"--embed-metadata", "--embed-chapters", "--embed-thumbnail", "--embed-subs", "--sponsorblock", "--convert-thumbnails", "loudnorm"} {
		if strings.Contains(joined, unwanted) {
			t.Errorf("args change the files with %q: %s", unwanted, joined)
		}
	}
}

func TestCaptureRecord(t *testing.T) {
	dir := t.TempDir()
	output := filepath.Join(dir, "Me at the zoo [jNQXAC9IVRw] 240p.mp4")
	files := map[string]string{
		"Me at the zoo [jNQXAC9IVRw] 240p.mp4":       "merged",
		"Me at the zoo [jNQXAC9IVRw] 240p.f133.mp4":  "video as served",
		"Me at the zoo [jNQXAC9IVRw] 240p.f140.m4a":  "audio as served",
		"Me at the zoo [jNQXAC9IVRw] 240p.info.json": `{"title":"Me at the zoo","channel":"jawed","upload_date":"20050424","webpage_url":"https://www.youtube.com/watch?v=jNQXAC9IVRw"}`,
		"Me at the zoo [jNQXAC9IVRw] 240p 2.mp4":     "another download", // starts alike, isn't this one
	}
	for name, text := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	job, _ := domain.NewJob("https://youtu.be/jNQXAC9IVRw", domain.Video720)
	finished := time.Date(2026, 10, 17, 18, 30, 0, 0, time.UTC)
	if err := writeEvidence(context.Background(), config.Config{Version: "1.23.0"}, job, output, finished); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(strings.TrimSuffix(output, ".mp4") + EvidenceSuffix)
	if err != nil {
		t.Fatal(err)
	}
	var record Evidence
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatal(err)
	}
	if record.Channel != "jawed" || record.UploadDate != "2005-04-24" || record.CapturedAt != "2026-10-17T18:30:00Z" || record.Tools["ytgrab"] != "1.23.0" || record.Site != "youtube" {
		t.Errorf("record = %+v", record)
	}
	if len(record.Files) != 4 {
		t.Fatalf("files = %+v, want the merged file, both streams, and info.json", record.Files)
	}
	for _, file := range record.Files {
		sum := sha256.Sum256([]byte(files[file.Name]))
		if file.SHA256 != hex.EncodeToString(sum[:]) || file.Bytes != int64(len(files[file.Name])) {
			t.Errorf("%s: %+v", file.Name, file)
		}
	}
}
