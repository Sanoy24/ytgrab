package app

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// identityHeader marks responses from YTGrab, so a second launch can recognize a running
// copy on the port.
const identityHeader = "X-YTGrab-Version"

// AlreadyRunningError reports that YTGrab is already serving at URL.
type AlreadyRunningError struct{ URL string }

func (err *AlreadyRunningError) Error() string {
	return "YTGrab is already running at " + err.URL
}

// runningYTGrab reports whether the server at address is YTGrab.
func runningYTGrab(address string) bool {
	client := &http.Client{Timeout: 2 * time.Second}
	response, err := client.Get("http://" + address + "/")
	if err != nil {
		return false
	}
	response.Body.Close()
	return response.Header.Get(identityHeader) != ""
}

// lockDataDir holds an exclusive lock on the data folder for as long as the app runs, so
// two copies never share one job database. The lock ends if the process dies.
func lockDataDir(dir string) (func(), error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(filepath.Join(dir, "ytgrab.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := lockFile(file); err != nil {
		file.Close()
		return nil, errors.New("another YTGrab is using the data folder " + dir)
	}
	return func() { file.Close() }, nil
}

// identify adds YTGrab's identifying header to every response.
func identify(version string, next http.Handler) http.Handler {
	if version == "" {
		version = "dev"
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(identityHeader, version)
		next.ServeHTTP(w, r)
	})
}

// OpenBrowser opens url (YTGrab's own loopback address) in the default browser.
func OpenBrowser(url string) error { return openBrowser(url) }

func listenError(address string, err error) error {
	return fmt.Errorf("listen on %s: %w. Another program is using this port; start YTGrab with YTGRAB_LISTEN_ADDR=127.0.0.1:8788 to use a different one", address, err)
}
