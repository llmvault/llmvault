package middleware

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

const AdminSecretHeader = "X-Hivy-Admin-Secret"

// RequireAdminSecret validates the shared admin secret before serving admin APIs.
func RequireAdminSecret(secret string) func(http.Handler) http.Handler {
	expected := strings.TrimSpace(secret)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			provided := strings.TrimSpace(r.Header.Get(AdminSecretHeader))
			if expected == "" || provided == "" || subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) != 1 {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid admin secret"})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
