package library

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Sanoy24/ytgrab/internal/domain"
	sqlitestore "github.com/Sanoy24/ytgrab/internal/store/sqlite"
)

func write(t *testing.T, path, text string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestReadingSubtitles(t *testing.T) {
	dir := t.TempDir()
	srt := filepath.Join(dir, "a.en.srt")
	// YouTube's automatic captions: each line is repeated as the next one rolls in.
	write(t, srt, bom+"1\n00:00:01,000 --> 00:00:03,000\n<font color=\"#fff\">today we build</font>\n\n2\n00:00:03,000 --> 00:00:05,000\ntoday we build\na binary search tree\n\n3\n00:12:31,500 --> 00:12:34,000\n{\\an8}and balance it\n")
	chunks := readSubtitles(srt)
	if len(chunks) != 2 {
		t.Fatalf("chunks = %+v", chunks)
	}
	if chunks[0].Start != 1 || chunks[0].Text != "today we build a binary search tree" {
		t.Errorf("first = %+v", chunks[0])
	}
	if chunks[1].Start != 751.5 || chunks[1].Text != "and balance it" {
		t.Errorf("second = %+v", chunks[1])
	}

	vtt := filepath.Join(dir, "a.en.vtt")
	write(t, vtt, "WEBVTT\nKind: captions\n\n01:02.250 --> 01:04.000 align:start\nhello <00:01:02.500><c>world</c>\n")
	if chunks := readSubtitles(vtt); len(chunks) != 1 || chunks[0].Start != 62.25 || chunks[0].Text != "hello world" {
		t.Errorf("vtt = %+v", chunks)
	}
}

func TestSearchQueryOnlyPassesWords(t *testing.T) {
	for typed, want := range map[string]string{
		"Binary tre":          `"binary" "tre"*`,
		`tree" OR "x NEAR(a)`: `"tree" "or" "x" "near" "a"*`,
		"  ":                  "",
		"café-au-lait":        `"café" "au" "lait"*`,
	} {
		if got := SearchQuery(typed); got != want {
			t.Errorf("SearchQuery(%q) = %q, want %q", typed, got, want)
		}
	}
}

func TestIndexingTheLibrary(t *testing.T) {
	ctx := context.Background()
	store, err := sqlitestore.Open(ctx, filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	dir := t.TempDir()
	video := filepath.Join(dir, "Lecture 4 [jNQXAC9IVRw] 720p.mp4")
	write(t, video, "video")
	write(t, strings.TrimSuffix(video, ".mp4")+".en.srt", "1\n00:12:31,000 --> 00:12:33,000\nnow a binary search tree\n")
	job, _ := domain.NewJob("https://youtu.be/jNQXAC9IVRw", domain.Video720)
	title := "Lecture 4: Trees"
	job.Title = &title
	_ = store.Create(ctx, job)
	_ = job.Transition(domain.Downloading)
	_ = store.Update(ctx, job, domain.Queued)
	job.OutputPath = &video
	_ = job.Transition(domain.Completed)
	if err := store.Update(ctx, job, domain.Downloading); err != nil {
		t.Fatal(err)
	}

	if err := RefreshIndex(ctx, store); err != nil {
		t.Fatal(err)
	}
	hits, err := store.Search(ctx, SearchQuery("binary sear"), 10)
	if err != nil || len(hits) != 1 || hits[0].JobID != job.ID || hits[0].Start != 751 || !strings.Contains(hits[0].Snippet, "\x02binary\x03") {
		t.Fatalf("hits = %+v, %v", hits, err)
	}
	if hits, _ := store.Search(ctx, SearchQuery("trees"), 10); len(hits) != 1 || hits[0].Start != -1 {
		t.Fatalf("title hit = %+v", hits)
	}

	// Removing the download drops it from the index.
	if err := store.Delete(ctx, job.ID); err != nil {
		t.Fatal(err)
	}
	if err := RefreshIndex(ctx, store); err != nil {
		t.Fatal(err)
	}
	if hits, _ := store.Search(ctx, SearchQuery("binary"), 10); len(hits) != 0 {
		t.Errorf("still found after removal: %+v", hits)
	}
}
