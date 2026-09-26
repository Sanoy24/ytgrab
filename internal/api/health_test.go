package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"ytgrab/internal/app/deps"
)

func TestHealthRouteReportsDegradedDependencies(t *testing.T) {
	handler := NewHandler(func(context.Context) deps.Report {
		return deps.Report{
			Status: "degraded",
			Dependencies: []deps.Tool{
				{Name: "yt-dlp", Required: true, Message: "Install yt-dlp."},
			},
		}
	})
	request := httptest.NewRequest(http.MethodGet, "/api/system/health", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	var report deps.Report
	if err := json.Unmarshal(response.Body.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.Status != "degraded" || len(report.Dependencies) != 1 || report.Dependencies[0].Available {
		t.Fatalf("unexpected report: %+v", report)
	}

	request = httptest.NewRequest(http.MethodPost, "/api/system/health", nil)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST status = %d, want 405", response.Code)
	}
}
