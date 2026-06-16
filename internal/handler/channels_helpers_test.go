package handler_test

import (
	"bytes"
	"encoding/json"
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

type channelHarness struct {
	db     *gorm.DB
	router *chi.Mux
}

type channelFixture struct {
	org    model.Org
	owner  model.User
	member model.User
	agent  model.Agent
}

type channelCreateOut struct {
	Channel channelOut `json:"channel"`
}

type channelDetailOut struct {
	Channel channelOut `json:"channel"`
	Members []struct {
		UserID string `json:"user_id"`
		Role   string `json:"role"`
	} `json:"members"`
}

type channelListOut struct {
	Data    []channelOut `json:"data"`
	HasMore bool         `json:"has_more"`
}

type channelOut struct {
	ID                   string `json:"id"`
	Name                 string `json:"name"`
	Visibility           string `json:"visibility"`
	DefaultAgentID       string `json:"default_agent_id"`
	Origin               string `json:"origin"`
	ExternalProvider     string `json:"external_provider"`
	ExternalWorkspaceKey string `json:"external_workspace_key"`
	ExternalResourceType string `json:"external_resource_type"`
	ExternalResourceKey  string `json:"external_resource_key"`
	Role                 string `json:"role"`
	MemberCount          int64  `json:"member_count"`
}

func newChannelHarness(t *testing.T) *channelHarness {
	t.Helper()
	db := connectTestDB(t)
	h := handler.NewChannelHandler(db)
	r := chi.NewRouter()
	r.Route("/v1", func(r chi.Router) {
		r.Use(middleware.ResolveOrgFromHeader(db))
		r.Use(middleware.RequireAPIKeyScopeOrJWT("channels"))
		r.Get("/channels", h.List)
		r.Post("/channels", h.Create)
		r.Get("/channels/{id}", h.Get)
		r.Patch("/channels/{id}", h.Update)
		r.Delete("/channels/{id}", h.Archive)
		r.Post("/channels/{id}/join", h.Join)
		r.Put("/channels/{id}/members/{userID}", h.PutMember)
		r.Delete("/channels/{id}/members/{userID}", h.DeleteMember)
	})
	return &channelHarness{db: db, router: r}
}

func (h *channelHarness) seed(t *testing.T) channelFixture {
	t.Helper()
	org := model.Org{Name: "chan-org-" + uuid.NewString()[:8], Active: true}
	if err := h.db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	owner := h.seedUser(t, org.ID, "owner")
	member := h.seedUser(t, org.ID, "member")
	agent := model.Agent{
		OrgID:           &org.ID,
		Name:            "Hivy",
		Description:     ptrString("default"),
		IsDefault:       true,
		SandboxStrategy: "always_on",
		Model:           "deepseek-v4-flash",
		Tools:           model.JSON{},
		McpServers:      model.RawJSON("[]"),
		Skills:          model.JSON{},
		RuntimeConfig:   model.JSON{},
		Permissions:     model.JSON{},
		Resources:       model.JSON{},
		Status:          "active",
	}
	if err := h.db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	t.Cleanup(func() {
		h.db.Where("org_id = ?", org.ID).Delete(&model.Channel{})
		h.db.Where("org_id = ?", org.ID).Delete(&model.Agent{})
		h.db.Where("org_id = ?", org.ID).Delete(&model.OrgMembership{})
		h.db.Where("id IN ?", []uuid.UUID{owner.ID, member.ID}).Delete(&model.User{})
		h.db.Where("id = ?", org.ID).Delete(&model.Org{})
	})
	return channelFixture{org: org, owner: owner, member: member, agent: agent}
}

func (h *channelHarness) seedUser(t *testing.T, orgID uuid.UUID, role string) model.User {
	t.Helper()
	user := model.User{Email: "chan-" + role + "-" + uuid.NewString()[:8] + "@test.com", Name: role}
	if err := h.db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := h.db.Create(&model.OrgMembership{UserID: user.ID, OrgID: orgID, Role: role}).Error; err != nil {
		t.Fatalf("create membership: %v", err)
	}
	return user
}

func (h *channelHarness) doJSON(t *testing.T, method, path string, fx channelFixture, user model.User, body any) *httptest.ResponseRecorder {
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

func decodeChannelCreate(t *testing.T, rr *httptest.ResponseRecorder) channelCreateOut {
	t.Helper()
	var out channelCreateOut
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode channel response: %v\n%s", err, rr.Body.String())
	}
	return out
}

func ptrString(value string) *string {
	return &value
}
