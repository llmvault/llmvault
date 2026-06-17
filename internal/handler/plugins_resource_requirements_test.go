package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

func TestPluginHandler_GetReportsMissingResourceRequirements(t *testing.T) {
	db := connectTestDB(t)
	org := createTestOrg(t, db)
	user := createTestUser(t, db, "plugin-resources-"+uuid.New().String()[:8]+"@test.com")
	integration := createTestIntegration(t, db, "github-app")
	conn := model.Connection{
		ID:                uuid.New(),
		OrgID:             org.ID,
		UserID:            user.ID,
		IntegrationID:     integration.ID,
		NangoConnectionID: "github-plugin-resources-test",
		Meta:              model.JSON{},
	}
	if err := db.Create(&conn).Error; err != nil {
		t.Fatalf("create connection: %v", err)
	}
	plugin := model.Plugin{
		ID:       uuid.New(),
		Slug:     "github-plugin-resources-" + uuid.NewString()[:8],
		Name:     "GitHub Plugin Resources",
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
	if err := db.Create(&model.OrgPluginInstall{ID: uuid.New(), OrgID: org.ID, PluginID: plugin.ID}).Error; err != nil {
		t.Fatalf("create plugin install: %v", err)
	}
	t.Cleanup(func() {
		db.Where("org_id = ?", org.ID).Delete(&model.OrgPluginInstall{})
		db.Where("id = ?", conn.ID).Delete(&model.Connection{})
		db.Where("plugin_id = ?", plugin.ID).Delete(&model.PluginIntegration{})
		db.Where("id = ?", plugin.ID).Delete(&model.Plugin{})
	})

	requirement := getPluginRepositoryRequirement(t, db, org, plugin.Slug)
	if requirement.ConnectionID != conn.ID.String() {
		t.Fatalf("connection_id=%q, want %q", requirement.ConnectionID, conn.ID.String())
	}
	if requirement.ResourceKey != "repository" {
		t.Fatalf("resource_key=%q, want repository", requirement.ResourceKey)
	}
	if !requirement.Missing {
		t.Fatalf("missing=false, want true")
	}
	if requirement.Selected || requirement.SelectedCount != 0 {
		t.Fatalf("selected=%v selected_count=%d, want false/0", requirement.Selected, requirement.SelectedCount)
	}

	if err := db.Model(&model.Connection{}).
		Where("id = ?", conn.ID).
		Update("meta", model.JSON{"resources": model.JSON{
			"repository": []map[string]any{
				{"id": "usehivy/hivy", "name": "hivy", "type": "repository", "full_name": "usehivy/hivy"},
			},
		}}).Error; err != nil {
		t.Fatalf("update connection resources: %v", err)
	}

	requirement = getPluginRepositoryRequirement(t, db, org, plugin.Slug)
	if requirement.Missing {
		t.Fatalf("missing=true, want false after resource selection")
	}
	if !requirement.Selected || requirement.SelectedCount != 1 {
		t.Fatalf("selected=%v selected_count=%d, want true/1", requirement.Selected, requirement.SelectedCount)
	}
}

func getPluginRepositoryRequirement(t *testing.T, db *gorm.DB, org model.Org, slug string) resourceRequirementResponse {
	t.Helper()
	h := handler.NewPluginHandler(db)
	r := chi.NewRouter()
	r.Get("/v1/plugins/{slug}", h.Get)
	req := httptest.NewRequest(http.MethodGet, "/v1/plugins/"+slug, nil)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		ResourceRequirements []resourceRequirementResponse `json:"resource_requirements"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode plugin response: %v", err)
	}
	for _, requirement := range resp.ResourceRequirements {
		if requirement.ResourceKey == "repository" {
			return requirement
		}
	}
	t.Fatalf("repository resource requirement not found: %+v", resp.ResourceRequirements)
	return resourceRequirementResponse{}
}

type resourceRequirementResponse struct {
	ConnectionID  string `json:"connection_id"`
	ResourceKey   string `json:"resource_key"`
	Selected      bool   `json:"selected"`
	SelectedCount int    `json:"selected_count"`
	Missing       bool   `json:"missing"`
}
