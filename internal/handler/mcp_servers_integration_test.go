package handler_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/auth"
	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/mcpservers"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

type mcpHarness struct {
	db      *gorm.DB
	router  *chi.Mux
	service *mcpservers.Service
	key     *crypto.SymmetricKey
}

type mcpFixture struct {
	org       model.Org
	otherOrg  model.Org
	owner     model.User
	member    model.User
	otherUser model.User
	team      model.Team
	agent     model.Agent
}

func newMCPHarness(t *testing.T, client *http.Client, callbackURL string) *mcpHarness {
	t.Helper()
	db := connectTestDB(t)
	keyBytes := bytes.Repeat([]byte{0x4d}, 32)
	key, err := crypto.NewSymmetricKey(base64.StdEncoding.EncodeToString(keyBytes))
	if err != nil {
		t.Fatalf("new encryption key: %v", err)
	}
	service := mcpservers.NewService(db, key, callbackURL)
	if client != nil {
		service.WithHTTPClient(client)
	}
	h := handler.NewMCPServerHandler(db, service, "")
	router := chi.NewRouter()
	router.Get("/v1/mcp-servers/oauth/callback", h.OAuthCallback)
	router.Get("/v1/mcp-servers/oauth/client-metadata", h.OAuthClientMetadata)
	router.Group(func(r chi.Router) {
		r.Use(middleware.ResolveOrgFromHeader(db))
		h.Mount(r)
	})
	return &mcpHarness{db: db, router: router, service: service, key: key}
}

func (h *mcpHarness) seed(t *testing.T) mcpFixture {
	t.Helper()
	owner := model.User{Email: "mcp-owner-" + uuid.NewString()[:8] + "@test.com", Name: "MCP Owner"}
	member := model.User{Email: "mcp-member-" + uuid.NewString()[:8] + "@test.com", Name: "MCP Member"}
	otherUser := model.User{Email: "mcp-other-" + uuid.NewString()[:8] + "@test.com", Name: "Other"}
	for _, user := range []*model.User{&owner, &member, &otherUser} {
		if err := h.db.WithContext(t.Context()).Create(user).Error; err != nil {
			t.Fatalf("create user: %v", err)
		}
	}
	org := model.Org{Name: "mcp-org-" + uuid.NewString()[:8], Active: true}
	otherOrg := model.Org{Name: "mcp-other-org-" + uuid.NewString()[:8], Active: true}
	if err := h.db.WithContext(t.Context()).Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	if err := h.db.WithContext(t.Context()).Create(&otherOrg).Error; err != nil {
		t.Fatalf("create other org: %v", err)
	}
	memberships := []model.OrgMembership{
		{OrgID: org.ID, UserID: owner.ID, Role: "owner"},
		{OrgID: org.ID, UserID: member.ID, Role: "member"},
		{OrgID: otherOrg.ID, UserID: otherUser.ID, Role: "owner"},
	}
	if err := h.db.WithContext(t.Context()).Create(&memberships).Error; err != nil {
		t.Fatalf("create memberships: %v", err)
	}
	team := model.Team{OrgID: org.ID, Name: "MCP Team", CreatedBy: &owner.ID}
	if err := h.db.WithContext(t.Context()).Create(&team).Error; err != nil {
		t.Fatalf("create team: %v", err)
	}
	teamMembers := []model.TeamMember{
		{OrgID: org.ID, TeamID: team.ID, UserID: owner.ID, Role: "owner"},
		{OrgID: org.ID, TeamID: team.ID, UserID: member.ID, Role: "member"},
	}
	if err := h.db.WithContext(t.Context()).Create(&teamMembers).Error; err != nil {
		t.Fatalf("create team members: %v", err)
	}
	agent := model.Agent{
		OrgID: &org.ID, TeamID: team.ID, Name: "MCP Agent", Model: "test-model",
		Tools: model.JSON{}, McpServers: model.RawJSON("[]"), Skills: model.JSON{},
		RuntimeConfig: model.JSON{}, Permissions: model.JSON{}, Resources: model.JSON{}, Status: "active",
	}
	if err := h.db.WithContext(t.Context()).Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	t.Cleanup(func() {
		_ = h.db.WithContext(t.Context()).Where("id IN ?", []uuid.UUID{org.ID, otherOrg.ID}).Delete(&model.Org{}).Error
		_ = h.db.WithContext(t.Context()).Where("id IN ?", []uuid.UUID{owner.ID, member.ID, otherUser.ID}).Delete(&model.User{}).Error
	})
	return mcpFixture{org: org, otherOrg: otherOrg, owner: owner, member: member, otherUser: otherUser, team: team, agent: agent}
}

func (h *mcpHarness) request(t *testing.T, method, path string, org model.Org, user *model.User, body any) *httptest.ResponseRecorder {
	t.Helper()
	var encoded bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&encoded).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	request := httptest.NewRequest(method, path, &encoded)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Org-ID", org.ID.String())
	if user != nil {
		request = middleware.WithAuthClaims(request, &auth.AuthClaims{UserID: user.ID.String(), OrgID: org.ID.String()})
	}
	response := httptest.NewRecorder()
	h.router.ServeHTTP(response, request)
	return response
}

func responseMCPServerID(t *testing.T, response *httptest.ResponseRecorder) uuid.UUID {
	t.Helper()
	var envelope struct {
		MCPServer struct {
			ID string `json:"id"`
		} `json:"mcp_server"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode MCP server: %v: %s", err, response.Body.String())
	}
	id, err := uuid.Parse(envelope.MCPServer.ID)
	if err != nil {
		t.Fatalf("parse MCP server id %q: %v", envelope.MCPServer.ID, err)
	}
	return id
}
