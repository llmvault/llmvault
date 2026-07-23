package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/auth"
	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/registry"
)

type agentInboxTestResponse struct {
	Available    bool   `json:"available"`
	Email        string `json:"email"`
	MessageCount int64  `json:"message_count"`
}

func TestAgentInboxProvisionAndGet(t *testing.T) {
	db := connectTestDB(t)
	org := createTestOrg(t, db)
	member := seedOrgUser(t, db, org.ID, "member")
	team := seedTeam(t, db, org.ID, "inbox")
	seedTeamMember(t, db, org.ID, team.ID, member.ID)
	agent := seedTeamAgent(t, db, org.ID, team.ID)
	agent.Name = "Inbox Agent"
	if err := db.Model(&agent).Update("name", agent.Name).Error; err != nil {
		t.Fatalf("name agent: %v", err)
	}

	h := handler.NewAgentHandler(
		db,
		nil,
		agentruntime.CompileDeps{Cfg: &config.Config{AgentInboxDomain: "agents.example.test"}},
		registry.Global(),
	)
	router := chi.NewRouter()
	router.Get("/v1/agents/{id}/inbox", h.GetInbox)
	router.Post("/v1/agents/{id}/inbox", h.ProvisionInbox)

	before := performAgentInboxRequest(t, router, org, member, http.MethodGet, agent.ID)
	if before.Code != http.StatusOK {
		t.Fatalf("GET before provisioning status = %d, body = %s", before.Code, before.Body.String())
	}
	beforeBody := decodeAgentInboxResponse(t, before)
	if beforeBody.Available || beforeBody.Email != "" || beforeBody.MessageCount != 0 {
		t.Fatalf("GET before provisioning = %#v, want unavailable inbox", beforeBody)
	}
	versionBefore, err := agentruntime.MCPConfigVersion(t.Context(), db, org.ID)
	if err != nil {
		t.Fatalf("load MCP config version before provisioning: %v", err)
	}

	created := performAgentInboxRequest(t, router, org, member, http.MethodPost, agent.ID)
	if created.Code != http.StatusCreated {
		t.Fatalf("POST status = %d, body = %s", created.Code, created.Body.String())
	}
	createdBody := decodeAgentInboxResponse(t, created)
	if !createdBody.Available || !strings.HasPrefix(createdBody.Email, "inbox-agent-") ||
		!strings.HasSuffix(createdBody.Email, "@agents.example.test") {
		t.Fatalf("POST response = %#v, want provisioned inbox address", createdBody)
	}
	versionAfter, err := agentruntime.MCPConfigVersion(t.Context(), db, org.ID)
	if err != nil {
		t.Fatalf("load MCP config version after provisioning: %v", err)
	}
	if versionAfter != versionBefore+1 {
		t.Fatalf("MCP config version = %d, want %d after inbox provisioning", versionAfter, versionBefore+1)
	}
	var provisionedAgent model.Agent
	if err := db.First(&provisionedAgent, "id = ?", agent.ID).Error; err != nil {
		t.Fatalf("reload provisioned agent: %v", err)
	}
	effectiveFilter := agentruntime.ResolveAgentMCPToolFilter(t.Context(), db, &provisionedAgent)
	for _, id := range model.AgentEmailMCPToolIDs {
		if !containsAgentInboxTool(effectiveFilter.Allow, id) {
			t.Fatalf("effective MCP allow = %#v, want inbox-derived tool %q", effectiveFilter.Allow, id)
		}
	}

	repeated := performAgentInboxRequest(t, router, org, member, http.MethodPost, agent.ID)
	if repeated.Code != http.StatusOK {
		t.Fatalf("repeated POST status = %d, body = %s", repeated.Code, repeated.Body.String())
	}
	repeatedBody := decodeAgentInboxResponse(t, repeated)
	if repeatedBody.Email != createdBody.Email {
		t.Fatalf("repeated POST email = %q, want %q", repeatedBody.Email, createdBody.Email)
	}
	versionAfterRepeat, err := agentruntime.MCPConfigVersion(t.Context(), db, org.ID)
	if err != nil {
		t.Fatalf("load MCP config version after repeated provisioning: %v", err)
	}
	if versionAfterRepeat != versionAfter {
		t.Fatalf("repeated provisioning changed MCP config version from %d to %d", versionAfter, versionAfterRepeat)
	}

	thread := model.AgentEmailThread{
		OrgID:         org.ID,
		AgentID:       agent.ID,
		ReplyToken:    uuid.NewString(),
		LastMessageAt: time.Now().UTC(),
	}
	if err := db.Create(&thread).Error; err != nil {
		t.Fatalf("create inbox thread: %v", err)
	}
	for _, direction := range []string{
		model.AgentEmailDirectionInbound,
		model.AgentEmailDirectionInbound,
		model.AgentEmailDirectionOutbound,
	} {
		message := model.AgentEmailMessage{
			OrgID:       org.ID,
			AgentID:     agent.ID,
			ThreadID:    thread.ID,
			Direction:   direction,
			ProviderAt:  time.Now().UTC(),
			References:  model.RawJSON("[]"),
			ToAddresses: model.RawJSON("[]"),
			CCAddresses: model.RawJSON("[]"),
			Headers:     model.RawJSON("{}"),
		}
		if err := db.Create(&message).Error; err != nil {
			t.Fatalf("create %s message: %v", direction, err)
		}
	}

	after := performAgentInboxRequest(t, router, org, member, http.MethodGet, agent.ID)
	if after.Code != http.StatusOK {
		t.Fatalf("GET after messages status = %d, body = %s", after.Code, after.Body.String())
	}
	afterBody := decodeAgentInboxResponse(t, after)
	if afterBody.MessageCount != 2 {
		t.Fatalf("message_count = %d, want 2 inbound messages", afterBody.MessageCount)
	}
}

func containsAgentInboxTool(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func performAgentInboxRequest(
	t *testing.T,
	router http.Handler,
	org model.Org,
	user model.User,
	method string,
	agentID uuid.UUID,
) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, "/v1/agents/"+agentID.String()+"/inbox", nil)
	request = middleware.WithOrg(request, &org)
	request = middleware.WithAuthClaims(request, &auth.AuthClaims{
		UserID: user.ID.String(),
		OrgID:  org.ID.String(),
	})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func decodeAgentInboxResponse(t *testing.T, response *httptest.ResponseRecorder) agentInboxTestResponse {
	t.Helper()
	var body agentInboxTestResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode inbox response: %v", err)
	}
	return body
}
