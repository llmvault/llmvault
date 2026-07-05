package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestAutomationCatalogEndpointsServeGlobalFiles(t *testing.T) {
	catalogHandler := NewAutomationCatalogHandler("global/triggers", "global/schedules")
	router := chi.NewRouter()
	router.Get("/v1/catalog/triggers", catalogHandler.ListTriggers)
	router.Get("/v1/catalog/automations", catalogHandler.ListAutomations)

	cases := []struct {
		name    string
		path    string
		kind    string
		wantMin int
	}{
		{
			name:    "triggers",
			path:    "/v1/catalog/triggers",
			kind:    "trigger",
			wantMin: 1,
		},
		{
			name:    "automations",
			path:    "/v1/catalog/automations",
			kind:    "schedule",
			wantMin: 11,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			router.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
			}

			var resp struct {
				Data []struct {
					Kind        string `json:"kind"`
					Slug        string `json:"slug"`
					Category    string `json:"category"`
					Enabled     bool   `json:"enabled"`
					Integration struct {
						Provider string `json:"provider"`
					} `json:"integration"`
					Instructions string `json:"instructions"`
					Trigger      *struct {
						Key      string `json:"key"`
						Defaults struct {
							Value        string `json:"value"`
							Instructions string `json:"instructions"`
						} `json:"defaults"`
					} `json:"trigger,omitempty"`
					Schedule *struct {
						Kind     string `json:"kind"`
						Cron     string `json:"cron"`
						Timezone string `json:"timezone"`
					} `json:"schedule,omitempty"`
					Install struct {
						DefaultAgent   string `json:"default_agent"`
						DefaultChannel string `json:"default_channel"`
					} `json:"install"`
				} `json:"data"`
			}
			if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if len(resp.Data) < tc.wantMin {
				t.Fatalf("items = %d, want at least %d", len(resp.Data), tc.wantMin)
			}
			if tc.kind == "trigger" {
				wantTriggers := map[string]struct {
					provider string
					key      string
				}{
					"slack-reaction":       {provider: "slack", key: "reaction_added"},
					"github-issue-mention": {provider: "github-app", key: "issue_mention"},
					"github-pr-mention":    {provider: "github-app", key: "pr_mention"},
				}
				if len(resp.Data) != len(wantTriggers) {
					t.Fatalf("trigger items = %d, want %d", len(resp.Data), len(wantTriggers))
				}
				for _, item := range resp.Data {
					want, ok := wantTriggers[item.Slug]
					if !ok {
						t.Fatalf("unexpected trigger template %q", item.Slug)
					}
					if item.Integration.Provider != want.provider || item.Trigger == nil || item.Trigger.Key != want.key {
						t.Fatalf("trigger item %q = provider %q key %v, want provider %q key %q",
							item.Slug, item.Integration.Provider, item.Trigger, want.provider, want.key)
					}
					if item.Trigger.Defaults.Instructions == "" {
						t.Fatalf("trigger item %q missing default instructions", item.Slug)
					}
				}
			}
			for _, item := range resp.Data {
				if item.Kind != tc.kind {
					t.Fatalf("item %q kind = %q, want %q", item.Slug, item.Kind, tc.kind)
				}
				if item.Slug == "" || item.Category == "" || item.Integration.Provider == "" {
					t.Fatalf("item %q missing required catalog metadata", item.Slug)
				}
				if !item.Enabled || item.Instructions == "" {
					t.Fatalf("item %q should be enabled with instructions", item.Slug)
				}
				if tc.kind == "schedule" {
					if item.Install.DefaultAgent == "" || item.Install.DefaultChannel == "" {
						t.Fatalf("schedule item %q missing install defaults", item.Slug)
					}
					if item.Schedule == nil || item.Schedule.Cron == "" {
						t.Fatalf("schedule item %q missing schedule cron", item.Slug)
					}
				}
			}
		})
	}
}
