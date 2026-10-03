package api

import "net/http"

// contentSecurityPolicy lets the page load only its own files and the thumbnails of YouTube
// (i.ytimg.com), X (pbs.twimg.com), Reddit (external-preview.redd.it, preview.redd.it),
// Instagram (its regional servers under fbcdn.net and cdninstagram.com), and Vimeo
// (i.vimeocdn.com),
// and forbids other sites from framing it: a framed copy would make requests that count
// as same-site, so a disguised click could change downloads or delete files.
const contentSecurityPolicy = "default-src 'self'; script-src 'self'; style-src 'self'; " +
	"img-src 'self' data: https://i.ytimg.com https://pbs.twimg.com https://external-preview.redd.it https://preview.redd.it https://*.fbcdn.net https://*.cdninstagram.com https://i.vimeocdn.com; connect-src 'self'; object-src 'none'; " +
	"base-uri 'none'; form-action 'self'; frame-ancestors 'none'"

// secureHeaders adds browser protections to every response.
func secureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", contentSecurityPolicy)
		h.Set("X-Frame-Options", "DENY") // for browsers without frame-ancestors
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}
