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

func TestParsePlaylistURL(t *testing.T) {
	const list = "PLbpi6ZahtOH6Blw3RGYpWkSByi_T7Rygb"
	for _, raw := range []string{
		"https://www.youtube.com/playlist?list=" + list,
		"https://youtube.com/watch?v=dQw4w9WgXcQ&list=" + list + "&index=3",
		"https://music.youtube.com/playlist?list=" + list,
	} {
		canonical, id, err := ParsePlaylistURL(raw)
		if err != nil || id != list || canonical != "https://www.youtube.com/playlist?list="+list {
			t.Errorf("ParsePlaylistURL(%q) = %q, %q, %v", raw, canonical, id, err)
		}
	}
	for _, raw := range []string{
		"https://www.youtube.com/watch?v=dQw4w9WgXcQ",
		"https://www.youtube.com/playlist?list=short",
		"https://www.youtube.com/playlist?list=" + list + "%22%3B",
		"https://evil.example/playlist?list=" + list,
	} {
		if _, _, err := ParsePlaylistURL(raw); err == nil {
			t.Errorf("ParsePlaylistURL(%q) unexpectedly succeeded", raw)
		}
	}
	if _, _, err := ParsePlaylistURL("https://www.youtube.com/watch?v=dQw4w9WgXcQ&list=RDdQw4w9WgXcQ"); err != ErrMixPlaylist {
		t.Errorf("mix playlist error = %v", err)
	}
}

func TestPlaylistJobsUseCanonicalVideoURLs(t *testing.T) {
	job, err := NewJob(VideoURL("dQw4w9WgXcQ"), AudioM4A)
	if err != nil || job.URL != "https://www.youtube.com/watch?v=dQw4w9WgXcQ" || job.VideoID != "dQw4w9WgXcQ" {
		t.Fatalf("job = %+v, %v", job, err)
	}
	if ValidVideoID("dQw4w9WgXcQ&x") || !ValidVideoID("dQw4w9WgXcQ") {
		t.Fatal("video ID validation is wrong")
	}
}

func TestRequeueReturnsRunningJobToQueue(t *testing.T) {
	job, err := NewJob("https://youtu.be/dQw4w9WgXcQ", AudioM4A)
	if err != nil {
		t.Fatal(err)
	}
	if err := job.Requeue(); err == nil {
		t.Fatal("a queued job cannot be requeued")
	}
	if err := job.Transition(Downloading); err != nil {
		t.Fatal(err)
	}
	job.Progress = &Progress{DownloadedBytes: 10}
	if err := job.Requeue(); err != nil {
		t.Fatal(err)
	}
	if job.State != Queued || job.Attempt != 2 || job.Progress != nil || !CanTransition(Downloading, Queued) {
		t.Fatalf("requeued job = %+v", job)
	}
	if CanTransition(Completed, Queued) {
		t.Fatal("completed jobs must not return to the queue")
	}
}
