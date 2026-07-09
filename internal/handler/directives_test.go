package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/auth"
	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

type memoryControlHarness struct {
	db     *gorm.DB
	router *chi.Mux
}

// newMemoryControlHarness mounts the directives + observations routes behind
// the same auth stack the memories routes use.
func newMemoryControlHarness(t *testing.T) *memoryControlHarness {
	t.Helper()
	db := connectTestDB(t)
	directiveHandler := handler.NewDirectiveHandler(db)
	observationHandler := handler.NewObservationHandler(db)
	r := chi.NewRouter()
	r.Route("/v1", func(r chi.Router) {
		r.Use(middleware.ResolveOrgFromHeader(db))
		r.Use(middleware.RequireAPIKeyScopeOrJWT("memories"))
		r.Get("/directives", directiveHandler.List)
		r.Post("/directives", directiveHandler.Create)
		r.Patch("/directives/{id}", directiveHandler.Update)
		r.Delete("/directives/{id}", directiveHandler.Delete)
		r.Get("/observations", observationHandler.List)
		r.Post("/observations/{id}/confirm", observationHandler.Confirm)
		r.Post("/observations/{id}/correct", observationHandler.Correct)
		r.Post("/observations/{id}/pin", observationHandler.Pin)
		r.Delete("/observations/{id}", observationHandler.Delete)
	})
	return &memoryControlHarness{db: db, router: r}
}

func (h *memoryControlHarness) doJSON(t *testing.T, method, path string, fx channelFixture, user model.User, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-ID", fx.org.ID.String())
	req = middleware.WithAuthClaims(req, &auth.AuthClaims{
		UserID: user.ID.String(),
		OrgID:  fx.org.ID.String(),
	})
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	return rr
}

func (h *memoryControlHarness) seed(t *testing.T) (channelFixture, model.Channel) {
	t.Helper()
	fx := (&channelHarness{db: h.db}).seed(t)
	channel := model.Channel{
		OrgID:          fx.org.ID,
		Name:           "directives-" + uuid.NewString()[:8],
		Category:       "engineering",
		TeamID:         fx.agent.TeamID,
		DefaultAgentID: fx.agent.ID,
	}
	if err := h.db.Create(&channel).Error; err != nil {
		t.Fatalf("create channel: %v", err)
	}
	t.Cleanup(func() {
		h.db.Where("org_id = ?", fx.org.ID).Delete(&model.AgentDirective{})
	})
	return fx, channel
}

type directiveOut struct {
	ID              string  `json:"id"`
	ChannelID       *string `json:"channel_id"`
	Content         string  `json:"content"`
	Source          string  `json:"source"`
	Active          bool    `json:"active"`
	CreatedByUserID *string `json:"created_by_user_id"`
	CreatedAt       string  `json:"created_at"`
}

func decodeDirective(t *testing.T, rr *httptest.ResponseRecorder) directiveOut {
	t.Helper()
	var out struct {
		Directive directiveOut `json:"directive"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode directive: %v\n%s", err, rr.Body.String())
	}
	return out.Directive
}

func TestIntegration_DirectivesCRUD(t *testing.T) {
	h := newMemoryControlHarness(t)
	fx, channel := h.seed(t)

	// Members cannot create directives.
	forbidden := h.doJSON(t, http.MethodPost, "/v1/directives", fx, fx.member, map[string]any{
		"content": "never do this",
	})
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("member create status=%d body=%s", forbidden.Code, forbidden.Body.String())
	}

	// Empty content is rejected.
	empty := h.doJSON(t, http.MethodPost, "/v1/directives", fx, fx.owner, map[string]any{"content": "  "})
	if empty.Code != http.StatusBadRequest {
		t.Fatalf("empty content status=%d body=%s", empty.Code, empty.Body.String())
	}

	orgWide := h.doJSON(t, http.MethodPost, "/v1/directives", fx, fx.owner, map[string]any{
		"content": "Always use UTC timestamps.",
	})
	if orgWide.Code != http.StatusCreated {
		t.Fatalf("org-wide create status=%d body=%s", orgWide.Code, orgWide.Body.String())
	}
	orgDirective := decodeDirective(t, orgWide)
	if orgDirective.ChannelID != nil || orgDirective.Source != "user-pinned" || !orgDirective.Active {
		t.Fatalf("org-wide directive=%+v", orgDirective)
	}
	if orgDirective.CreatedByUserID == nil || *orgDirective.CreatedByUserID != fx.owner.ID.String() {
		t.Fatalf("created_by_user_id=%v, want %s", orgDirective.CreatedByUserID, fx.owner.ID)
	}

	scoped := h.doJSON(t, http.MethodPost, "/v1/directives", fx, fx.owner, map[string]any{
		"channel_id": channel.ID.String(),
		"content":    "Deploys only on weekdays.",
	})
	if scoped.Code != http.StatusCreated {
		t.Fatalf("channel create status=%d body=%s", scoped.Code, scoped.Body.String())
	}
	channelDirective := decodeDirective(t, scoped)
	if channelDirective.ChannelID == nil || *channelDirective.ChannelID != channel.ID.String() {
		t.Fatalf("channel directive channel_id=%v", channelDirective.ChannelID)
	}

	// Unknown channel rejected.
	badChannel := h.doJSON(t, http.MethodPost, "/v1/directives", fx, fx.owner, map[string]any{
		"channel_id": uuid.NewString(),
		"content":    "orphan",
	})
	if badChannel.Code != http.StatusNotFound {
		t.Fatalf("bad channel status=%d body=%s", badChannel.Code, badChannel.Body.String())
	}

	// Channel filter returns channel + org-wide directives.
	list := h.doJSON(t, http.MethodGet, "/v1/directives?channel_id="+channel.ID.String(), fx, fx.owner, nil)
	if list.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", list.Code, list.Body.String())
	}
	var listOut struct {
		Data []directiveOut `json:"data"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &listOut); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(listOut.Data) != 2 {
		t.Fatalf("channel list=%d directives, want 2 (channel + org-wide)", len(listOut.Data))
	}

	// "org" filter returns only org-wide.
	orgList := h.doJSON(t, http.MethodGet, "/v1/directives?channel_id=org", fx, fx.owner, nil)
	if err := json.Unmarshal(orgList.Body.Bytes(), &listOut); err != nil {
		t.Fatalf("decode org list: %v", err)
	}
	if len(listOut.Data) != 1 || listOut.Data[0].ID != orgDirective.ID {
		t.Fatalf("org list=%+v, want only the org-wide directive", listOut.Data)
	}

	// Deactivate; content is immutable, so a content field in the PATCH body
	// is ignored (delete + re-add is the only way to change a rule's text).
	update := h.doJSON(t, http.MethodPatch, "/v1/directives/"+channelDirective.ID, fx, fx.owner, map[string]any{
		"content": "Deploys only on weekdays before 16:00.",
		"active":  false,
	})
	if update.Code != http.StatusOK {
		t.Fatalf("update status=%d body=%s", update.Code, update.Body.String())
	}
	updated := decodeDirective(t, update)
	if updated.Content != "Deploys only on weekdays." || updated.Active {
		t.Fatalf("updated directive=%+v, want original content and active=false", updated)
	}
	var stored model.AgentDirective
	if err := h.db.Where("id = ?", channelDirective.ID).First(&stored).Error; err != nil {
		t.Fatalf("load directive: %v", err)
	}
	if stored.Content != "Deploys only on weekdays." || stored.Active {
		t.Fatalf("stored directive content=%q active=%v, want immutable content and active=false", stored.Content, stored.Active)
	}

	// Reactivate.
	reactivate := h.doJSON(t, http.MethodPatch, "/v1/directives/"+channelDirective.ID, fx, fx.owner, map[string]any{
		"active": true,
	})
	if reactivate.Code != http.StatusOK {
		t.Fatalf("reactivate status=%d body=%s", reactivate.Code, reactivate.Body.String())
	}
	if reactivated := decodeDirective(t, reactivate); !reactivated.Active {
		t.Fatalf("reactivated directive=%+v, want active=true", reactivated)
	}

	// Delete is a soft delete: the row survives as immutable history (as-of
	// temporal audit) but leaves every read path.
	del := h.doJSON(t, http.MethodDelete, "/v1/directives/"+channelDirective.ID, fx, fx.owner, nil)
	if del.Code != http.StatusOK {
		t.Fatalf("delete status=%d body=%s", del.Code, del.Body.String())
	}
	var deleted model.AgentDirective
	if err := h.db.Where("id = ?", channelDirective.ID).First(&deleted).Error; err != nil {
		t.Fatalf("soft-deleted row must survive: %v", err)
	}
	if deleted.DeletedAt == nil || deleted.Content != "Deploys only on weekdays." {
		t.Fatalf("soft-deleted directive deleted_at=%v content=%q, want timestamp + preserved content", deleted.DeletedAt, deleted.Content)
	}

	// Deleted rules leave the list and cannot be mutated back to life.
	afterDelete := h.doJSON(t, http.MethodGet, "/v1/directives?channel_id="+channel.ID.String(), fx, fx.owner, nil)
	if err := json.Unmarshal(afterDelete.Body.Bytes(), &listOut); err != nil {
		t.Fatalf("decode list after delete: %v", err)
	}
	for _, directive := range listOut.Data {
		if directive.ID == channelDirective.ID {
			t.Fatalf("soft-deleted directive still listed: %+v", directive)
		}
	}
	resurrect := h.doJSON(t, http.MethodPatch, "/v1/directives/"+channelDirective.ID, fx, fx.owner, map[string]any{
		"active": true,
	})
	if resurrect.Code != http.StatusNotFound {
		t.Fatalf("patch on deleted directive status=%d, want 404", resurrect.Code)
	}
}
