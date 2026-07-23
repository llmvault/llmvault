package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestMountOrgMemberLifecycleRoutesPreservesOrgCurrentHandlers(t *testing.T) {
	t.Parallel()

	r := chi.NewRouter()
	r.Get("/orgs/current", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	r.Patch("/orgs/current", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mountOrgMemberLifecycleRoutes(r, nil)

	for _, method := range []string{http.MethodGet, http.MethodPatch} {
		t.Run(method, func(t *testing.T) {
			t.Parallel()

			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(method, "/orgs/current", nil)
			r.ServeHTTP(recorder, request)

			if recorder.Code != http.StatusOK {
				t.Fatalf("%s /orgs/current: expected 200, got %d: %s", method, recorder.Code, recorder.Body.String())
			}
		})
	}
}
