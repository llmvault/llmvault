package agentrouter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/cache"
	"github.com/usehivy/hivy/internal/credentials"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/registry"
	"github.com/usehivy/hivy/internal/trigger/hivy"
)

// routerTimeout bounds a single LLM routing call. Routing latency must never
// hold up an inbound message beyond this — on timeout we fall back to default.
// It is a var (not a const) so tests can shrink it to exercise the timeout path.
var routerTimeout = 10 * time.Second

// routerMaxTokens caps the router's output; the JSON verdict is tiny.
const routerMaxTokens = 200

var errRouterModelUnavailable = errors.New("router model unavailable")

const routerResponseSchema = `{
	"type": "object",
	"additionalProperties": false,
	"properties": {
		"agent_id": {"type": "string"},
		"confidence": {"type": "number"},
		"reason": {"type": "string"}
	},
	"required": ["agent_id", "confidence", "reason"]
}`

type routerVerdict struct {
	AgentID    string  `json:"agent_id"`
	Confidence float64 `json:"confidence"`
	Reason     string  `json:"reason"`
}

type routerCredential struct {
	credential *model.Credential
	apiKey     string
	modelID    string
}

// routeViaLLM runs the fast LLM router. The bool is false whenever routing
// should fall back to the channel default agent: missing dependencies,
// credential/model unavailability, provider error or timeout, malformed output,
// an agent outside the candidate list, or confidence below the threshold.
func (r *Router) routeViaLLM(ctx context.Context, in Input) (Decision, bool) {
	if r.db == nil || r.cacheManager == nil {
		return Decision{}, false
	}

	loader := r.loadCredential
	if loader == nil {
		loader = loadRouterCredential
	}
	config, err := loader(ctx, r.db, r.cacheManager, r.registry(), r.canonicalModel())
	if err != nil {
		logging.FromContext(ctx).WarnContext(ctx, "agent_router_credential_unavailable", "error", err)
		return Decision{}, false
	}

	factory := r.newClient
	if factory == nil {
		factory = hivy.NewCompletionClient
	}
	client := factory(config.credential, config.apiKey)

	callCtx, cancel := context.WithTimeout(ctx, routerTimeout)
	defer cancel()

	verdict, err := callRouterCompletion(callCtx, client, config.modelID, in)
	if err != nil {
		logging.FromContext(ctx).WarnContext(ctx, "agent_router_call_failed", "error", err)
		return Decision{}, false
	}

	id, err := uuid.Parse(strings.TrimSpace(verdict.AgentID))
	if err != nil || !isCandidate(id, in.Candidates) {
		logging.FromContext(ctx).WarnContext(ctx, "agent_router_invalid_choice",
			"agent_id", verdict.AgentID)
		return Decision{}, false
	}
	if verdict.Confidence < ConfidenceThreshold {
		logging.FromContext(ctx).InfoContext(ctx, "agent_router_low_confidence",
			"agent_id", id.String(), "confidence", verdict.Confidence)
		return Decision{}, false
	}

	return Decision{
		AgentID:    id,
		Confidence: verdict.Confidence,
		Reason:     verdict.Reason,
		Layer:      LayerLLMRouter,
	}, true
}

func (r *Router) registry() *registry.Registry {
	if r != nil && r.reg != nil {
		return r.reg
	}
	return registry.Global()
}

func (r *Router) canonicalModel() string {
	if r != nil && r.modelID != "" {
		return r.modelID
	}
	return ModelID
}

func callRouterCompletion(ctx context.Context, client hivy.CompletionClient, modelID string, in Input) (routerVerdict, error) {
	systemPrompt := strings.Join([]string{
		"You are a message router for a shared team channel.",
		"Several AI agents are assigned to this channel, each with a specific role.",
		"Given the assigned agents (with roles) and the channel's recent session history, pick the SINGLE best agent to handle the new message.",
		"Consider topical continuity: if the message continues or relates to work an agent recently handled, prefer that agent.",
		"You MUST choose exactly one agent from the provided list, identified by its exact agent_id. Never invent an agent_id.",
		`Respond with STRICT JSON only, no prose, in the shape {"agent_id":"<uuid>","confidence":<0..1>,"reason":"<short>"}.`,
		"confidence reflects how sure you are; use a low value when the message is ambiguous or fits no agent well.",
	}, " ")

	req := hivy.CompletionRequest{
		Model: modelID,
		Messages: []hivy.Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: buildRouterUserPrompt(in)},
		},
		MaxTokens:   routerMaxTokens,
		Temperature: floatPtr(0),
		ResponseFormat: &hivy.ResponseFormat{
			Type: hivy.ResponseFormatJSONSchema,
			JSONSchema: &hivy.ResponseJSONSchema{
				Name:   "agent_route",
				Schema: json.RawMessage(routerResponseSchema),
				Strict: true,
			},
		},
	}

	resp, err := client.ChatCompletion(ctx, req)
	if err != nil {
		return routerVerdict{}, err
	}

	var verdict routerVerdict
	if err := json.Unmarshal([]byte(strings.TrimSpace(resp.Message.Content)), &verdict); err != nil {
		return routerVerdict{}, fmt.Errorf("decode router verdict json: %w", err)
	}
	return verdict, nil
}

func buildRouterUserPrompt(in Input) string {
	var b strings.Builder
	b.WriteString("Assigned agents:\n")
	for _, candidate := range in.Candidates {
		b.WriteString("- id: ")
		b.WriteString(candidate.ID.String())
		b.WriteString(" | name: ")
		b.WriteString(candidate.Name)
		role := strings.TrimSpace(candidate.Role)
		if role != "" {
			b.WriteString(" | role: ")
			b.WriteString(collapseWhitespace(role))
		}
		b.WriteString("\n")
	}

	if len(in.RecentSessions) > 0 {
		b.WriteString("\nRecent sessions (most recent first):\n")
		for _, session := range in.RecentSessions {
			name := strings.TrimSpace(session.Name)
			if name == "" {
				name = "(untitled)"
			}
			b.WriteString("- \"")
			b.WriteString(name)
			b.WriteString("\" handled by ")
			b.WriteString(session.AgentName)
			b.WriteString("\n")
		}
	}

	b.WriteString("\nNew message:\n")
	b.WriteString(strings.TrimSpace(in.MessageText))
	return b.String()
}

func collapseWhitespace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func floatPtr(f float32) *float32 { return &f }

// loadRouterCredential resolves the platform OpenAI system credential for
// the router model, mirroring the session-naming credential flow.
func loadRouterCredential(
	ctx context.Context,
	db *gorm.DB,
	cacheManager *cache.Manager,
	reg *registry.Registry,
	modelID string,
) (*routerCredential, error) {
	if db == nil || cacheManager == nil {
		return nil, fmt.Errorf("missing router dependencies")
	}
	if reg == nil {
		reg = registry.Global()
	}
	if modelID == "" {
		modelID = ModelID
	}

	if _, ok := reg.ResolveModel(ProviderID, modelID); !ok {
		return nil, fmt.Errorf("%w: provider=%q model=%q", errRouterModelUnavailable, ProviderID, modelID)
	}

	credential, err := pickRouterCredential(ctx, db, reg, modelID)
	if err != nil {
		return nil, err
	}

	route, _ := reg.ResolveModel(credential.ProviderID, modelID)

	decrypted, err := cacheManager.GetDecryptedCredentialByID(ctx, credential.ID.String())
	if err != nil {
		return nil, fmt.Errorf("decrypt router credential: %w", err)
	}

	return &routerCredential{
		credential: credential,
		apiKey:     string(decrypted.APIKey),
		modelID:    route.UpstreamID,
	}, nil
}

func pickRouterCredential(
	ctx context.Context,
	db *gorm.DB,
	reg *registry.Registry,
	modelID string,
) (*model.Credential, error) {
	if reg == nil {
		reg = registry.Global()
	}
	if err := reg.ValidateCanonicalModel(modelID); err != nil {
		return nil, err
	}

	var creds []model.Credential
	if err := db.WithContext(ctx).
		Where("org_id IS NULL AND revoked_at IS NULL AND provider_id = ?", ProviderID).
		Order("created_at ASC").
		Find(&creds).Error; err != nil {
		return nil, fmt.Errorf("list router credentials: %w", err)
	}

	for index := range creds {
		if _, ok := reg.ResolveModel(creds[index].ProviderID, modelID); ok {
			return &creds[index], nil
		}
	}

	return nil, fmt.Errorf("%w: provider=%q model=%q", credentials.ErrNoSystemCredential, ProviderID, modelID)
}
