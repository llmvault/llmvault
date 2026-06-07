package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/employeeruntime"
	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/registry"
	"github.com/usehivy/hivy/internal/tasks"
)

func TestEmployeeHandler_UpdateConnectionResourcesStoresOnEmployeeAndQueuesGitHubClone(t *testing.T) {
	db := connectTestDB(t)
	t.Cleanup(func() {
		db.Where("1=1").Delete(&model.Connection{})
		db.Where("1=1").Delete(&model.Integration{})
		db.Where("1=1").Delete(&model.Employee{})
	})

	org := createTestOrg(t, db)
	user := createTestUser(t, db, "resources-"+uuid.New().String()[:8]+"@test.com")
	integration := createTestIntegration(t, db, "github-app")
	_ = createTestIntegrationManagedSkill(t, db, "git-github-test-"+uuid.New().String()[:8], []string{"github-app"})
	employee := model.Employee{
		ID:            uuid.New(),
		OrgID:         &org.ID,
		IsEmployee:    true,
		Model:         employeeruntime.DefaultEmployeeModel,
		Status:        "active",
		Tools:         model.JSON{},
		McpServers:    model.JSON{},
		Skills:        model.JSON{},
		RuntimeConfig: model.JSON{},
		Permissions:   model.JSON{},
		Resources:     model.JSON{},
	}
	if err := db.Create(&employee).Error; err != nil {
		t.Fatalf("create employee: %v", err)
	}
	conn := model.Connection{
		ID:                uuid.New(),
		OrgID:             org.ID,
		UserID:            user.ID,
		IntegrationID:     integration.ID,
		NangoConnectionID: "github-resources-test",
		Meta:              model.JSON{"resources": "must-not-be-used"},
	}
	if err := db.Create(&conn).Error; err != nil {
		t.Fatalf("create connection: %v", err)
	}

	enq := &enqueue.MockClient{}
	h := handler.NewEmployeeHandler(db, nil, employeeruntime.CompileDeps{}, registry.Global())
	h.SetEnqueuer(enq)
	r := chi.NewRouter()
	r.Put("/v1/employees/{id}/connections/{connectionID}/resources", h.UpdateConnectionResources)

	body, _ := json.Marshal(map[string]any{
		"resources": map[string]any{
			"repository": []map[string]any{
				{"id": "usehivy/hivy", "name": "hivy", "type": "repository"},
			},
		},
	})
	req := httptest.NewRequest(http.MethodPut,
		"/v1/employees/"+employee.ID.String()+"/connections/"+conn.ID.String()+"/resources",
		bytes.NewReader(body),
	)
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var reloaded model.Employee
	if err := db.First(&reloaded, "id = ?", employee.ID).Error; err != nil {
		t.Fatalf("reload employee: %v", err)
	}
	stored := reloaded.Resources[conn.ID.String()].(map[string]any)
	repos := stored["repository"].([]any)
	repo := repos[0].(map[string]any)
	if repo["full_name"] != "usehivy/hivy" {
		t.Fatalf("stored full_name = %v, want usehivy/hivy", repo["full_name"])
	}
	enq.AssertEnqueued(t, tasks.TypeEmployeeGitHubResourcesClone)
}
