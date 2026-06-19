package handler_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

func TestUploadAgentAsset_ImageCreatesDriveAsset(t *testing.T) {
	h := newStreamHarness(t)

	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			req = middleware.WithOrg(req, &model.Org{ID: h.orgID})
			next.ServeHTTP(w, req)
		})
	})
	r.Post("/v1/assets/upload", h.publicAsset.UploadAgentAsset)

	body, contentType := multipartImageUploadBody(t, h.agentID, "screenshot.png", tinyPNG(t))
	req := httptest.NewRequest(http.MethodPost, "/v1/assets/upload", body)
	req.Header.Set("Content-Type", contentType)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	var resp struct {
		ID          uuid.UUID `json:"id"`
		AssetURL    string    `json:"asset_url"`
		Key         string    `json:"key"`
		Path        string    `json:"path"`
		Filename    string    `json:"filename"`
		ContentType string    `json:"content_type"`
		Bytes       int64     `json:"bytes"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Filename != "screenshot.png" || resp.ContentType != "image/png" || resp.Bytes == 0 {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if resp.AssetURL == "" || resp.Key == "" {
		t.Fatalf("missing asset URL/key: %+v", resp)
	}

	var row model.AgentAsset
	if err := h.db.Where("id = ?", resp.ID).First(&row).Error; err != nil {
		t.Fatalf("load asset row: %v", err)
	}
	if row.OrgID != h.orgID || row.AgentID != h.agentID || row.SandboxID != h.sandboxID {
		t.Fatalf("asset ownership mismatch: %+v", row)
	}
}

func multipartImageUploadBody(t *testing.T, agentID uuid.UUID, filename string, fileBytes []byte) (*bytes.Buffer, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("agent_id", agentID.String()); err != nil {
		t.Fatalf("write agent_id: %v", err)
	}
	if err := writer.WriteField("path", "uploads"); err != nil {
		t.Fatalf("write path: %v", err)
	}
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("create file part: %v", err)
	}
	if _, err := part.Write(fileBytes); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart: %v", err)
	}
	return &body, writer.FormDataContentType()
}

func tinyPNG(t *testing.T) []byte {
	t.Helper()
	raw, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/p9sAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatalf("decode png: %v", err)
	}
	return raw
}
