package agentrouter

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/cache"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/registry"
	"github.com/usehivy/hivy/internal/trigger/hivy"
)

func TestRouteMultiAgentHighConfidenceRoutesToChoice(t *testing.T) {
	ada, grace := twoCandidates()
	mock := hivy.NewMockCompletionClient()
	mock.SetFallback(verdictJSON(t, grace.ID, 0.9, "data question"))
	router := testRouter(fakeCredential, mock)

	d := router.Route(context.Background(), Input{
		MessageText:    "what does last quarter revenue look like",
		Candidates:     []Candidate{ada, grace},
		RecentSessions: []RecentSession{{Name: "q3-revenue-pull", AgentName: "Grace"}},
		DefaultAgentID: ada.ID,
	})

	if d.Layer != LayerLLMRouter || d.AgentID != grace.ID || d.Confidence != 0.9 {
		t.Fatalf("expected LLM route to Grace @0.9, got %+v", d)
	}
	mock.AssertCallCount(t, 1)

	req := mock.LastRequest()
	if req.ResponseFormat == nil || req.ResponseFormat.JSONSchema == nil ||
		req.ResponseFormat.JSONSchema.Name != "agent_route" || !req.ResponseFormat.JSONSchema.Strict {
		t.Fatalf("expected strict agent_route schema, got %#v", req.ResponseFormat)
	}
	if !strings.Contains(req.Messages[1].Content, grace.ID.String()) {
		t.Fatal("user prompt should list candidate agent ids")
	}
	if !strings.Contains(req.Messages[1].Content, "q3-revenue-pull") {
		t.Fatal("user prompt should include recent session history")
	}
}

func TestRouteLowConfidenceFallsBackToDefault(t *testing.T) {
	ada, grace := twoCandidates()
	mock := hivy.NewMockCompletionClient()
	mock.SetFallback(verdictJSON(t, grace.ID, 0.3, "unsure"))
	router := testRouter(fakeCredential, mock)

	d := router.Route(context.Background(), Input{
		MessageText:    "ambiguous request",
		Candidates:     []Candidate{ada, grace},
		DefaultAgentID: ada.ID,
	})

	if d.Layer != LayerDefault || d.AgentID != ada.ID {
		t.Fatalf("expected default on low confidence, got layer=%s agent=%s", d.Layer, d.AgentID)
	}
}

func TestRouteNonCandidateChoiceFallsBackToDefault(t *testing.T) {
	ada, grace := twoCandidates()
	mock := hivy.NewMockCompletionClient()
	mock.SetFallback(verdictJSON(t, uuid.New(), 0.95, "a stranger")) // id not in candidates
	router := testRouter(fakeCredential, mock)

	d := router.Route(context.Background(), Input{
		MessageText:    "ambiguous request",
		Candidates:     []Candidate{ada, grace},
		DefaultAgentID: ada.ID,
	})

	if d.Layer != LayerDefault || d.AgentID != ada.ID {
		t.Fatalf("expected default on non-candidate choice, got layer=%s agent=%s", d.Layer, d.AgentID)
	}
}

func TestRouteLLMErrorFallsBackToDefault(t *testing.T) {
	ada, grace := twoCandidates()
	router := testRouter(fakeCredential, errorClient{err: fmt.Errorf("provider 500")})

	d := router.Route(context.Background(), Input{
		MessageText:    "ambiguous request",
		Candidates:     []Candidate{ada, grace},
		DefaultAgentID: ada.ID,
	})

	if d.Layer != LayerDefault || d.AgentID != ada.ID {
		t.Fatalf("expected default on LLM error, got layer=%s agent=%s", d.Layer, d.AgentID)
	}
}

func TestRouteLLMTimeoutFallsBackToDefault(t *testing.T) {
	ada, grace := twoCandidates()
	prev := routerTimeout
	routerTimeout = 20 * time.Millisecond
	t.Cleanup(func() { routerTimeout = prev })

	router := testRouter(fakeCredential, blockingClient{})

	start := time.Now()
	d := router.Route(context.Background(), Input{
		MessageText:    "ambiguous request",
		Candidates:     []Candidate{ada, grace},
		DefaultAgentID: ada.ID,
	})
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("routing did not honor the timeout, took %s", elapsed)
	}
	if d.Layer != LayerDefault || d.AgentID != ada.ID {
		t.Fatalf("expected default on timeout, got layer=%s agent=%s", d.Layer, d.AgentID)
	}
}

func TestRouteCredentialUnavailableFallsBackToDefault(t *testing.T) {
	ada, grace := twoCandidates()
	loader := func(context.Context, *gorm.DB, *cache.Manager, *registry.Registry, string) (*routerCredential, error) {
		return nil, fmt.Errorf("no system credential")
	}
	router := testRouter(loader, hivy.NewMockCompletionClient())

	d := router.Route(context.Background(), Input{
		MessageText:    "ambiguous request",
		Candidates:     []Candidate{ada, grace},
		DefaultAgentID: ada.ID,
	})

	if d.Layer != LayerDefault || d.AgentID != ada.ID {
		t.Fatalf("expected default when credential unavailable, got layer=%s agent=%s", d.Layer, d.AgentID)
	}
}

func TestRouteNilCacheManagerSkipsLLM(t *testing.T) {
	ada, grace := twoCandidates()
	router := New(&gorm.DB{}, nil) // no cache manager → LLM layer disabled

	d := router.Route(context.Background(), Input{
		MessageText:    "ambiguous request",
		Candidates:     []Candidate{ada, grace},
		DefaultAgentID: ada.ID,
	})

	if d.Layer != LayerDefault || d.AgentID != ada.ID {
		t.Fatalf("expected default with nil cache manager, got layer=%s agent=%s", d.Layer, d.AgentID)
	}
}

func TestRouteEmitsDecisionLog(t *testing.T) {
	ada, grace := twoCandidates()
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx := logging.WithLogger(context.Background(), logger)

	mock := hivy.NewMockCompletionClient()
	mock.SetFallback(verdictJSON(t, grace.ID, 0.88, "topical continuity"))
	router := testRouter(fakeCredential, mock)

	router.Route(ctx, Input{
		MessageText:    "quarterly numbers please",
		Candidates:     []Candidate{ada, grace},
		DefaultAgentID: ada.ID,
	})

	out := buf.String()
	if !strings.Contains(out, "agent_router_decision") {
		t.Fatalf("expected decision log line, got: %s", out)
	}
	if !strings.Contains(out, LayerLLMRouter) || !strings.Contains(out, grace.ID.String()) {
		t.Fatalf("decision log missing layer/agent fields: %s", out)
	}
	if !strings.Contains(out, "topical continuity") {
		t.Fatalf("decision log missing reason: %s", out)
	}
}
