package handler_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

type assetsListHarness struct {
	db     *gorm.DB
	router *chi.Mux

	orgA model.Org
	orgB model.Org

	agentA1 uuid.UUID
	agentA2 uuid.UUID
	agentB1 uuid.UUID

	sandboxA1 uuid.UUID
	sandboxA2 uuid.UUID
	sandboxB1 uuid.UUID
}

func seedAssetRow(t *testing.T, db *gorm.DB, orgID, employeeID, sandboxID uuid.UUID, folder, filename, contentType string, bytes int64, createdAt time.Time) model.EmployeeAsset {
	t.Helper()
	key := fmt.Sprintf("pub/e/%s/%s", employeeID, filename)
	if folder != "" {
		key = fmt.Sprintf("pub/e/%s/%s/%s", employeeID, folder, filename)
	}
	a := model.EmployeeAsset{
		ID:          uuid.New(),
		OrgID:       orgID,
		EmployeeID:  employeeID,
		SandboxID:   sandboxID,
		Path:        folder,
		Filename:    filename,
		Key:         key,
		PublicURL:   "https://cdn.example.com/" + key,
		ContentType: contentType,
		Bytes:       bytes,
		CreatedAt:   createdAt,
		UpdatedAt:   createdAt,
	}
	if err := db.Create(&a).Error; err != nil {
		t.Fatalf("seed asset: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", a.ID).Delete(&model.EmployeeAsset{}) })
	return a
}

func newAssetsListHarness(t *testing.T) *assetsListHarness {
	t.Helper()
	db := connectTestDB(t)

	h := handler.NewUploadsHandler(db, nil)
	h.WithAssetPreviewBaseURL("https://api.usehivy.test")
	r := chi.NewRouter()
	r.Get("/v1/assets", h.ListAssets)

	mkOrg := func(prefix string) model.Org {
		o := model.Org{
			ID:        uuid.New(),
			Name:      fmt.Sprintf("%s-%s", prefix, uuid.New().String()[:8]),
			RateLimit: 1000,
			Active:    true,
		}
		if err := db.Create(&o).Error; err != nil {
			t.Fatalf("create org: %v", err)
		}
		t.Cleanup(func() { db.Where("id = ?", o.ID).Delete(&model.Org{}) })
		return o
	}
	mkAgent := func(orgID uuid.UUID, name string) uuid.UUID {
		id := uuid.New()
		if err := db.Create(&model.Employee{ID: id, OrgID: &orgID, Name: name, Status: "active"}).Error; err != nil {
			t.Fatalf("create agent: %v", err)
		}
		return id
	}
	mkSandbox := func(orgID, agentID uuid.UUID) uuid.UUID {
		id := uuid.New()
		if err := db.Create(&model.Sandbox{
			ID:                     id,
			OrgID:                  &orgID,
			EmployeeID:             &agentID,
			EncryptedRuntimeSecret: []byte("placeholder"),
			Status:                 "running",
			ExternalID:             "x",
			RuntimeURL:             "http://localhost:1",
		}).Error; err != nil {
			t.Fatalf("create sandbox: %v", err)
		}
		return id
	}
	orgA := mkOrg("assets-a")
	orgB := mkOrg("assets-b")

	agentA1 := mkAgent(orgA.ID, "agent-a1")
	agentA2 := mkAgent(orgA.ID, "agent-a2")
	agentB1 := mkAgent(orgB.ID, "agent-b1")

	sbA1 := mkSandbox(orgA.ID, agentA1)
	sbA2 := mkSandbox(orgA.ID, agentA2)
	sbB1 := mkSandbox(orgB.ID, agentB1)

	return &assetsListHarness{
		db: db, router: r,
		orgA: orgA, orgB: orgB,
		agentA1: agentA1, agentA2: agentA2, agentB1: agentB1,
		sandboxA1: sbA1, sandboxA2: sbA2, sandboxB1: sbB1,
	}
}

func (h *assetsListHarness) get(t *testing.T, query string, org *model.Org) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/v1/assets"+query, nil)
	if org != nil {
		req = middleware.WithOrg(req, org)
	}
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	return rr
}

type assetListPage struct {
	Data       []map[string]any `json:"data"`
	HasMore    bool             `json:"has_more"`
	NextCursor *string          `json:"next_cursor,omitempty"`
}

func decodeAssetList(t *testing.T, rr *httptest.ResponseRecorder) assetListPage {
	t.Helper()
	var p assetListPage
	if err := json.Unmarshal(rr.Body.Bytes(), &p); err != nil {
		t.Fatalf("decode list: %v body=%s", err, rr.Body.String())
	}
	return p
}
