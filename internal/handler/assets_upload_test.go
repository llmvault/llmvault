package handler_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
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

func TestUploadAgentAsset_AudioCreatesDriveAsset(t *testing.T) {
	h := newStreamHarness(t)

	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			req = middleware.WithOrg(req, &model.Org{ID: h.orgID})
			next.ServeHTTP(w, req)
		})
	})
	r.Post("/v1/assets/upload", h.publicAsset.UploadAgentAsset)

	body, contentType := multipartAssetUploadBody(t, h.agentID, "voice.webm", "audio/webm", []byte("fake webm voice note"))
	req := httptest.NewRequest(http.MethodPost, "/v1/assets/upload", body)
	req.Header.Set("Content-Type", contentType)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	var resp struct {
		ID          uuid.UUID `json:"id"`
		Filename    string    `json:"filename"`
		ContentType string    `json:"content_type"`
		Bytes       int64     `json:"bytes"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Filename != "voice.webm" || resp.ContentType != "audio/webm" || resp.Bytes == 0 {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func multipartImageUploadBody(t *testing.T, agentID uuid.UUID, filename string, fileBytes []byte) (*bytes.Buffer, string) {
	t.Helper()
	return multipartAssetUploadBody(t, agentID, filename, "", fileBytes)
}

func multipartAssetUploadBody(t *testing.T, agentID uuid.UUID, filename, contentType string, fileBytes []byte) (*bytes.Buffer, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("agent_id", agentID.String()); err != nil {
		t.Fatalf("write agent_id: %v", err)
	}
	if err := writer.WriteField("path", "uploads"); err != nil {
		t.Fatalf("write path: %v", err)
	}
	var part io.Writer
	var err error
	if contentType == "" {
		part, err = writer.CreateFormFile("file", filename)
	} else {
		header := make(textproto.MIMEHeader)
		header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename="%s"`, filename))
		header.Set("Content-Type", contentType)
		part, err = writer.CreatePart(header)
	}
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
