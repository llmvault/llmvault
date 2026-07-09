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

func TestSessionUsage_ReturnsSessionDebitedCredits(t *testing.T) {
	h := newSessionHarness(t)
	fx := h.seed(t)
	created := h.createSession(t, fx, fx.owner, "hello")
	sessionID := uuid.MustParse(created.Session.ID)
	otherSession := uuid.New()
	now := time.Now().UTC()
	billed := now

	gens := []model.Generation{
		{ID: uuid.NewString(), OrgID: fx.org.ID, CredentialID: uuid.New(), TokenJTI: "t1", ProviderID: "openrouter", Model: "m", SessionID: &sessionID, IsSystem: true, Cost: 0.003, CreditsDebited: 3, BilledAt: &billed, CreatedAt: now},
		{ID: uuid.NewString(), OrgID: fx.org.ID, CredentialID: uuid.New(), TokenJTI: "t2", ProviderID: "openrouter", Model: "m", SessionID: &sessionID, IsSystem: true, Cost: 0.0012, CreatedAt: now},
		{ID: uuid.NewString(), OrgID: fx.org.ID, CredentialID: uuid.New(), TokenJTI: "t3", ProviderID: "openrouter", Model: "m", SessionID: &sessionID, IsSystem: false, Cost: 0.005, CreatedAt: now},
		{ID: uuid.NewString(), OrgID: fx.org.ID, CredentialID: uuid.New(), TokenJTI: "t4", ProviderID: "openrouter", Model: "m", SessionID: &otherSession, IsSystem: true, Cost: 0.01, CreatedAt: now},
	}
	if err := h.db.Create(&gens).Error; err != nil {
		t.Fatalf("seed generations: %v", err)
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
	if resp.Credits != 5 {
		t.Fatalf("credits=%d, want 5 (debited 3 + unbilled estimate 2)", resp.Credits)
	}
	if math.Abs(resp.CostUSD-0.005) > 0.0000001 {
		t.Fatalf("cost_usd=%f, want 0.005", resp.CostUSD)
	}
}
