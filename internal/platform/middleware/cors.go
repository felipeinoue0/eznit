package middleware

import (
	"net/http"
	"os"
	"strings"
)

// CORSEnv names the allow-list of browser origins that may call the API from
// another site: a comma-separated list of origins (`https://ui.example.com`),
// or `*` for any. Unset or empty keeps the API same-origin only — the web UI it
// serves itself needs no CORS at all.
const CORSEnv = "CORS_ALLOWED_ORIGINS"

// CORSOriginsFromEnv reads CORSEnv. Each entry is trimmed and loses a trailing
// slash, because a browser sends `Origin` without one and a copied address
// usually has it.
func CORSOriginsFromEnv() []string {
	return ParseOrigins(os.Getenv(CORSEnv))
}

// ParseOrigins splits a CORSEnv value into its origins.
func ParseOrigins(raw string) []string {
	var origins []string
	for _, part := range strings.Split(raw, ",") {
		origin := strings.TrimRight(strings.TrimSpace(part), "/")
		if origin != "" {
			origins = append(origins, origin)
		}
	}
	return origins
}

// CORS answers cross-origin requests from the listed origins.
//
// The session is a bearer token the client sends in a header, never a cookie,
// so the browser has no ambient credential to leak: Allow-Credentials is not
// sent, and a page on an allowed origin can only act with a token it already
// holds. A preflight is answered here, before the session guard, because the
// browser sends it without the Authorization header it is asking about.
//
// With no origins the middleware is a pass-through, which is the default.
func CORS(origins []string) func(http.Handler) http.Handler {
	anyOrigin := false
	allowed := make(map[string]bool, len(origins))
	for _, origin := range origins {
		if origin == "*" {
			anyOrigin = true
		}
		allowed[origin] = true
	}

	return func(next http.Handler) http.Handler {
		if len(allowed) == 0 {
			return next
		}

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			// The answer depends on Origin whether or not it is allowed, so a
			// shared cache must not hand one origin's answer to another.
			w.Header().Add("Vary", "Origin")

			if origin == "" || !(anyOrigin || allowed[origin]) {
				next.ServeHTTP(w, r)
				return
			}

			h := w.Header()
			h.Set("Access-Control-Allow-Origin", origin)
			// The archive and file routes name the download in this header;
			// without it a cross-origin page saves every file as "download".
			h.Set("Access-Control-Expose-Headers", "Content-Disposition")

			if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
				h.Add("Vary", "Access-Control-Request-Method")
				h.Add("Vary", "Access-Control-Request-Headers")
				h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
				h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Accept")
				h.Set("Access-Control-Max-Age", "600")
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
