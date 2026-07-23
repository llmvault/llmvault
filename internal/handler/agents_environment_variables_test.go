package handler_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/registry"
)

func TestAgentEnvironmentVariablesListDisableAndEnable(t *testing.T) {
	db := connectTestDB(t)
	org := createTestOrg(t, db)
	user := createTestUser(t, db, fmt.Sprintf("agent-env-%s@example.com", uuid.NewString()[:8]))
	teamID := firstTeamID(t, db, org.ID)
	for _, row := range []any{
		&model.OrgMembership{OrgID: org.ID, UserID: user.ID, Role: "member"},
		&model.TeamMember{OrgID: org.ID, TeamID: teamID, UserID: user.ID, Role: "member"},
	} {
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("create access row: %v", err)
		}
	}
	agent := createSandboxConfigTestAgent(t, db, org.ID, model.SandboxImageDefault, "small")
	vars := []model.TeamEnvVar{
		{OrgID: org.ID, TeamID: teamID, Name: "ANALYTICS_TOKEN", EncryptedValue: []byte("ciphertext"), Description: "Queries analytics data"},
		{OrgID: org.ID, TeamID: teamID, Name: "CRM_TOKEN", EncryptedValue: []byte("ciphertext"), Description: "Queries customer records"},
	}
	if err := db.Create(&vars).Error; err != nil {
		t.Fatalf("create team environment variables: %v", err)
	}

	h := handler.NewAgentHandler(db, nil, agentruntime.CompileDeps{}, registry.Global())
	router := chi.NewRouter()
	router.Get("/v1/agents/{id}/environment-variables", h.ListEnvironmentVariables)
	router.Patch("/v1/agents/{id}/environment-variables/{name}", h.UpdateEnvironmentVariableAccess)

	list := serveAgentEnvironmentVariables(t, router, &org, &user, http.MethodGet,
		"/v1/agents/"+agent.ID.String()+"/environment-variables", nil)
	if list.Code != http.StatusOK {
		t.Fatalf("initial list status = %d, body = %s", list.Code, list.Body.String())
	}
	var initial struct {
		Data []struct {
			Name    string `json:"name"`
			Enabled bool   `json:"enabled"`
		} `json:"data"`
	}
	if err := json.NewDecoder(list.Body).Decode(&initial); err != nil {
		t.Fatalf("decode initial list: %v", err)
	}
	if len(initial.Data) != 2 || initial.Data[0].Name != "ANALYTICS_TOKEN" || !initial.Data[0].Enabled || !initial.Data[1].Enabled {
		t.Fatalf("initial variables = %#v, want two enabled variables in name order", initial.Data)
	}

	disable := serveAgentEnvironmentVariables(t, router, &org, &user, http.MethodPatch,
		"/v1/agents/"+agent.ID.String()+"/environment-variables/ANALYTICS_TOKEN",
		bytes.NewBufferString(`{"enabled":false}`))
	if disable.Code != http.StatusOK {
		t.Fatalf("disable status = %d, body = %s", disable.Code, disable.Body.String())
	}
	var disabled struct {
		EnvironmentVariable struct {
			Name    string `json:"name"`
			Enabled bool   `json:"enabled"`
		} `json:"environment_variable"`
	}
	if err := json.NewDecoder(disable.Body).Decode(&disabled); err != nil {
		t.Fatalf("decode disable response: %v", err)
	}
	if disabled.EnvironmentVariable.Name != "ANALYTICS_TOKEN" || disabled.EnvironmentVariable.Enabled {
		t.Fatalf("disable response = %#v", disabled.EnvironmentVariable)
	}

	var denyCount int64
	if err := db.Model(&model.AgentTeamEnvVarDeny{}).
		Where("org_id = ? AND agent_id = ?", org.ID, agent.ID).
		Count(&denyCount).Error; err != nil {
		t.Fatalf("count denies: %v", err)
	}
	if denyCount != 1 {
		t.Fatalf("deny count = %d, want 1", denyCount)
	}

	enable := serveAgentEnvironmentVariables(t, router, &org, &user, http.MethodPatch,
		"/v1/agents/"+agent.ID.String()+"/environment-variables/ANALYTICS_TOKEN",
		bytes.NewBufferString(`{"enabled":true}`))
	if enable.Code != http.StatusOK {
		t.Fatalf("enable status = %d, body = %s", enable.Code, enable.Body.String())
	}
	if err := db.Model(&model.AgentTeamEnvVarDeny{}).
		Where("org_id = ? AND agent_id = ?", org.ID, agent.ID).
		Count(&denyCount).Error; err != nil {
		t.Fatalf("count denies after enable: %v", err)
	}
	if denyCount != 0 {
		t.Fatalf("deny count after enable = %d, want 0", denyCount)
	}
}

func TestAgentEnvironmentVariablesHideAgentOutsideActorsTeam(t *testing.T) {
	db := connectTestDB(t)
	org := createTestOrg(t, db)
	user := createTestUser(t, db, fmt.Sprintf("agent-env-hidden-%s@example.com", uuid.NewString()[:8]))
	agent := createSandboxConfigTestAgent(t, db, org.ID, model.SandboxImageDefault, "small")
	otherTeam := model.Team{ID: uuid.New(), OrgID: org.ID, Name: "other-" + uuid.NewString()[:8]}
	for _, row := range []any{
		&model.OrgMembership{OrgID: org.ID, UserID: user.ID, Role: "member"},
		&otherTeam,
		&model.TeamMember{OrgID: org.ID, TeamID: otherTeam.ID, UserID: user.ID, Role: "member"},
	} {
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("create access row: %v", err)
		}
	}

	h := handler.NewAgentHandler(db, nil, agentruntime.CompileDeps{}, registry.Global())
	router := chi.NewRouter()
	router.Get("/v1/agents/{id}/environment-variables", h.ListEnvironmentVariables)
	response := serveAgentEnvironmentVariables(t, router, &org, &user, http.MethodGet,
		"/v1/agents/"+agent.ID.String()+"/environment-variables", nil)
	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body = %s", response.Code, response.Body.String())
	}
}

func serveAgentEnvironmentVariables(
	t *testing.T,
	router http.Handler,
	org *model.Org,
	user *model.User,
	method string,
	path string,
	body *bytes.Buffer,
) *httptest.ResponseRecorder {
	t.Helper()
	var requestBody *bytes.Buffer
	if body == nil {
		requestBody = bytes.NewBuffer(nil)
	} else {
		requestBody = body
	}
	req := httptest.NewRequest(method, path, requestBody)
	req = middleware.WithOrg(req, org)
	req = middleware.WithUser(req, user)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	return rr
}
