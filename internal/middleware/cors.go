package middleware

import (
	"log/slog"
	"net/http"
)

// CORS returns middleware that allows cross-origin requests from the specified
// origins.
//
// When allowedOrigins is non-empty, only listed origins receive CORS headers
// and the Vary/credentials headers are set for cookie-bearing flows.
//
// When allowedOrigins is empty the behaviour depends on isProduction:
//   - production: no CORS headers are emitted (fail-closed). OPTIONS requests
//     receive a 204 with no allow-origin so the browser rejects cross-origin
//     requests. A startup warning is logged.
//   - non-production: all origins are allowed via the wildcard header (legacy
//     development behaviour).
func CORS(allowedOrigins []string, isProduction bool) func(http.Handler) http.Handler {
	allowAll := len(allowedOrigins) == 0 && !isProduction

	if len(allowedOrigins) == 0 && isProduction {
		slog.Warn("HIVY_CORS_ORIGINS is unset in production — CORS is disabled (fail-closed); set HIVY_CORS_ORIGINS to allow browser clients")
	}

	allowed := make(map[string]bool, len(allowedOrigins))
	for _, o := range allowedOrigins {
		allowed[o] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" && (allowAll || allowed[origin]) {
				if allowAll {
					w.Header().Set("Access-Control-Allow-Origin", "*")
				} else {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Set("Access-Control-Allow-Credentials", "true")
					w.Header().Set("Vary", "Origin")
				}
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Org-ID, X-Hivy-Admin-Secret, Last-Event-ID, Cache-Control")
				w.Header().Set("Access-Control-Max-Age", "300")
			}

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
