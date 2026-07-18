package proxy

import (
	"errors"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/registry"
)

func TestShouldFailover(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		want   bool
	}{
		{name: "provider billing error", status: http.StatusPaymentRequired, want: true},
		{name: "rate limited", status: http.StatusTooManyRequests, want: true},
		{name: "provider unavailable", status: http.StatusServiceUnavailable, want: true},
		{name: "invalid client request", status: http.StatusBadRequest, want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldFailoverStatus(tc.status); got != tc.want {
				t.Fatalf("shouldFailoverStatus() = %v, want %v", got, tc.want)
			}
		})
	}
	if !shouldFailover(nil, errors.New("dial failed")) {
		t.Fatal("transport error must fail over")
	}
}

func TestCandidatesForRoutesPreservesCatalogOrderAndSkipsMissingCredentials(t *testing.T) {
	openRouterID := uuid.New()
	routes := []registry.ModelRoute{
		{ProviderID: "openrouter", ModelID: "deepseek/default"},
		{ProviderID: "provider-a", ModelID: "deepseek/a"},
		{ProviderID: "provider-b", ModelID: "deepseek/b"},
	}
	credentials := []model.Credential{
		{ID: openRouterID, ProviderID: "openrouter"},
		{ID: uuid.New(), ProviderID: "provider-b"},
	}

	candidates := candidatesForRoutes(routes, credentials)
	if len(candidates) != 2 {
		t.Fatalf("candidate count = %d, want 2", len(candidates))
	}
	if candidates[0].ProviderID != "openrouter" || candidates[0].UpstreamID != "deepseek/default" {
		t.Fatalf("primary candidate = %#v", candidates[0])
	}
	if candidates[1].ProviderID != "provider-b" || candidates[1].UpstreamID != "deepseek/b" {
		t.Fatalf("fallback candidate = %#v", candidates[1])
	}
}
