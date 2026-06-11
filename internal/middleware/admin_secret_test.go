package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequireAdminSecret(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	tests := []struct {
		name       string
		configured string
		provided   string
		wantStatus int
	}{
		{
			name:       "allows matching secret",
			configured: "secret-value",
			provided:   "secret-value",
			wantStatus: http.StatusNoContent,
		},
		{
			name:       "rejects missing header",
			configured: "secret-value",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "rejects mismatched secret",
			configured: "secret-value",
			provided:   "wrong-value",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "rejects empty configured secret",
			configured: "",
			provided:   "secret-value",
			wantStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/v1/admin/integrations", nil)
			if tt.provided != "" {
				req.Header.Set(AdminSecretHeader, tt.provided)
			}
			rec := httptest.NewRecorder()

			RequireAdminSecret(tt.configured)(next).ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}
