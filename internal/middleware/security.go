package middleware

import "net/http"

// SecurityHeaders applies robust, modern security headers to all outgoing responses.
func (m *Middleware) SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 1. Prevent MIME-sniffing: forces the browser to respect the declared Content-Type
		w.Header().Set("X-Content-Type-Options", "nosniff")

		// 2. Prevent Clickjacking: forbids embedding the API responses in <frame>, <iframe>, or <embed>
		w.Header().Set("X-Frame-Options", "DENY")

		// 3. Referrer Control: restricts sensitive path info from being sent in HTTP Referer headers
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

		// 4. Content Security Policy (Strict API Defaults): prevents framing ancestors and default assets
		w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'; sandbox")

		// 5. HSTS: instructs browsers to only connect over HTTPS for the next 2 years
		w.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains; preload")

		next.ServeHTTP(w, r)
	})
}
