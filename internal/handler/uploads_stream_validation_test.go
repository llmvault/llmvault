package handler_test

import (
	"bytes"
	"fmt"
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func TestStreamAsset_BadBearer(t *testing.T) {
	h := newStreamHarness(t)
	rr := h.put(t,
		fmt.Sprintf("/internal/employees/%s/drive/x.png", h.agentID),
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
		fmt.Sprintf("/internal/employees/%s/drive/x.png", h.agentID),
		bytes.NewReader([]byte("hi")),
		"image/png",
		"",
	)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestStreamAsset_EmployeeNotFound(t *testing.T) {
	h := newStreamHarness(t)
	rr := h.put(t,
		fmt.Sprintf("/internal/employees/%s/drive/x.png", uuid.New()),
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
		fmt.Sprintf("/internal/employees/%s/drive/../../etc/passwd", h.agentID),
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
		fmt.Sprintf("/internal/employees/%s/drive/", h.agentID),
		bytes.NewReader([]byte("x")),
		"text/plain",
		h.runtimeSecret,
	)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}
