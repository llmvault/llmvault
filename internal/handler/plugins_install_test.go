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

	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/tasks"
)

func TestPluginHandler_InstallEnqueuesPluginInstallSync(t *testing.T) {
	db := connectTestDB(t)
	enq := &recordingEnqueuer{}
	h := handler.NewPluginHandler(db, enq)
	r := chi.NewRouter()
	r.Post("/v1/plugins/{slug}/install", h.Install)

	user := createTestUser(t, db, fmt.Sprintf("plugin-install-%s@test.com", uuid.New().String()[:8]))
	org := createTestOrg(t, db)
	integ := createTestIntegration(t, db, "linear")
	conn := model.Connection{
		ID:                uuid.New(),
		OrgID:             org.ID,
		UserID:            user.ID,
		IntegrationID:     integ.ID,
		NangoConnectionID: "linear-plugin-install-test",
		Meta:              model.JSON{},
	}
	if err := db.Create(&conn).Error; err != nil {
		t.Fatalf("create connection: %v", err)
	}
	plugin := model.Plugin{
		ID:       uuid.New(),
		Slug:     "linear-plugin-install-" + uuid.NewString()[:8],
		Name:     "Linear Plugin Install Test",
		Status:   model.PluginStatusActive,
		Manifest: model.RawJSON(`{}`),
	}
	if err := db.Create(&plugin).Error; err != nil {
		t.Fatalf("create plugin: %v", err)
	}
	if err := db.Create(&model.PluginIntegration{
		PluginID: plugin.ID,
		Provider: "linear",
		Kind:     model.PluginIntegrationKindIntegration,
		Required: true,
	}).Error; err != nil {
		t.Fatalf("create plugin integration: %v", err)
	}
	t.Cleanup(func() {
		db.Where("org_id = ?", org.ID).Delete(&model.AgentPluginInstall{})
		db.Where("org_id = ?", org.ID).Delete(&model.OrgPluginInstall{})
		db.Where("id = ?", conn.ID).Delete(&model.Connection{})
		db.Where("plugin_id = ?", plugin.ID).Delete(&model.PluginIntegration{})
		db.Where("id = ?", plugin.ID).Delete(&model.Plugin{})
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/plugins/"+plugin.Slug+"/install", bytes.NewReader([]byte(`{}`)))
	req = middleware.WithUser(req, &user)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("install status=%d body=%s", rr.Code, rr.Body.String())
	}
	if len(enq.tasks) != 1 {
		t.Fatalf("enqueued tasks=%d, want 1", len(enq.tasks))
	}
	task := enq.tasks[0]
	if task.Type() != tasks.TypePluginInstallSync {
		t.Fatalf("task type=%q, want %q", task.Type(), tasks.TypePluginInstallSync)
	}
	var payload tasks.PluginInstallSyncPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		t.Fatalf("decode plugin sync payload: %v", err)
	}
	if payload.OrgID != org.ID || payload.PluginID != plugin.ID || payload.InstallID == uuid.Nil {
		t.Fatalf("bad plugin sync payload: %+v", payload)
	}
	var orgInstallCount int64
	if err := db.Model(&model.OrgPluginInstall{}).
		Where("org_id = ? AND plugin_id = ? AND revoked_at IS NULL", org.ID, plugin.ID).
		Count(&orgInstallCount).Error; err != nil {
		t.Fatalf("count org plugin install: %v", err)
	}
	if orgInstallCount != 1 {
		t.Fatalf("org plugin install count = %d, want 1", orgInstallCount)
	}
	var agentInstallCount int64
	if err := db.Model(&model.AgentPluginInstall{}).
		Where("org_id = ? AND plugin_id = ?", org.ID, plugin.ID).
		Count(&agentInstallCount).Error; err != nil {
		t.Fatalf("count agent plugin installs: %v", err)
	}
	if agentInstallCount != 0 {
		t.Fatalf("agent plugin install count = %d, want 0", agentInstallCount)
	}
}

func TestPluginHandler_GetReturnsSkillHumanDescription(t *testing.T) {
	db := connectTestDB(t)
	h := handler.NewPluginHandler(db)
	r := chi.NewRouter()
	r.Get("/v1/plugins/{slug}", h.Get)

	org := createTestOrg(t, db)
	plugin := model.Plugin{
		ID:       uuid.New(),
		Slug:     "human-description-plugin-" + uuid.NewString()[:8],
		Name:     "Human Description Plugin",
		Status:   model.PluginStatusActive,
		Manifest: model.RawJSON(`{}`),
	}
	if err := db.Create(&plugin).Error; err != nil {
		t.Fatalf("create plugin: %v", err)
	}
	agentDesc := "Agent-facing plugin skill description."
	humanDesc := "User-facing plugin skill summary."
	skill := model.Skill{
		ID:               uuid.New(),
		PluginID:         &plugin.ID,
		Slug:             "plugin-skill",
		Name:             "Plugin Skill",
		Description:      &agentDesc,
		HumanDescription: &humanDesc,
		Category:         "Testing",
		SourceType:       model.SkillSourceInline,
		RepoRef:          "main",
		Bundle:           model.RawJSON(`{"description":"Agent-facing plugin skill description.","content":"# Plugin Skill"}`),
		Status:           model.SkillStatusPublished,
	}
	if err := db.Create(&skill).Error; err != nil {
		t.Fatalf("create plugin skill: %v", err)
	}
	t.Cleanup(func() {
		db.Where("id = ?", skill.ID).Delete(&model.Skill{})
		db.Where("id = ?", plugin.ID).Delete(&model.Plugin{})
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/plugins/"+plugin.Slug, nil)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Skills []struct {
			Description      string  `json:"description"`
			HumanDescription *string `json:"human_description"`
		} `json:"skills"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Skills) != 1 {
		t.Fatalf("skills = %d, want 1", len(resp.Skills))
	}
	if resp.Skills[0].Description != agentDesc {
		t.Fatalf("description = %q, want %q", resp.Skills[0].Description, agentDesc)
	}
	if resp.Skills[0].HumanDescription == nil || *resp.Skills[0].HumanDescription != humanDesc {
		t.Fatalf("human_description = %#v", resp.Skills[0].HumanDescription)
	}
}
