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
	var agentInstall model.AgentPluginInstall
	if err := db.Where("org_id = ? AND plugin_id = ?", org.ID, plugin.ID).First(&agentInstall).Error; err != nil {
		t.Fatalf("agent plugin install missing: %v", err)
	}
}
