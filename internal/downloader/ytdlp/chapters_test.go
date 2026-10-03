package ytdlp

import (
	"slices"
	"testing"

	"github.com/Sanoy24/ytgrab/internal/config"
	"github.com/Sanoy24/ytgrab/internal/domain"
)

func TestSplitChapters(t *testing.T) {
	job, _ := domain.NewJob("https://youtu.be/M7lc1UVf-VE", domain.AudioM4A)
	if slices.Contains(buildArgs(job, config.Config{}), "--split-chapters") {
		t.Fatal("split without asking")
	}
	job.SplitChapters = true
	args := buildArgs(job, config.Config{})
	if !slices.Contains(args, "--split-chapters") {
		t.Fatalf("args = %v", args)
	}
	want := "chapter:%(title).150B [%(id)s] %(abr).0fk/%(section_number)02d %(section_title).100B.%(ext)s"
	found := false
	for i, a := range args {
		found = found || (a == "-o" && i+1 < len(args) && args[i+1] == want)
	}
	if !found {
		t.Fatalf("chapter output template missing: %v", args)
	}
	if args[len(args)-1] != job.URL || args[len(args)-2] != "--" {
		t.Fatal("URL must stay the final argument")
	}
}

func TestInspectionCountsChapters(t *testing.T) {
	data := []byte(`{"id":"M7lc1UVf-VE","title":"x","chapters":[{"title":"Intro"},{"title":"Docs"}],"formats":[{"format_id":"140","ext":"m4a","vcodec":"none","acodec":"mp4a.40.2"}]}`)
	got, err := parseInspection(data, "M7lc1UVf-VE")
	if err != nil || got.Chapters != 2 {
		t.Fatalf("chapters = %d, %v", got.Chapters, err)
	}
}
