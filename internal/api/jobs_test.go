package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Sanoy24/ytgrab/internal/app/deps"
	"github.com/Sanoy24/ytgrab/internal/domain"
	sqlitestore "github.com/Sanoy24/ytgrab/internal/store/sqlite"
)

func TestJobAPIContract(t *testing.T) {
	store, err := sqlitestore.Open(context.Background(), filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	handler := NewHandler(func(context.Context) deps.Report { return deps.Report{} }, store, nil)
	request := func(method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(method, path, bytes.NewBufferString(body)))
		return response
	}

	created := request(http.MethodPost, "/api/jobs", `{"url":"https://youtu.be/dQw4w9WgXcQ","preset":"video-best"}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("create status = %d: %s", created.Code, created.Body.String())
	}
	var job domain.Job
	if err := json.Unmarshal(created.Body.Bytes(), &job); err != nil || job.State != domain.Queued {
		t.Fatalf("created job = %+v, %v", job, err)
	}
	if duplicate := request(http.MethodPost, "/api/jobs", `{"url":"https://www.youtube.com/watch?v=dQw4w9WgXcQ","preset":"audio-m4a"}`); duplicate.Code != http.StatusConflict {
		t.Fatalf("duplicate status = %d, want 409", duplicate.Code)
	}
	if invalid := request(http.MethodPost, "/api/jobs", `{"url":"https://evil.example/video","preset":"video-best"}`); invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid URL status = %d, want 400", invalid.Code)
	}
	listed := request(http.MethodGet, "/api/jobs", "")
	var list struct {
		Jobs []domain.Job `json:"jobs"`
	}
	if err := json.Unmarshal(listed.Body.Bytes(), &list); err != nil || len(list.Jobs) != 1 {
		t.Fatalf("listed jobs = %+v, %v", list, err)
	}
	if got := request(http.MethodGet, "/api/jobs/"+job.ID, ""); got.Code != http.StatusOK {
		t.Fatalf("get status = %d, want 200", got.Code)
	}
	if cancelled := request(http.MethodPost, "/api/jobs/"+job.ID+"/cancel", ""); cancelled.Code != http.StatusOK {
		t.Fatalf("cancel status = %d: %s", cancelled.Code, cancelled.Body.String())
	}
	events := request(http.MethodGet, "/api/jobs/"+job.ID+"/events", "")
	if events.Code != http.StatusOK || !bytes.Contains(events.Body.Bytes(), []byte(`"state":"cancelled"`)) {
		t.Fatalf("terminal SSE response = %d: %s", events.Code, events.Body.String())
	}
	retried := request(http.MethodPost, "/api/jobs/"+job.ID+"/retry", "")
	if err := json.Unmarshal(retried.Body.Bytes(), &job); err != nil || job.State != domain.Queued || job.Attempt != 2 {
		t.Fatalf("retried job = %+v, %v", job, err)
	}

	crossOrigin := httptest.NewRequest(http.MethodPost, "/api/jobs/"+job.ID+"/cancel", nil)
	crossOrigin.Header.Set("Origin", "https://untrusted.example")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, crossOrigin)
	if response.Code != http.StatusForbidden {
		t.Fatalf("cross-origin status = %d, want 403", response.Code)
	}
}

func TestJobEventsStreamUpdates(t *testing.T) {
	ctx := context.Background()
	store, err := sqlitestore.Open(ctx, filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	job, err := domain.NewJob("https://youtu.be/dQw4w9WgXcQ", domain.AudioM4A)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Create(ctx, job); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(NewHandler(func(context.Context) deps.Report { return deps.Report{} }, store, nil))
	defer server.Close()
	client := &http.Client{Timeout: 5 * time.Second}
	response, err := client.Get(server.URL + "/api/jobs/" + job.ID + "/events")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("SSE response = %d, %q", response.StatusCode, response.Header.Get("Content-Type"))
	}
	reader := bufio.NewReader(response.Body)
	readState := func() domain.Job {
		t.Helper()
		var line string
		for {
			line, err = reader.ReadString('\n')
			if err != nil || strings.TrimSpace(line) != "" {
				break
			}
		}
		if err != nil || !strings.HasPrefix(line, "data: ") {
			t.Fatalf("SSE line = %q, %v", line, err)
		}
		var observed domain.Job
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &observed); err != nil {
			t.Fatal(err)
		}
		return observed
	}
	if initial := readState(); initial.State != domain.Queued {
		t.Fatalf("initial SSE state = %s", initial.State)
	}
	if err := job.Transition(domain.Downloading); err != nil {
		t.Fatal(err)
	}
	job.Progress = &domain.Progress{DownloadedBytes: 1234}
	if err := store.Update(ctx, job, domain.Queued); err != nil {
		t.Fatal(err)
	}
	if updated := readState(); updated.State != domain.Downloading || updated.Progress == nil || updated.Progress.DownloadedBytes != 1234 {
		t.Fatalf("updated SSE job = %+v", updated)
	}
}

func TestJobAudioLanguage(t *testing.T) {
	store, err := sqlitestore.Open(context.Background(), filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	handler := NewHandler(func(context.Context) deps.Report { return deps.Report{} }, store, nil)
	post := func(body string) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/jobs", bytes.NewBufferString(body)))
		return response
	}
	// Only language codes reach yt-dlp's format filter.
	if got := post(`{"url":"https://youtu.be/Txzj3pNt20o","preset":"video-compat","audio_language":"es]+bv*"}`); got.Code != http.StatusBadRequest {
		t.Fatalf("invalid language = %d", got.Code)
	}
	created := post(`{"url":"https://youtu.be/Txzj3pNt20o","preset":"video-compat","audio_language":"es-US"}`)
	var job domain.Job
	if err := json.Unmarshal(created.Body.Bytes(), &job); err != nil || created.Code != http.StatusCreated || job.AudioLanguage != "es-US" || *job.Preset != domain.VideoCompat {
		t.Fatalf("created = %d %s", created.Code, created.Body.String())
	}
	if stored, _ := store.Get(context.Background(), job.ID); stored.AudioLanguage != "es-US" {
		t.Errorf("stored language = %q", stored.AudioLanguage)
	}
}
