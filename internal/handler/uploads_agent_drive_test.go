package handler_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/usehivy/hivy/internal/model"
)

func TestStreamAgentDrive_HappyPath(t *testing.T) {
	h := newStreamHarness(t)
	rr := h.put(t, h.drivePath("reports/2026/summary.txt"), bytes.NewReader([]byte("hello drive")), "text/plain", h.runtimeSecret)
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		PublicURL string `json:"asset_url"`
		Path      string `json:"path"`
		Filename  string `json:"filename"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Path != "reports/2026" || resp.Filename != "summary.txt" || resp.PublicURL == "" {
		t.Fatalf("response = %+v", resp)
	}
	if !strings.HasPrefix(resp.PublicURL, "https://api.usehivy.test/v1/assets/preview?path=") {
		t.Fatalf("expected preview asset_url, got %q", resp.PublicURL)
	}

	var asset model.AgentAsset
	if err := h.db.Where("agent_id = ? AND path = ? AND filename = ?", h.agentID, "reports/2026", "summary.txt").First(&asset).Error; err != nil {
		t.Fatalf("load agent drive file: %v", err)
	}
	if asset.Key != fmt.Sprintf("pub/e/%s/reports/2026/summary.txt", h.agentID) {
		t.Fatalf("asset key = %q", asset.Key)
	}
}
