package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/Sanoy24/ytgrab/internal/app/deps"
	"github.com/Sanoy24/ytgrab/internal/downloader/ytdlp"
)

type fakeSearcher struct{ fakeInspector }

func (fakeSearcher) Search(_ context.Context, query string) ([]ytdlp.SearchResult, error) {
	if len(query) < 2 {
		return nil, ytdlp.ErrInvalidQuery
	}
	if query == "blocked" {
		return nil, &ytdlp.Error{Code: "blocked", Message: "YouTube is limiting requests from this network."}
	}
	return []ytdlp.SearchResult{{VideoID: "jNQXAC9IVRw", Title: "Me at the zoo", Channel: "jawed"}}, nil
}

func TestSearchRoute(t *testing.T) {
	handler := NewHandlerWithInspector(func(context.Context) deps.Report { return deps.Report{} }, nil, nil, fakeSearcher{})
	get := func(q string) (int, map[string]any) {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/search?q="+url.QueryEscape(q), nil))
		var body map[string]any
		_ = json.Unmarshal(response.Body.Bytes(), &body)
		return response.Code, body
	}
	if code, body := get("me at the zoo"); code != http.StatusOK || len(body["results"].([]any)) != 1 {
		t.Fatalf("search = %d %v", code, body)
	}
	if code, _ := get("x"); code != http.StatusBadRequest {
		t.Fatalf("short query = %d", code)
	}
	if code, _ := get("blocked"); code != http.StatusTooManyRequests && code != http.StatusServiceUnavailable {
		t.Fatalf("blocked search = %d", code)
	}
}
