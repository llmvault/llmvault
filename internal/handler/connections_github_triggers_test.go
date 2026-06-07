package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/mcp/catalog"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/nango"
)

func TestConnectionHandler_CreateGitHubAppCreatesEmployeeTriggers(t *testing.T) {
	db := connectTestDB(t)
	t.Cleanup(func() {
		db.Where("1=1").Delete(&model.Connection{})
		db.Where("1=1").Delete(&model.Integration{})
	})

	nangoSrv := httptest.NewServer(newNangoConnMock(&nangoConnMockConfig{}))
	t.Cleanup(nangoSrv.Close)
	nangoClient := nango.NewClient(nangoSrv.URL, "test-secret-key")
	_ = nangoClient.FetchProviders(context.Background())

	h := handler.NewConnectionHandler(db, nangoClient, catalog.Global(), nil)
	r := chi.NewRouter()
	r.Post("/v1/integrations/{id}/connections", h.Create)

	user := createTestUser(t, db, fmt.Sprintf("github-triggers-%s@test.com", uuid.New().String()[:8]))
	org := createTestOrg(t, db)
	integ := createTestIntegration(t, db, "github-app")

	body, _ := json.Marshal(map[string]any{"nango_connection_id": "github-app-conn-123"})
	req := httptest.NewRequest(http.MethodPost, "/v1/integrations/"+integ.ID.String()+"/connections", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithUser(req, &user)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var employee model.Employee
	if err := db.Where("org_id = ? AND status <> ?", org.ID, "archived").First(&employee).Error; err != nil {
		t.Fatalf("load employee: %v", err)
	}
	var conn model.Connection
	if err := db.Where("org_id = ? AND integration_id = ?", org.ID, integ.ID).First(&conn).Error; err != nil {
		t.Fatalf("load connection: %v", err)
	}

	var triggers []model.EmployeeTrigger
	if err := db.Where("org_id = ? AND employee_id = ? AND connection_id = ?",
		org.ID, employee.ID, conn.ID).
		Order("created_at ASC").
		Find(&triggers).Error; err != nil {
		t.Fatalf("load triggers: %v", err)
	}
	if len(triggers) != 2 {
		t.Fatalf("github triggers = %d, want 2: %#v", len(triggers), triggers)
	}

	byKey := map[string]model.EmployeeTrigger{}
	for _, trigger := range triggers {
		if len(trigger.TriggerKeys) != 1 {
			t.Fatalf("trigger keys = %#v, want exactly one", trigger.TriggerKeys)
		}
		byKey[trigger.TriggerKeys[0]] = trigger
		if !trigger.Enabled || trigger.TriggerType != "webhook" {
			t.Fatalf("trigger not enabled webhook: %#v", trigger)
		}
	}

	mention := byKey["issue_comment.created"]
	if mention.ID == uuid.Nil {
		t.Fatal("missing issue_comment.created trigger")
	}
	if !strings.Contains(mention.Instructions, "@usehivy") ||
		!strings.Contains(mention.Instructions, "Inspect the linked issue or pull request") {
		t.Fatalf("mention instructions unexpected:\n%s", mention.Instructions)
	}
	if strings.Contains(mention.Instructions, "Route this request through the employee agent") {
		t.Fatalf("mention instructions leaked internal routing text:\n%s", mention.Instructions)
	}
	assertTriggerCondition(t, mention, "comment.body", "matches", "@usehivy")

	ci := byKey["check_suite.completed"]
	if ci.ID == uuid.Nil {
		t.Fatal("missing check_suite.completed trigger")
	}
	if !strings.Contains(ci.Instructions, "Hivy-created pull request") ||
		!strings.Contains(ci.Instructions, "check suite result") {
		t.Fatalf("ci instructions unexpected:\n%s", ci.Instructions)
	}
	if strings.Contains(ci.Instructions, "Route this event through the employee agent") {
		t.Fatalf("ci instructions leaked internal routing text:\n%s", ci.Instructions)
	}
	assertTriggerCondition(t, ci, "check_suite.pull_requests.0.number", "exists", "")
}

func assertTriggerCondition(t *testing.T, trigger model.EmployeeTrigger, path, operator, valueContains string) {
	t.Helper()
	var match model.TriggerMatch
	if err := json.Unmarshal(trigger.Conditions, &match); err != nil {
		t.Fatalf("decode conditions: %v", err)
	}
	if match.Mode != "all" || len(match.Conditions) != 1 {
		t.Fatalf("conditions = %#v", match)
	}
	condition := match.Conditions[0]
	if condition.Path != path || condition.Operator != operator {
		t.Fatalf("condition = %#v, want path=%s operator=%s", condition, path, operator)
	}
	if valueContains != "" && !strings.Contains(fmt.Sprint(condition.Value), valueContains) {
		t.Fatalf("condition value = %#v, want containing %q", condition.Value, valueContains)
	}
}
