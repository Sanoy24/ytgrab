package ytdlp

import (
	"context"
	"testing"

	"github.com/Sanoy24/ytgrab/internal/domain"
)

func TestUnsafeStoredFolderIsRefused(t *testing.T) {
	job, _ := domain.NewJob("https://youtu.be/jNQXAC9IVRw", domain.AudioM4A)
	job.Folder = "../outside"
	_, err := Downloader{}.Download(context.Background(), job, func(Event) error { return nil })
	if e, ok := err.(*Error); !ok || e.Code != "invalid_folder" {
		t.Fatalf("Download = %v", err)
	}
}
