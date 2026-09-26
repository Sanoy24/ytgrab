package ytdlp

import (
	"strings"
	"testing"
	"time"

	"ytgrab/internal/config"
	"ytgrab/internal/domain"
)

func TestParseInspectionAndCachedSelection(t *testing.T) {
	const id = "jNQXAC9IVRw"
	data := []byte(`{"id":"jNQXAC9IVRw","title":"Example","duration":19,"formats":[{"format_id":"137","ext":"mp4","height":1080,"width":1920,"fps":30,"vcodec":"avc1.640028","acodec":"none","filesize_approx":1000},{"format_id":"140","ext":"m4a","vcodec":"none","acodec":"mp4a.40.2","abr":129.5,"filesize":200},{"format_id":"18","ext":"mp4","vcodec":"avc1","acodec":"mp4a","filesize":300},{"format_id":"bad+selector","ext":"mp4","vcodec":"avc1","acodec":"none"}]}`)
	result, err := parseInspection(data, id)
	if err != nil || len(result.Video) != 1 || len(result.Audio) != 1 || result.Video[0].ID != "137" || result.Audio[0].ID != "140" {
		t.Fatalf("inspection = %+v, %v", result, err)
	}
	inspector := NewInspector(config.Config{})
	inspector.cache[id] = cachedInspection{result: result, expires: time.Now().Add(time.Minute)}
	selection, ok := inspector.Select(id, "video", "137")
	if !ok || selection.Label != "Video · 1080p" {
		t.Fatalf("video selection = %+v, %v", selection, ok)
	}
	if _, ok := inspector.Select(id, "audio", "137"); ok {
		t.Fatal("video ID accepted as audio")
	}
	if _, ok := inspector.Select(id, "video", "137+ba"); ok {
		t.Fatal("free-form selector accepted")
	}
	if _, err := parseInspection(data, "dQw4w9WgXcQ"); err == nil {
		t.Fatal("mismatched video ID accepted")
	}
}

func TestSelectedFormatArguments(t *testing.T) {
	for _, test := range []struct {
		kind string
		want string
	}{
		{"video", "137+ba/137"},
		{"audio", "140"},
	} {
		job, err := domain.NewFormatJob("https://youtu.be/jNQXAC9IVRw", domain.FormatSelection{Kind: test.kind, ID: map[string]string{"video": "137", "audio": "140"}[test.kind], Label: "Selected"})
		if err != nil {
			t.Fatal(err)
		}
		args := buildArgs(job, config.Config{})
		joined := strings.Join(args, " ")
		if !strings.Contains(joined, "-f "+test.want) || args[len(args)-1] != job.URL {
			t.Fatalf("format args = %v", args)
		}
	}
}
