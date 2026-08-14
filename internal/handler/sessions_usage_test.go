package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/sandbox"
)

func TestSessionUsageReturnsFractionalSandboxComputeSpend(t *testing.T) {
	db := connectTestDB(t)
	org := createTestOrg(t, db)
	team := seedTeam(t, db, org.ID, "session-usage")
	agent := seedTeamAgent(t, db, org.ID, team.ID)
	session := model.Session{
		ID: uuid.New(), OrgID: org.ID, TeamID: team.ID, AgentID: agent.ID,
		Status: "active", AgentTurnStatus: model.SessionAgentTurnIdle,
	}
	if err := db.Create(&session).Error; err != nil {
		t.Fatalf("create session: %v", err)
	}
	startedAt := time.Now().UTC().Add(-30 * time.Second)
	usage := model.SandboxTurnUsage{
		OrgID: org.ID, SessionID: session.ID, TurnID: "turn-1",
		SandboxVCPU: 1, PricingVersion: 1, CreditsPerVCPUMinute: 1,
		StartedAt: startedAt, ObservedThrough: startedAt.Add(30 * time.Second),
		ActiveMilliseconds: 30_000,
	}
	if err := db.Create(&usage).Error; err != nil {
		t.Fatalf("create sandbox usage: %v", err)
	}
	t.Cleanup(func() {
		db.Where("session_id = ?", session.ID).Delete(&model.SandboxTurnUsage{})
		db.Where("id = ?", session.ID).Delete(&model.Session{})
		db.Where("id = ?", agent.ID).Delete(&model.Agent{})
		db.Where("id = ?", team.ID).Delete(&model.Team{})
	})

	sessionHandler := handler.NewSessionHandler(db)
	router := chi.NewRouter()
	router.Get("/v1/sessions/{id}/usage", sessionHandler.GetUsage)
	req := httptest.NewRequest(http.MethodGet, "/v1/sessions/"+session.ID.String()+"/usage", nil)
	req = middleware.WithOrg(req, &org)
	req = middleware.WithAPIKeyClaims(req, &middleware.APIKeyClaims{
		OrgID: org.ID.String(), Scopes: []string{"sessions"},
	})
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rr.Code, rr.Body.String())
	}
	var response struct {
		CostUSD            float64 `json:"cost_usd"`
		Credits            float64 `json:"credits"`
		SandboxCostUSD     float64 `json:"sandbox_cost_usd"`
		SandboxCredits     float64 `json:"sandbox_credits"`
		SandboxVCPUSeconds int64   `json:"sandbox_vcpu_seconds"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&response); err != nil {
		t.Fatalf("decode usage response: %v", err)
	}
	if response.SandboxCredits != 0.5 || response.SandboxCostUSD != 0.0005 || response.SandboxVCPUSeconds != 30 {
		t.Fatalf("sandbox usage = %+v, want 0.5 credits, $0.0005, and 30 vCPU-seconds", response)
	}
	if response.Credits != response.SandboxCredits || response.CostUSD != response.SandboxCostUSD {
		t.Fatalf("total usage = %+v, want sandbox-only totals", response)
	}
}

func TestSessionUsageDoesNotChargeDesktopCompute(t *testing.T) {
	db := connectTestDB(t)
	org := createTestOrg(t, db)
	team := seedTeam(t, db, org.ID, "desktop-session-usage")
	agent := seedTeamAgent(t, db, org.ID, team.ID)
	sb := model.Sandbox{
		ID: uuid.New(), OrgID: &org.ID, AgentID: &agent.ID,
		ProviderID: sandbox.ProviderDesktop, ExternalID: "desktop-test",
		RuntimeURL: "desktop://localhost", EncryptedRuntimeSecret: []byte{1}, Status: "running",
	}
	if err := db.Create(&sb).Error; err != nil {
		t.Fatalf("create desktop sandbox: %v", err)
	}
	session := model.Session{
		ID: uuid.New(), OrgID: org.ID, TeamID: team.ID, AgentID: agent.ID, SandboxID: &sb.ID,
		Status: "active", AgentTurnStatus: model.SessionAgentTurnIdle,
	}
	if err := db.Create(&session).Error; err != nil {
		t.Fatalf("create desktop session: %v", err)
	}
	startedAt := time.Now().UTC().Add(-30 * time.Second)
	usage := model.SandboxTurnUsage{
		OrgID: org.ID, SessionID: session.ID, TurnID: "turn-desktop",
		SandboxVCPU: 1, PricingVersion: 1, CreditsPerVCPUMinute: 1,
		StartedAt: startedAt, ObservedThrough: startedAt.Add(30 * time.Second),
		ActiveMilliseconds: 30_000,
	}
	if err := db.Create(&usage).Error; err != nil {
		t.Fatalf("create prior desktop usage: %v", err)
	}
	t.Cleanup(func() {
		db.Where("session_id = ?", session.ID).Delete(&model.SandboxTurnUsage{})
		db.Where("id = ?", session.ID).Delete(&model.Session{})
		db.Where("id = ?", sb.ID).Delete(&model.Sandbox{})
		db.Where("id = ?", agent.ID).Delete(&model.Agent{})
		db.Where("id = ?", team.ID).Delete(&model.Team{})
	})

	sessionHandler := handler.NewSessionHandler(db)
	router := chi.NewRouter()
	router.Get("/v1/sessions/{id}/usage", sessionHandler.GetUsage)
	req := httptest.NewRequest(http.MethodGet, "/v1/sessions/"+session.ID.String()+"/usage", nil)
	req = middleware.WithOrg(req, &org)
	req = middleware.WithAPIKeyClaims(req, &middleware.APIKeyClaims{
		OrgID: org.ID.String(), Scopes: []string{"sessions"},
	})
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rr.Code, rr.Body.String())
	}
	var response struct {
		SandboxCostUSD     float64 `json:"sandbox_cost_usd"`
		SandboxCredits     float64 `json:"sandbox_credits"`
		SandboxVCPUSeconds int64   `json:"sandbox_vcpu_seconds"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&response); err != nil {
		t.Fatalf("decode usage response: %v", err)
	}
	if response.SandboxCredits != 0 || response.SandboxCostUSD != 0 || response.SandboxVCPUSeconds != 0 {
		t.Fatalf("desktop sandbox usage = %+v, want no hosted compute charge", response)
	}
}
