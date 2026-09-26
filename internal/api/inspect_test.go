package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"ytgrab/internal/app/deps"
	"ytgrab/internal/domain"
	"ytgrab/internal/downloader/ytdlp"
	sqlitestore "ytgrab/internal/store/sqlite"
)

type fakeInspector struct{ failure error }

func (fake fakeInspector) Inspect(_ context.Context, url string) (ytdlp.Inspection, error) {
	if fake.failure != nil {
		return ytdlp.Inspection{}, fake.failure
	}
	if _, _, err := domain.ParseVideoURL(url); err != nil {
		return ytdlp.Inspection{}, err
	}
	return ytdlp.Inspection{VideoID: "jNQXAC9IVRw", Title: "Example", Video: []ytdlp.Format{{ID: "137", Ext: "mp4"}}, Audio: []ytdlp.Format{}}, nil
}

func (fakeInspector) Select(videoID, kind, id string) (domain.FormatSelection, bool) {
	if videoID == "jNQXAC9IVRw" && kind == "video" && id == "137" {
		return domain.FormatSelection{Kind: kind, ID: id, Label: "Video · 1080p"}, true
	}
	return domain.FormatSelection{}, false
}

func TestInspectionAndFormatJobContract(t *testing.T) {
	store, err := sqlitestore.Open(context.Background(), filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	handler := NewHandlerWithInspector(func(context.Context) deps.Report { return deps.Report{} }, store, nil, fakeInspector{})
	request := func(method, target, body string) *httptest.ResponseRecorder {
		t.Helper()
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(method, target, bytes.NewBufferString(body)))
		return response
	}
	const url = "https://youtu.be/jNQXAC9IVRw"
	if got := request(http.MethodGet, "/api/inspect?url="+url, ""); got.Code != http.StatusOK || !bytes.Contains(got.Body.Bytes(), []byte(`"format_id":"137"`)) {
		t.Fatalf("inspect = %d: %s", got.Code, got.Body.String())
	}
	if got := request(http.MethodGet, "/api/inspect?url=https://evil.example/", ""); got.Code != http.StatusBadRequest {
		t.Fatalf("invalid URL = %d: %s", got.Code, got.Body.String())
	}
	if got := request(http.MethodPost, "/api/jobs", `{"url":"`+url+`","format":{"kind":"video","id":"137+ba"}}`); got.Code != http.StatusBadRequest {
		t.Fatalf("unsafe ID = %d: %s", got.Code, got.Body.String())
	}
	if got := request(http.MethodPost, "/api/jobs", `{"url":"`+url+`","format":{"kind":"video","id":"999"}}`); got.Code != http.StatusConflict {
		t.Fatalf("uninspected ID = %d: %s", got.Code, got.Body.String())
	}
	created := request(http.MethodPost, "/api/jobs", `{"url":"`+url+`","format":{"kind":"video","id":"137"}}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("create = %d: %s", created.Code, created.Body.String())
	}
	var job domain.Job
	if err := json.Unmarshal(created.Body.Bytes(), &job); err != nil || job.Preset != nil || job.Format == nil || job.Format.Label != "Video · 1080p" {
		t.Fatalf("created format job = %+v, %v", job, err)
	}
}

func TestInspectionBlockedError(t *testing.T) {
	handler := NewHandlerWithInspector(func(context.Context) deps.Report { return deps.Report{} }, nil, nil,
		fakeInspector{failure: &ytdlp.Error{Code: "blocked", Message: "YouTube is limiting requests from this network."}})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/inspect?url=https://youtu.be/jNQXAC9IVRw", nil))
	if response.Code != http.StatusTooManyRequests || !bytes.Contains(response.Body.Bytes(), []byte(`"code":"blocked"`)) {
		t.Fatalf("blocked response = %d: %s", response.Code, response.Body.String())
	}
}
