package phone

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net"
	"net/http"
	"strings"

	"rsc.io/qr"
)

// phoneRoutes are the API routes a paired phone may use. Everything else under /api/ is
// refused, including routes added later, until they are listed here.
var phoneRoutes = func() *http.ServeMux {
	mux := http.NewServeMux()
	for _, pattern := range []string{
		"GET /api/system/health",
		"GET /api/system/version",
		"POST /api/system/resume",
		"GET /api/settings",
		"GET /api/inspect",
		"GET /api/search",
		"GET /api/playlist",
		"POST /api/playlist/jobs",
		"GET /api/jobs",
		"POST /api/jobs",
		"GET /api/jobs/{id}",
		"GET /api/jobs/{id}/events",
		"GET /api/jobs/{id}/file",
		"POST /api/jobs/{id}/cancel",
		"POST /api/jobs/{id}/pause",
		"POST /api/jobs/{id}/resume",
		"POST /api/jobs/{id}/retry",
		"POST /api/jobs/{id}/top",
		"DELETE /api/jobs/{id}",
		"POST /api/history/clear",
		"GET /api/library/files",
		"GET /api/watches",
		"POST /api/watches",
		"PUT /api/watches/{id}",
		"DELETE /api/watches/{id}",
		"POST /api/watches/{id}/check",
	} {
		mux.Handle(pattern, http.NotFoundHandler())
	}
	return mux
}()

// allowed reports whether a paired phone may make this request. Files on the computer are
// only ever deleted from the computer.
func allowed(r *http.Request) bool {
	if !strings.HasPrefix(r.URL.Path, "/api/") {
		return r.Method == http.MethodGet || r.Method == http.MethodHead // the page itself
	}
	query := r.URL.Query()
	if query.Get("delete_file") != "" || query.Get("delete_files") != "" {
		return false
	}
	_, pattern := phoneRoutes.Handler(r)
	return pattern != ""
}

// ServeHTTP is the phone server: local-network visitors only, paired phones only.
func (s *Service) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !fromLocalNetwork(r.RemoteAddr) || !addressHost(r.Host) {
		http.Error(w, "YTGrab only answers phones on the same local network.", http.StatusForbidden)
		return
	}
	if r.URL.Path == "/pair" {
		s.servePair(w, r)
		return
	}
	cookie, _ := r.Cookie(CookieName)
	if cookie == nil || !s.paired(cookie.Value) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			writeError(w, http.StatusUnauthorized, "not_paired", "Pair this phone first: scan the code in YTGrab's settings on your computer.")
			return
		}
		writePage(w, http.StatusUnauthorized, "Pair this phone", "On your computer, open YTGrab, go to <b>Settings → Phone</b>, and scan the code shown there with this phone's camera.")
		return
	}
	if !allowed(r) {
		writeError(w, http.StatusForbidden, "computer_only", "This can only be done in YTGrab on your computer.")
		return
	}
	s.mu.Lock()
	app := s.app
	s.mu.Unlock()
	app.ServeHTTP(w, r)
}

func (s *Service) servePair(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	key, err := s.pair(r.Context(), r.URL.Query().Get("code"), r.UserAgent())
	switch {
	case errors.Is(err, ErrTooMany):
		writePage(w, http.StatusForbidden, "Too many phones", fmt.Sprintf("YTGrab can pair up to %d phones. Remove one in <b>Settings → Phone</b> on your computer, then scan a new code.", MaxDevices))
		return
	case err != nil:
		writePage(w, http.StatusForbidden, "This code has expired", "Each code works once, for 10 minutes. On your computer, open <b>Settings → Phone</b> in YTGrab and scan the new code.")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    key,
		Path:     "/",
		MaxAge:   int(keyLifetime.Seconds()),
		HttpOnly: true,
		// Lax, not Strict: the first visit comes from the camera app, and must keep the key.
		// Changes need a same-origin request anyway (the API checks Origin).
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// fromLocalNetwork reports whether a visitor is on a private network (or this computer):
// never the internet, even when a router forwards the port.
func fromLocalNetwork(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && (ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast())
}

// addressHost reports whether the page was opened by IP address, as the pairing link does.
// A web page that renames itself to this computer's address (DNS rebinding) has a name.
func addressHost(host string) bool {
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	return net.ParseIP(strings.Trim(host, "[]")) != nil
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": code, "message": message}})
}

// writePage shows a short message page; body is trusted HTML.
func writePage(w http.ResponseWriter, status int, title, body string) {
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	h.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; frame-ancestors 'none'")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "no-referrer")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>YTGrab: %s</title>
<style>body{font:16px/1.5 system-ui,sans-serif;margin:0;padding:48px 20px;background:#f6f7f9;color:#1d2330}main{max-width:420px;margin:auto;background:#fff;border-radius:14px;padding:24px;box-shadow:0 1px 3px #0002}h1{font-size:20px;margin:0 0 8px}@media (prefers-color-scheme:dark){body{background:#12151b;color:#e8ebf1}main{background:#1c2029}}</style>
</head><body><main><h1>%s</h1><p>%s</p></main></body></html>`, html.EscapeString(title), html.EscapeString(title), body)
}

// qrDataURL draws text as a QR code, an SVG image in a data: URL.
func qrDataURL(text string) (string, error) {
	code, err := qr.Encode(text, qr.M)
	if err != nil {
		return "", err
	}
	const quiet = 4 // the white border scanners need
	size := code.Size + 2*quiet
	var path strings.Builder
	for y := 0; y < code.Size; y++ {
		for x := 0; x < code.Size; x++ {
			if code.Black(x, y) {
				fmt.Fprintf(&path, "M%d %dh1v1h-1z", x+quiet, y+quiet)
			}
		}
	}
	svg := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" shape-rendering="crispEdges"><rect width="%d" height="%d" fill="#fff"/><path fill="#000" d="%s"/></svg>`, size, size, size, size, path.String())
	return "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(svg)), nil
}
