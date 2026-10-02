package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Sanoy24/ytgrab/internal/app/deps"
)

func TestPageIsRevalidated(t *testing.T) {
	handler := NewHandler(func(context.Context) deps.Report { return deps.Report{} }, nil, nil)
	for _, path := range []string{"/", "/app.js", "/app.css"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-cache" {
			t.Errorf("%s: %d, Cache-Control %q", path, response.Code, response.Header().Get("Cache-Control"))
		}
	}
}
