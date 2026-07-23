// Package agentrouter resolves which agent should handle an inbound channel
// message when several agents are assigned to the same channel.
//
// It implements the deterministic layers of channel routing that do not depend
// on the caller's transport (Slack, web, etc.):
//
//  1. Name match — the message names exactly one assigned agent.
//  2. Single agent — the channel has exactly one assigned agent.
//  3. Fast LLM router — a low-latency model picks the best agent from the
//     assigned candidates, considering recent session history.
//  4. Default — used whenever the router is unavailable, errors, times out, or
//     returns low confidence / an agent outside the assigned list.
//
// Thread affinity (a message continuing an existing session) is handled by the
// caller before this package is consulted and always wins.
//
// The router never selects an agent outside the supplied candidate list. Paired
// with the caller only ever passing a channel's assigned agents, this makes it
// impossible to invoke an agent in a channel it is not assigned to.
package agentrouter

import (
	"context"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/cache"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/registry"
	"github.com/usehivy/hivy/internal/trigger/hivy"
)

const (
	// ProviderID and ModelID mirror the session-naming flow: a fast, cheap
	// direct-provider model with a strict JSON schema. ModelID is a canonical id
	// resolved through the registry; override via Router.modelID if needed.
	ProviderID = "openai"
	ModelID    = "gpt-4o-mini"

	// ConfidenceThreshold is the minimum confidence the LLM router must report
	// for its choice to be honored. Below it, routing falls back to the channel
	// default agent.
	ConfidenceThreshold = 0.6

	// RecentSessionsLimit bounds how many prior sessions we feed the router as
	// topical-continuity context.
	RecentSessionsLimit = 10
)

// Layer identifies which routing layer produced a decision. It is emitted in
// the structured decision log and is useful in tests.
const (
	LayerNameMatch   = "name_match"
	LayerSingleAgent = "single_agent"
	LayerLLMRouter   = "llm_router"
	LayerDefault     = "default"
)

// Candidate is an agent assigned to the channel that the router may choose.
// Role is the agent's short description (falling back to its instructions) and
// gives the LLM router topical signal.
type Candidate struct {
	ID   uuid.UUID
	Name string
	Role string
}

// RecentSession is a compact line of channel history fed to the LLM router for
// topical continuity.
type RecentSession struct {
	Name      string
	AgentName string
}

// Input is everything the router needs to make a decision.
type Input struct {
	MessageText    string
	Candidates     []Candidate
	RecentSessions []RecentSession
	// DefaultAgentID is the channel's default agent, used as the ultimate
	// fallback. It is guaranteed by the caller to be an assigned agent.
	DefaultAgentID uuid.UUID
}

// Decision is the routing outcome. AgentID is always a candidate (or the
// channel default). Layer records which layer decided.
type Decision struct {
	AgentID    uuid.UUID
	Confidence float64
	Reason     string
	Layer      string
}

type credentialLoader func(ctx context.Context, db *gorm.DB, cacheManager *cache.Manager, reg *registry.Registry, modelID string) (*routerCredential, error)

type clientFactory func(credential *model.Credential, apiKey string) hivy.CompletionClient

// Router resolves an agent for an inbound message. Construct it with New. The
// zero value is not usable.
type Router struct {
	db             *gorm.DB
	cacheManager   *cache.Manager
	reg            *registry.Registry
	modelID        string
	loadCredential credentialLoader
	newClient      clientFactory
}

// New builds a Router. cacheManager may be nil — the deterministic layers still
// work, and the LLM layer degrades to the default agent.
func New(db *gorm.DB, cacheManager *cache.Manager) *Router {
	return &Router{
		db:             db,
		cacheManager:   cacheManager,
		reg:            registry.Global(),
		modelID:        ModelID,
		loadCredential: loadRouterCredential,
		newClient:      hivy.NewCompletionClient,
	}
}

// Route returns the agent that should handle the message. It never returns an
// error: any failure resolves to the channel default agent so an inbound
// message is never blocked by routing.
func (r *Router) Route(ctx context.Context, in Input) Decision {
	def := Decision{
		AgentID:    in.DefaultAgentID,
		Confidence: 0,
		Reason:     "channel default agent",
		Layer:      LayerDefault,
	}

	if len(in.Candidates) == 0 {
		return r.logDecision(ctx, in, def)
	}

	// Layer 2 — the message names exactly one assigned agent.
	if match, ok := matchByName(in.MessageText, in.Candidates); ok {
		return r.logDecision(ctx, in, Decision{
			AgentID:    match.ID,
			Confidence: 1,
			Reason:     "message names assigned agent " + match.Name,
			Layer:      LayerNameMatch,
		})
	}

	// Layer 3 — a single assigned agent short-circuits before any LLM call.
	if len(in.Candidates) == 1 {
		return r.logDecision(ctx, in, Decision{
			AgentID:    in.Candidates[0].ID,
			Confidence: 1,
			Reason:     "channel has a single assigned agent",
			Layer:      LayerSingleAgent,
		})
	}

	// Layer 4 — fast LLM router across the assigned candidates.
	if decision, ok := r.routeViaLLM(ctx, in); ok {
		return r.logDecision(ctx, in, decision)
	}

	// Layer 5 — default agent on any router miss.
	return r.logDecision(ctx, in, def)
}

func (r *Router) logDecision(ctx context.Context, in Input, d Decision) Decision {
	logging.FromContext(ctx).InfoContext(ctx, "agent_router_decision",
		"layer", d.Layer,
		"agent_id", d.AgentID.String(),
		"confidence", d.Confidence,
		"reason", d.Reason,
		"candidate_count", len(in.Candidates),
	)
	return d
}

// matchByName reports the single candidate whose name appears as a whole word
// (case-insensitive) in the message. If zero or more than one candidate match,
// it reports no match — an ambiguous mention must fall through to later layers.
func matchByName(message string, candidates []Candidate) (Candidate, bool) {
	if strings.TrimSpace(message) == "" {
		return Candidate{}, false
	}
	var matched Candidate
	count := 0
	for _, candidate := range candidates {
		name := strings.TrimSpace(candidate.Name)
		if name == "" {
			continue
		}
		if wholeWordMatch(message, name) {
			matched = candidate
			count++
		}
	}
	if count == 1 {
		return matched, true
	}
	return Candidate{}, false
}

func wholeWordMatch(message, name string) bool {
	pattern := `(?i)(^|[^\p{L}\p{N}_])` + regexp.QuoteMeta(name) + `($|[^\p{L}\p{N}_])`
	re, err := regexp.Compile(pattern)
	if err != nil {
		return false
	}
	return re.MatchString(message)
}

func isCandidate(id uuid.UUID, candidates []Candidate) bool {
	for _, candidate := range candidates {
		if candidate.ID == id {
			return true
		}
	}
	return false
}
