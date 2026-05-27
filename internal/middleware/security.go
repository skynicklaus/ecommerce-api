package middleware

import (
	"net/http"
	"strings"
)

// SecurityHeaders applies robust, modern security headers to all outgoing responses.
func (m *Middleware) SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 1. Prevent MIME-sniffing: forces the browser to respect the declared Content-Type
		w.Header().Set("X-Content-Type-Options", "nosniff")

		// 2. Prevent Clickjacking: forbids embedding the API responses in <frame>, <iframe>, or <embed>
		w.Header().Set("X-Frame-Options", "DENY")

		// 3. Referrer Control: restricts sensitive path info from being sent in HTTP Referer headers
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

		// 4. Content Security Policy.
		// Swagger UI needs its bundled scripts/styles and a small inline bootstrap script.
		// Keep the strict API default everywhere else.
		if strings.HasPrefix(r.URL.Path, "/swagger/") {
			w.Header().Set(
				"Content-Security-Policy",
				"default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self' data:; connect-src 'self'; frame-ancestors 'none'",
			)
		} else {
			w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'; sandbox")
		}

		// 5. HSTS: instructs browsers to only connect over HTTPS for the next 2 years
		w.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains; preload")

		next.ServeHTTP(w, r)
	})
}
