package agentrouter

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/cache"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/registry"
	"github.com/usehivy/hivy/internal/trigger/hivy"
)

// --- test doubles -----------------------------------------------------------

type errorClient struct{ err error }

func (c errorClient) ChatCompletion(context.Context, hivy.CompletionRequest) (*hivy.CompletionResponse, error) {
	return nil, c.err
}

// blockingClient blocks until the call context is cancelled, so a shrunken
// routerTimeout drives the real timeout code path.
type blockingClient struct{}

func (blockingClient) ChatCompletion(ctx context.Context, _ hivy.CompletionRequest) (*hivy.CompletionResponse, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func fakeCredential(context.Context, *gorm.DB, *cache.Manager, *registry.Registry, string) (*routerCredential, error) {
	return &routerCredential{
		credential: &model.Credential{ProviderID: ProviderID},
		apiKey:     "sk-test",
		modelID:    "openai/gpt-4o-mini",
	}, nil
}

// testRouter builds a Router with the DB/cache guards satisfied but all IO
// faked, so the deterministic layers and the LLM layer run without a database.
func testRouter(loader credentialLoader, client hivy.CompletionClient) *Router {
	return &Router{
		db:             &gorm.DB{},
		cacheManager:   &cache.Manager{},
		reg:            registry.Global(),
		modelID:        ModelID,
		loadCredential: loader,
		newClient:      func(*model.Credential, string) hivy.CompletionClient { return client },
	}
}

func verdictJSON(t *testing.T, agentID uuid.UUID, confidence float64, reason string) hivy.CompletionResponse {
	t.Helper()
	raw, err := json.Marshal(routerVerdict{AgentID: agentID.String(), Confidence: confidence, Reason: reason})
	if err != nil {
		t.Fatalf("marshal verdict: %v", err)
	}
	return hivy.CompletionResponse{Message: hivy.Message{Content: string(raw)}}
}

func twoCandidates() (Candidate, Candidate) {
	return Candidate{ID: uuid.New(), Name: "Ada", Role: "backend engineer"},
		Candidate{ID: uuid.New(), Name: "Grace", Role: "data analyst"}
}

// --- deterministic layers ---------------------------------------------------

func TestRouteExactNameMatchRoutesWithoutLLM(t *testing.T) {
	ada, grace := twoCandidates()
	mock := hivy.NewMockCompletionClient()
	router := testRouter(fakeCredential, mock)

	d := router.Route(context.Background(), Input{
		MessageText:    "hey Grace can you pull the numbers",
		Candidates:     []Candidate{ada, grace},
		DefaultAgentID: ada.ID,
	})

	if d.Layer != LayerNameMatch || d.AgentID != grace.ID {
		t.Fatalf("expected name match to Grace, got layer=%s agent=%s", d.Layer, d.AgentID)
	}
	mock.AssertCallCount(t, 0)
}

func TestRouteAmbiguousNameMatchFallsThroughToLLM(t *testing.T) {
	ada, grace := twoCandidates()
	mock := hivy.NewMockCompletionClient()
	mock.SetFallback(verdictJSON(t, ada.ID, 0.8, "picked ada"))
	router := testRouter(fakeCredential, mock)

	// Names both agents → ambiguous → no name match → multi-agent → LLM.
	d := router.Route(context.Background(), Input{
		MessageText:    "Ada and Grace should sync on this",
		Candidates:     []Candidate{ada, grace},
		DefaultAgentID: ada.ID,
	})

	if d.Layer != LayerLLMRouter || d.AgentID != ada.ID {
		t.Fatalf("expected LLM router to Ada, got layer=%s agent=%s", d.Layer, d.AgentID)
	}
	mock.AssertCallCount(t, 1)
}

func TestRouteSingleAssignedAgentShortCircuits(t *testing.T) {
	only := Candidate{ID: uuid.New(), Name: "Solo", Role: "generalist"}
	mock := hivy.NewMockCompletionClient()
	loaderCalled := false
	loader := func(ctx context.Context, db *gorm.DB, cm *cache.Manager, reg *registry.Registry, m string) (*routerCredential, error) {
		loaderCalled = true
		return fakeCredential(ctx, db, cm, reg, m)
	}
	router := testRouter(loader, mock)

	d := router.Route(context.Background(), Input{
		MessageText:    "no name mentioned here at all",
		Candidates:     []Candidate{only},
		DefaultAgentID: only.ID,
	})

	if d.Layer != LayerSingleAgent || d.AgentID != only.ID {
		t.Fatalf("expected single-agent short-circuit, got layer=%s agent=%s", d.Layer, d.AgentID)
	}
	mock.AssertCallCount(t, 0)
	if loaderCalled {
		t.Fatal("credential loader must not be called for a single-agent channel")
	}
}

func TestRouteNoCandidatesReturnsDefault(t *testing.T) {
	def := uuid.New()
	router := testRouter(fakeCredential, hivy.NewMockCompletionClient())
	d := router.Route(context.Background(), Input{MessageText: "anything", DefaultAgentID: def})
	if d.Layer != LayerDefault || d.AgentID != def {
		t.Fatalf("expected default, got layer=%s agent=%s", d.Layer, d.AgentID)
	}
}
