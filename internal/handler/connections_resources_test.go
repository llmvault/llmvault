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

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/mcp/catalog"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/tasks"
)

func TestConnectionHandler_UpdateResourcesQueuesCloneForAlwaysOnAgents(t *testing.T) {
	db := connectTestDB(t)
	org := createTestOrg(t, db)
	user := createTestUser(t, db, "connection-resources-"+uuid.New().String()[:8]+"@test.com")
	integration := createTestIntegration(t, db, "github-app")
	plugin := model.Plugin{
		ID:       uuid.New(),
		Slug:     "github-resources-" + uuid.NewString()[:8],
		Name:     "GitHub Resources",
		Status:   model.PluginStatusActive,
		Manifest: model.RawJSON(`{}`),
	}
	if err := db.Create(&plugin).Error; err != nil {
		t.Fatalf("create plugin: %v", err)
	}
	if err := db.Create(&model.PluginIntegration{
		PluginID: plugin.ID,
		Provider: "github-app",
		Kind:     model.PluginIntegrationKindIntegration,
		Required: true,
	}).Error; err != nil {
		t.Fatalf("create plugin integration: %v", err)
	}
	install := model.OrgPluginInstall{ID: uuid.New(), OrgID: org.ID, PluginID: plugin.ID}
	if err := db.Create(&install).Error; err != nil {
		t.Fatalf("create org plugin install: %v", err)
	}
	alwaysOn := createResourceTestAgent(t, db, org.ID, "always_on")
	perSession := createResourceTestAgent(t, db, org.ID, "per_session")
	for _, agent := range []model.Agent{alwaysOn, perSession} {
		if err := db.Create(&model.AgentPluginInstall{
			OrgID:    org.ID,
			AgentID:  agent.ID,
			PluginID: plugin.ID,
		}).Error; err != nil {
			t.Fatalf("create agent plugin install: %v", err)
		}
	}
	conn := model.Connection{
		ID:                uuid.New(),
		OrgID:             org.ID,
		UserID:            user.ID,
		IntegrationID:     integration.ID,
		NangoConnectionID: "github-default-resources-test",
		Meta:              model.JSON{},
	}
	if err := db.Create(&conn).Error; err != nil {
		t.Fatalf("create connection: %v", err)
	}
	t.Cleanup(func() {
		db.Where("org_id = ?", org.ID).Delete(&model.AgentPluginInstall{})
		db.Where("org_id = ?", org.ID).Delete(&model.OrgPluginInstall{})
		db.Where("org_id = ?", org.ID).Delete(&model.Connection{})
		db.Where("org_id = ?", org.ID).Delete(&model.Agent{})
		db.Where("plugin_id = ?", plugin.ID).Delete(&model.PluginIntegration{})
		db.Where("id = ?", plugin.ID).Delete(&model.Plugin{})
	})

	enq := &enqueue.MockClient{}
	h := handler.NewConnectionHandler(db, nil, catalog.Global(), enq)
	r := chi.NewRouter()
	r.Put("/v1/connections/{id}/resources", h.UpdateResources)

	body, _ := json.Marshal(map[string]any{
		"resources": map[string]any{
			"repository": []map[string]any{
				{"id": "usehivy/hivy", "name": "hivy", "type": "repository"},
			},
		},
	})
	req := httptest.NewRequest(http.MethodPut, "/v1/connections/"+conn.ID.String()+"/resources", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	queued := enq.Tasks()
	if len(queued) != 1 {
		t.Fatalf("queued tasks=%d, want 1", len(queued))
	}
	if queued[0].TypeName != tasks.TypeAgentGitHubResourcesClone {
		t.Fatalf("queued task type=%q, want %q", queued[0].TypeName, tasks.TypeAgentGitHubResourcesClone)
	}
	var payload tasks.AgentGitHubResourcesClonePayload
	if err := json.Unmarshal(queued[0].Payload, &payload); err != nil {
		t.Fatalf("decode clone payload: %v", err)
	}
	if payload.AgentID != alwaysOn.ID {
		t.Fatalf("queued agent=%s, want always-on agent %s", payload.AgentID, alwaysOn.ID)
	}
	if payload.ConnectionID != conn.ID {
		t.Fatalf("queued connection=%s, want %s", payload.ConnectionID, conn.ID)
	}
}

func createResourceTestAgent(t *testing.T, db *gorm.DB, orgID uuid.UUID, strategy string) model.Agent {
	t.Helper()
	agent := model.Agent{
		ID:              uuid.New(),
		OrgID:           &orgID,
		Name:            "Resource " + strategy + " " + uuid.NewString()[:8],
		IsManaged:       true,
		SandboxStrategy: strategy,
		Model:           agentruntime.DefaultAgentModel,
		Status:          "active",
		Tools:           model.JSON{},
		McpServers:      model.RawJSON("[]"),
		Skills:          model.JSON{},
		RuntimeConfig:   model.JSON{},
		Permissions:     model.JSON{},
		Resources:       model.JSON{},
	}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	return agent
}
