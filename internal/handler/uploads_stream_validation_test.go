package handler_test

import (
	"bytes"
	"fmt"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

func TestStreamAsset_BadBearer(t *testing.T) {
	h := newStreamHarness(t)
	rr := h.put(t,
		fmt.Sprintf("/internal/agents/%s/drive/x.png", h.agentID),
		bytes.NewReader([]byte("hi")),
		"image/png",
		"not-the-real-key",
	)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestStreamAsset_MissingBearer(t *testing.T) {
	h := newStreamHarness(t)
	rr := h.put(t,
		fmt.Sprintf("/internal/agents/%s/drive/x.png", h.agentID),
		bytes.NewReader([]byte("hi")),
		"image/png",
		"",
	)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestStreamAsset_AgentNotFound(t *testing.T) {
	h := newStreamHarness(t)
	rr := h.put(t,
		fmt.Sprintf("/internal/agents/%s/drive/x.png", uuid.New()),
		bytes.NewReader([]byte("hi")),
		"image/png",
		h.runtimeSecret,
	)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

func TestStreamAsset_PathTraversalRejected(t *testing.T) {
	h := newStreamHarness(t)
	rr := h.put(t,
		fmt.Sprintf("/internal/agents/%s/drive/../../etc/passwd", h.agentID),
		bytes.NewReader([]byte("hi")),
		"text/plain",
		h.runtimeSecret,
	)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestStreamAsset_FilenameRequired(t *testing.T) {
	h := newStreamHarness(t)
	rr := h.put(t,
		fmt.Sprintf("/internal/agents/%s/drive/", h.agentID),
		bytes.NewReader([]byte("x")),
		"text/plain",
		h.runtimeSecret,
	)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestStreamAsset_RejectsEmptyFile(t *testing.T) {
	h := newStreamHarness(t)
	rr := h.put(t,
		fmt.Sprintf("/internal/agents/%s/drive/images/empty.jpg", h.agentID),
		bytes.NewReader(nil),
		"image/jpeg",
		h.runtimeSecret,
	)
	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", rr.Code, rr.Body.String())
	}

	var count int64
	h.db.Model(&model.AgentAsset{}).
		Where("agent_id = ? AND path = ? AND filename = ?", h.agentID, "images", "empty.jpg").
		Count(&count)
	if count != 0 {
		t.Fatalf("empty upload should not create asset row, count=%d", count)
	}
}
