package handler_test

import (
	"encoding/json"
	"math"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

func TestSessionUsage_ReturnsModelUsageTotals(t *testing.T) {
	h := newSessionHarness(t)
	fx := h.seed(t)
	created := h.createSession(t, fx, fx.owner, "hello")
	sessionID := uuid.MustParse(created.Session.ID)
	now := time.Now().UTC()

	events := []model.SessionEvent{
		{
			OrgID:          fx.org.ID,
			SessionID:      sessionID,
			AgentID:        fx.agent.ID,
			EventID:        "evt-usage-1",
			EventType:      "model_usage",
			Source:         "runtime",
			SequenceNumber: 10,
			Payload: model.JSON{
				"usage": map[string]any{"cost": 0.0012},
			},
			EventAt: now,
		},
		{
			OrgID:          fx.org.ID,
			SessionID:      sessionID,
			AgentID:        fx.agent.ID,
			EventID:        "evt-usage-2",
			EventType:      "model_usage",
			Source:         "runtime",
			SequenceNumber: 12,
			Payload: model.JSON{
				"usage": map[string]any{"cost": 0.0023},
			},
			EventAt: now.Add(time.Second),
		},
		{
			OrgID:          fx.org.ID,
			SessionID:      sessionID,
			AgentID:        fx.agent.ID,
			EventID:        "evt-token",
			EventType:      "token",
			Source:         "runtime",
			SequenceNumber: 13,
			Payload: model.JSON{
				"usage": map[string]any{"cost": 99},
			},
			EventAt: now.Add(2 * time.Second),
		},
	}
	if err := h.db.Create(&events).Error; err != nil {
		t.Fatalf("seed session usage events: %v", err)
	}

	rr := h.doJSON(t, http.MethodGet, "/v1/sessions/"+created.Session.ID+"/usage", fx, fx.owner, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(rr.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode usage response map: %v", err)
	}
	if len(raw) != 2 || raw["cost_usd"] == nil || raw["credits"] == nil {
		t.Fatalf("response keys=%v, want only cost_usd and credits", raw)
	}

	var resp struct {
		CostUSD float64 `json:"cost_usd"`
		Credits int64   `json:"credits"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode usage response: %v", err)
	}
	if math.Abs(resp.CostUSD-0.0035) > 0.0000001 {
		t.Fatalf("cost_usd=%f, want 0.0035", resp.CostUSD)
	}
	if resp.Credits != 4 {
		t.Fatalf("credits=%d, want 4", resp.Credits)
	}
}
