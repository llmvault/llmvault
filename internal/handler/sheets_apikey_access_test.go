package handler_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

// doSheetsAPIKey issues a request authenticated as an org API key (no user),
// exercising canUseSheetChannel's API-key path.
func doSheetsAPIKey(t *testing.T, h *sheetsHarness, router http.Handler, scopes []string, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	req = middleware.WithOrg(req, &h.org)
	req = middleware.WithAPIKeyClaims(req, &middleware.APIKeyClaims{
		KeyID:  uuid.NewString(),
		OrgID:  h.org.ID.String(),
		Scopes: scopes,
	})
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	return rr
}

// TestSheetsAPIKeyChannelRestriction pins the hardened canUseSheetChannel:
// an org API key is not a team member, so it reaches sheets in channels open
// to any org member but is denied team-scoped channels (404, no existence
// leak). Previously API keys bypassed channel checks entirely.
func TestSheetsAPIKeyChannelRestriction(t *testing.T) {
	h := newSheetsHarness(t)

	t.Run("non-team channel allowed", func(t *testing.T) {
		resp := doSheetsAPIKey(t, h, h.router, []string{"sheets"}, http.MethodGet, "/v1/sheets?channel_id="+h.channel.ID.String())
		if resp.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
		}
	})

	t.Run("external channel allowed", func(t *testing.T) {
		agent := h.createAgent(t, h.org.ID)
		external := model.Channel{
			ID: uuid.New(), OrgID: h.org.ID, Name: "sheets-ext-" + uuid.NewString(),
			DefaultAgentID: agent.ID, Origin: "external", ExternalProvider: "slack",
		}
		if err := h.db.Create(&external).Error; err != nil {
			t.Fatalf("create external channel: %v", err)
		}
		t.Cleanup(func() { h.db.Delete(&model.Channel{}, "id = ?", external.ID) })
		resp := doSheetsAPIKey(t, h, h.router, []string{"sheets"}, http.MethodGet, "/v1/sheets?channel_id="+external.ID.String())
		if resp.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
		}
	})

	t.Run("team-scoped channel denied", func(t *testing.T) {
		agent := h.createAgent(t, h.org.ID)
		team := model.Team{ID: uuid.New(), OrgID: h.org.ID, Name: "sheets-team-" + uuid.NewString()}
		if err := h.db.Create(&team).Error; err != nil {
			t.Fatalf("create team: %v", err)
		}
		teamChannel := model.Channel{
			ID: uuid.New(), OrgID: h.org.ID, Name: "sheets-team-ch-" + uuid.NewString(),
			DefaultAgentID: agent.ID, TeamID: &team.ID,
		}
		if err := h.db.Create(&teamChannel).Error; err != nil {
			t.Fatalf("create team channel: %v", err)
		}
		t.Cleanup(func() {
			h.db.Delete(&model.Channel{}, "id = ?", teamChannel.ID)
			h.db.Delete(&model.Team{}, "id = ?", team.ID)
		})
		resp := doSheetsAPIKey(t, h, h.router, []string{"sheets"}, http.MethodGet, "/v1/sheets?channel_id="+teamChannel.ID.String())
		if resp.Code != http.StatusNotFound {
			t.Fatalf("status=%d body=%s (team channels must be denied to API keys)", resp.Code, resp.Body.String())
		}
	})
}

// TestSheetsAPIKeyScopeGate pins the new "sheets" API-key scope: mirroring
// production (serve_routes_v1.go), the sheet routes sit behind
// RequireAPIKeyScopeOrJWT("sheets").
func TestSheetsAPIKeyScopeGate(t *testing.T) {
	if !model.ValidAPIKeyScopes["sheets"] {
		t.Fatal(`"sheets" must be a valid API key scope`)
	}
	h := newSheetsHarness(t)
	router := chi.NewRouter()
	router.Route("/v1", func(r chi.Router) {
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireAPIKeyScopeOrJWT("sheets"))
			r.Get("/sheets", h.handler.ListSheets)
		})
	})
	path := "/v1/sheets?channel_id=" + h.channel.ID.String()

	t.Run("key without sheets scope is 403", func(t *testing.T) {
		resp := doSheetsAPIKey(t, h, router, []string{"agents"}, http.MethodGet, path)
		if resp.Code != http.StatusForbidden {
			t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
		}
	})

	t.Run("key with sheets scope passes", func(t *testing.T) {
		resp := doSheetsAPIKey(t, h, router, []string{"sheets"}, http.MethodGet, path)
		if resp.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
		}
	})

	t.Run("all scope passes", func(t *testing.T) {
		resp := doSheetsAPIKey(t, h, router, []string{"all"}, http.MethodGet, path)
		if resp.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
		}
	})
}
