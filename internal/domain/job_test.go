package domain

import "testing"

func TestParseVideoURL(t *testing.T) {
	valid := []string{
		"https://www.youtube.com/watch?v=dQw4w9WgXcQ",
		"https://youtu.be/dQw4w9WgXcQ",
		"https://m.youtube.com/shorts/dQw4w9WgXcQ",
		"https://youtube.com/live/dQw4w9WgXcQ",
	}
	for _, raw := range valid {
		_, id, err := ParseVideoURL(raw)
		if err != nil || id != "dQw4w9WgXcQ" {
			t.Errorf("ParseVideoURL(%q) = %q, %v", raw, id, err)
		}
	}
	invalid := []string{
		"https://evil.example/watch?v=dQw4w9WgXcQ",
		"https://youtube.com.evil.example/watch?v=dQw4w9WgXcQ",
		"https://youtube.com:444/watch?v=dQw4w9WgXcQ",
		"https://youtube.com/playlist?list=abc",
		"file:///tmp/video",
		"https://youtu.be/not-an-id",
	}
	for _, raw := range invalid {
		if _, _, err := ParseVideoURL(raw); err == nil {
			t.Errorf("ParseVideoURL(%q) unexpectedly succeeded", raw)
		}
	}
}

func TestRetryTransition(t *testing.T) {
	job, err := NewJob("https://youtu.be/dQw4w9WgXcQ", VideoBest)
	if err != nil {
		t.Fatal(err)
	}
	if err := job.Transition(Failed); err != nil {
		t.Fatal(err)
	}
	job.Error = &JobError{Code: "network", Message: "connection reset"}
	if err := job.Transition(Queued); err != nil {
		t.Fatal(err)
	}
	if job.Attempt != 2 || job.Error != nil || job.State != Queued {
		t.Fatalf("retry did not reset job: %+v", job)
	}
	if err := job.Transition(Completed); err == nil {
		t.Fatal("queued job should not transition directly to completed")
	}
}
