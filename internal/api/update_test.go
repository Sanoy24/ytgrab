package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Sanoy24/ytgrab/internal/app/deps"
)

type fakeUpdaterSettings struct {
	fakeSettingsOnly
	result YtdlpUpdate
	err    error
}

type fakeSettingsOnly struct{}

func (fakeSettingsOnly) DownloadsDir() string                          { return "/downloads" }
func (fakeSettingsOnly) SetDownloadsDir(context.Context, string) error { return nil }

func (fake *fakeUpdaterSettings) UpdateYtdlp(context.Context) (YtdlpUpdate, error) {
	return fake.result, fake.err
}

func TestUpdateYtdlpRoute(t *testing.T) {
	fake := &fakeUpdaterSettings{}
	handler := NewHandler(func(context.Context) deps.Report { return deps.Report{} }, nil, nil, fake)
	post := func() *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/system/update-ytdlp", nil))
		return response
	}
	for _, test := range []struct {
		result YtdlpUpdate
		err    error
		code   int
		body   string
	}{
		{YtdlpUpdate{Version: "2026.09.30", Previous: "2026.08.19", Updated: true}, nil, http.StatusOK, `"updated":true`},
		{YtdlpUpdate{Version: "2026.09.30"}, nil, http.StatusOK, `"updated":false`},
		{YtdlpUpdate{}, ErrDownloadsRunning, http.StatusConflict, `"code":"downloads_running"`},
		{YtdlpUpdate{}, ErrUpdateBusy, http.StatusConflict, `"code":"update_busy"`},
		{YtdlpUpdate{}, errors.New("checksum mismatch"), http.StatusBadGateway, `"code":"update_failed"`},
	} {
		fake.result, fake.err = test.result, test.err
		got := post()
		if got.Code != test.code || !strings.Contains(got.Body.String(), test.body) {
			t.Errorf("update with %v = %d %s", test.err, got.Code, got.Body.String())
		}
	}
}
