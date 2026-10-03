package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Sanoy24/ytgrab/internal/app/deps"
)

func TestPageCannotBeFramed(t *testing.T) {
	handler := NewHandler(func(context.Context) deps.Report { return deps.Report{} }, nil, nil)
	for _, path := range []string{"/", "/api/system/health"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		h := response.Header()
		if !strings.Contains(h.Get("Content-Security-Policy"), "frame-ancestors 'none'") || h.Get("X-Frame-Options") != "DENY" || h.Get("Referrer-Policy") != "no-referrer" {
			t.Errorf("%s: headers %v", path, h)
		}
	}
}
