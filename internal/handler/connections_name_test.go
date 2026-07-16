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
	"github.com/usehivy/hivy/internal/mcp/catalog"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

func TestConnectionHandlerRenameUpdatesMCPIdentity(t *testing.T) {
	db := connectTestDB(t)
	user := createTestUser(t, db, fmt.Sprintf("rename-connection-%s@example.com", uuid.NewString()))
	org := createTestOrg(t, db)
	addTestOrgOwner(t, db, org.ID, user.ID)
	integration := createTestIntegration(t, db, "slack")
	connection := model.Connection{OrgID: org.ID, UserID: user.ID, IntegrationID: integration.ID, NangoConnectionID: "rename-test", Name: "abc123", Slug: "abc123", NeedsName: true}
	if err := db.Create(&connection).Error; err != nil {
		t.Fatalf("create connection: %v", err)
	}
	t.Cleanup(func() { db.Delete(&connection) })

	h := handler.NewConnectionHandler(db, nil, catalog.Global(), nil)
	router := chi.NewRouter()
	router.Patch("/v1/connections/{id}/name", h.Rename)
	body, _ := json.Marshal(map[string]string{"name": "Sales Workspace"})
	req := httptest.NewRequest(http.MethodPatch, "/v1/connections/"+connection.ID.String()+"/name", bytes.NewReader(body))
	req = middleware.WithUser(req, &user)
	req = middleware.WithOrg(req, &org)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("rename status = %d: %s", recorder.Code, recorder.Body.String())
	}
	if err := db.First(&connection, "id = ?", connection.ID).Error; err != nil {
		t.Fatalf("reload connection: %v", err)
	}
	if connection.Name != "Sales Workspace" || connection.Slug != "sales-workspace" || connection.NeedsName {
		t.Fatalf("renamed connection = name %q slug %q needs_name %v", connection.Name, connection.Slug, connection.NeedsName)
	}

	second := model.Connection{OrgID: org.ID, UserID: user.ID, IntegrationID: integration.ID, NangoConnectionID: "rename-conflict", Name: "other", Slug: "other"}
	if err := db.Create(&second).Error; err != nil {
		t.Fatalf("create second connection: %v", err)
	}
	t.Cleanup(func() { db.Delete(&second) })
	conflictReq := httptest.NewRequest(http.MethodPatch, "/v1/connections/"+second.ID.String()+"/name", bytes.NewReader(body))
	conflictReq = middleware.WithUser(conflictReq, &user)
	conflictReq = middleware.WithOrg(conflictReq, &org)
	conflictRecorder := httptest.NewRecorder()
	router.ServeHTTP(conflictRecorder, conflictReq)
	if conflictRecorder.Code != http.StatusConflict {
		t.Fatalf("duplicate rename status = %d: %s", conflictRecorder.Code, conflictRecorder.Body.String())
	}
}

func TestDatabaseIntegrationHandlerRenameUpdatesMCPIdentity(t *testing.T) {
	db := connectTestDB(t)
	user := createTestUser(t, db, fmt.Sprintf("rename-database-%s@example.com", uuid.NewString()))
	org := createTestOrg(t, db)
	addTestOrgOwner(t, db, org.ID, user.ID)
	connection := model.DatabaseConnection{OrgID: org.ID, Provider: "postgres", DisplayName: "Postgres", Name: "def456", Slug: "def456", NeedsName: true, EncryptedDSN: []byte("encrypted"), WrappedDEK: []byte("wrapped"), SchemaSnapshot: model.RawJSON(`{}`), AccessPolicy: model.JSON{}}
	if err := db.Create(&connection).Error; err != nil {
		t.Fatalf("create database connection: %v", err)
	}
	t.Cleanup(func() { db.Delete(&connection) })

	h := handler.NewDatabaseIntegrationHandler(db, nil)
	router := chi.NewRouter()
	router.Patch("/v1/database-integrations/{id}/name", h.Rename)
	body, _ := json.Marshal(map[string]string{"name": "Reporting Database"})
	req := httptest.NewRequest(http.MethodPatch, "/v1/database-integrations/"+connection.ID.String()+"/name", bytes.NewReader(body))
	req = middleware.WithUser(req, &user)
	req = middleware.WithOrg(req, &org)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("rename status = %d: %s", recorder.Code, recorder.Body.String())
	}
	if err := db.First(&connection, "id = ?", connection.ID).Error; err != nil {
		t.Fatalf("reload database connection: %v", err)
	}
	if connection.Name != "Reporting Database" || connection.Slug != "reporting-database" || connection.NeedsName {
		t.Fatalf("renamed database = name %q slug %q needs_name %v", connection.Name, connection.Slug, connection.NeedsName)
	}
}
