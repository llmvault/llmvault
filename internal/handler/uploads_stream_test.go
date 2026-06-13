package handler_test

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/model"
)

type streamHarness struct {
	db            *gorm.DB
	router        *chi.Mux
	orgID         uuid.UUID
	agentID       uuid.UUID
	sandboxID     uuid.UUID
	runtimeSecret string
	publicBase    string
	publicAsset   *handler.UploadsHandler
}

func newStreamHarness(t *testing.T) *streamHarness {
	t.Helper()
	db := connectTestDB(t)
	presigner := newRealPresigner(t)
	encKey := testSymmetricKey(t)

	h := handler.NewUploadsHandler(db, presigner)
	h.WithAssetPreviewBaseURL("https://api.usehivy.test")
	h.WithStreamer(presigner, encKey)

	r := chi.NewRouter()
	r.Get("/v1/assets/preview", h.PreviewAsset)
	r.Put("/internal/agents/{agentID}/drive/*", h.StreamAgentAsset)
	r.Post("/internal/agents/{agentID}/drive/move", h.MoveAgentAsset)
	r.Delete("/internal/agents/{agentID}/drive/*", h.DeleteAgentAsset)

	orgID := uuid.New()
	if err := db.Create(&model.Org{
		ID:        orgID,
		Name:      fmt.Sprintf("stream-%s", uuid.New().String()[:8]),
		RateLimit: 1000,
		Active:    true,
	}).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", orgID).Delete(&model.Org{}) })

	agentID := uuid.New()
	if err := db.Create(&model.Agent{
		ID:     agentID,
		OrgID:  &orgID,
		Name:   "stream-agent",
		Status: "active",
	}).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}

	runtimeSecret := fmt.Sprintf("test-runtime-key-%s", uuid.New().String()[:8])
	encrypted, err := encKey.EncryptString(runtimeSecret)
	if err != nil {
		t.Fatalf("encrypt runtime secret: %v", err)
	}

	sandboxID := uuid.New()
	if err := db.Create(&model.Sandbox{
		ID:                     sandboxID,
		OrgID:                  &orgID,
		AgentID:                &agentID,
		EncryptedRuntimeSecret: encrypted,
		Status:                 "running",
		ExternalID:             "mock-external-id",
		RuntimeURL:             "http://localhost:25434",
	}).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}

	return &streamHarness{
		db:            db,
		router:        r,
		orgID:         orgID,
		agentID:       agentID,
		sandboxID:     sandboxID,
		runtimeSecret: runtimeSecret,
		publicBase:    testMinioEndpoint + "/" + testMinioBucket,
		publicAsset:   h,
	}
}

func (s *streamHarness) put(t *testing.T, urlPath string, body io.Reader, contentType, bearer string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut, urlPath, body)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	rr := httptest.NewRecorder()
	s.router.ServeHTTP(rr, req)
	return rr
}

func (s *streamHarness) post(t *testing.T, urlPath, bodyJSON, bearer string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, urlPath, strings.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	rr := httptest.NewRecorder()
	s.router.ServeHTTP(rr, req)
	return rr
}

func (s *streamHarness) delete(t *testing.T, urlPath, bearer string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodDelete, urlPath, nil)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	rr := httptest.NewRecorder()
	s.router.ServeHTTP(rr, req)
	return rr
}

func (s *streamHarness) seedAgentAsset(t *testing.T, folder, filename, body string) string {
	t.Helper()
	urlPath := "/internal/agents/" + s.agentID.String() + "/drive/"
	if folder != "" {
		urlPath += folder + "/"
	}
	urlPath += filename
	rr := s.put(t, urlPath, bytes.NewReader([]byte(body)), "text/plain", s.runtimeSecret)
	if rr.Code != http.StatusCreated {
		t.Fatalf("seed asset: got %d: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		PublicURL string `json:"asset_url"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode seed response: %v", err)
	}
	return resp.PublicURL
}

func TestStreamAgentDrive_LargeMultipartStream(t *testing.T) {
	h := newStreamHarness(t)

	const size = 24 * 1024 * 1024
	body := make([]byte, size)
	if _, err := rand.Read(body); err != nil {
		t.Fatalf("rand: %v", err)
	}

	rr := h.put(t,
		fmt.Sprintf("/internal/agents/%s/drive/videos/big.bin", h.agentID),
		bytes.NewReader(body),
		"application/octet-stream",
		h.runtimeSecret,
	)
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Bytes int64 `json:"bytes"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp.Bytes != int64(size) {
		t.Fatalf("bytes: got %d want %d", resp.Bytes, size)
	}
}

func TestStreamAgentDrive_OverwriteByPath(t *testing.T) {
	h := newStreamHarness(t)
	urlPath := fmt.Sprintf("/internal/agents/%s/drive/exports/data.csv", h.agentID)

	first := h.put(t, urlPath, bytes.NewReader([]byte("v1,a")), "text/csv", h.runtimeSecret)
	if first.Code != http.StatusCreated {
		t.Fatalf("first upload: got %d: %s", first.Code, first.Body.String())
	}
	var firstResp struct {
		ID    string `json:"id"`
		Bytes int64  `json:"bytes"`
	}
	_ = json.Unmarshal(first.Body.Bytes(), &firstResp)

	second := h.put(t, urlPath, bytes.NewReader([]byte("v2,a-longer-second-version")), "text/csv", h.runtimeSecret)
	if second.Code != http.StatusCreated {
		t.Fatalf("second upload: got %d: %s", second.Code, second.Body.String())
	}
	var secondResp struct {
		ID    string `json:"id"`
		Bytes int64  `json:"bytes"`
	}
	_ = json.Unmarshal(second.Body.Bytes(), &secondResp)

	if secondResp.ID != firstResp.ID {
		t.Fatalf("expected same row id (overwrite); got first=%s second=%s", firstResp.ID, secondResp.ID)
	}
	if secondResp.Bytes == firstResp.Bytes {
		t.Fatalf("expected new byte count after overwrite")
	}

	wantKey := fmt.Sprintf("pub/e/%s/exports/data.csv", h.agentID)
	var row model.AgentAsset
	if err := h.db.Where("key = ?", wantKey).First(&row).Error; err != nil {
		t.Fatalf("load row: %v", err)
	}
	if row.Bytes != secondResp.Bytes {
		t.Fatalf("row bytes %d != response bytes %d", row.Bytes, secondResp.Bytes)
	}

	var count int64
	h.db.Model(&model.AgentAsset{}).Where("key = ?", wantKey).Count(&count)
	if count != 1 {
		t.Fatalf("expected 1 row for key, got %d", count)
	}
}
