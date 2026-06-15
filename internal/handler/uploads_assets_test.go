package handler_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

func TestDeleteAgentAsset_HappyPath(t *testing.T) {
	h := newStreamHarness(t)
	h.seedAgentAsset(t, "tmp", "scratch.txt", "delete me")
	directURL := fmt.Sprintf("%s/pub/e/%s/tmp/scratch.txt", h.publicBase, h.agentID)

	urlPath := fmt.Sprintf("/internal/agents/%s/drive/tmp/scratch.txt", h.agentID)
	rr := h.delete(t, urlPath, h.runtimeSecret)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rr.Code, rr.Body.String())
	}

	var count int64
	h.db.Model(&model.AgentAsset{}).Where("key = ?", fmt.Sprintf("pub/e/%s/tmp/scratch.txt", h.agentID)).Count(&count)
	if count != 0 {
		t.Fatalf("row still present after delete (count=%d)", count)
	}

	getReq, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, directURL, nil)
	getResp, err := http.DefaultClient.Do(getReq)
	if err != nil {
		t.Fatalf("public GET: %v", err)
	}
	defer getResp.Body.Close()
	if getResp.StatusCode != http.StatusNotFound && getResp.StatusCode != http.StatusForbidden {
		body, _ := io.ReadAll(getResp.Body)
		t.Fatalf("expected 404/403 after delete, got %d: %s", getResp.StatusCode, body)
	}
}

func TestDeleteAgentAsset_NotFound(t *testing.T) {
	h := newStreamHarness(t)
	rr := h.delete(t,
		fmt.Sprintf("/internal/agents/%s/drive/nope/missing.txt", h.agentID),
		h.runtimeSecret,
	)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

func TestDeleteAgentAsset_BadBearer(t *testing.T) {
	h := newStreamHarness(t)
	h.seedAgentAsset(t, "tmp", "x.txt", "hi")
	rr := h.delete(t,
		fmt.Sprintf("/internal/agents/%s/drive/tmp/x.txt", h.agentID),
		"wrong-key",
	)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestMoveAgentAsset_ByRelativePath(t *testing.T) {
	h := newStreamHarness(t)
	h.seedAgentAsset(t, "videos", "demo.mp4", "fake mp4")

	body := `{"asset":"videos/demo.mp4","new_path":"archive/2026"}`
	rr := h.post(t,
		fmt.Sprintf("/internal/agents/%s/drive/move", h.agentID),
		body,
		h.runtimeSecret,
	)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Path     string `json:"path"`
		Key      string `json:"key"`
		Filename string `json:"filename"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Path != "archive/2026" {
		t.Fatalf("path: got %q want archive/2026", resp.Path)
	}
	wantKey := fmt.Sprintf("pub/e/%s/videos/demo.mp4", h.agentID)
	if resp.Key != wantKey {
		t.Fatalf("key changed: got %q want %q", resp.Key, wantKey)
	}

	var row model.AgentAsset
	if err := h.db.Where("key = ?", wantKey).First(&row).Error; err != nil {
		t.Fatalf("load row: %v", err)
	}
	if row.Path != "archive/2026" {
		t.Fatalf("row.Path = %q", row.Path)
	}
}

func TestMoveAgentAsset_ByPublicURL(t *testing.T) {
	h := newStreamHarness(t)
	publicURL := h.seedAgentAsset(t, "tmp", "doc.txt", "hi")

	body := fmt.Sprintf(`{"asset":%q,"new_path":""}`, publicURL)
	rr := h.post(t,
		fmt.Sprintf("/internal/agents/%s/drive/move", h.agentID),
		body,
		h.runtimeSecret,
	)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Path != "" {
		t.Fatalf("path: got %q want empty (root)", resp.Path)
	}
}

func TestMoveAgentAsset_ByDirectPublicURL(t *testing.T) {
	h := newStreamHarness(t)
	h.seedAgentAsset(t, "tmp", "old.txt", "hi")
	publicURL := fmt.Sprintf("%s/pub/e/%s/tmp/old.txt", h.publicBase, h.agentID)

	body := fmt.Sprintf(`{"asset":%q,"new_path":"archive"}`, publicURL)
	rr := h.post(t,
		fmt.Sprintf("/internal/agents/%s/drive/move", h.agentID),
		body,
		h.runtimeSecret,
	)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Path != "archive" {
		t.Fatalf("path: got %q want archive", resp.Path)
	}
}

func TestMoveAgentAsset_RejectsForeignURL(t *testing.T) {
	h := newStreamHarness(t)
	body := fmt.Sprintf(`{"asset":"https://example.com/pub/e/%s/foo.txt","new_path":"archive"}`, uuid.New())
	rr := h.post(t,
		fmt.Sprintf("/internal/agents/%s/drive/move", h.agentID),
		body,
		h.runtimeSecret,
	)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestMoveAgentAsset_RejectsTraversalNewPath(t *testing.T) {
	h := newStreamHarness(t)
	h.seedAgentAsset(t, "tmp", "x.txt", "hi")
	body := `{"asset":"tmp/x.txt","new_path":"../escape"}`
	rr := h.post(t,
		fmt.Sprintf("/internal/agents/%s/drive/move", h.agentID),
		body,
		h.runtimeSecret,
	)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestMoveAgentAsset_BadBearer(t *testing.T) {
	h := newStreamHarness(t)
	body := `{"asset":"tmp/x.txt","new_path":"archive"}`
	rr := h.post(t,
		fmt.Sprintf("/internal/agents/%s/drive/move", h.agentID),
		body,
		"not-the-key",
	)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}
