package handler

import (
	"context"

	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/hindsight"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/registry"
	"github.com/usehivy/hivy/internal/sandbox"
)

const (
	agentHarness            = "agent-sandbox"
	hivyAgentName           = "Hivy"
	hivyAgentDescription    = "Hivy is the organization's managed AI agent."
	hivyAgentAvatarURL      = "/assets/hivy-avatar.png"
	agentStrategyAlwaysOn   = "always_on"
	agentStrategyPerSession = "per_session"
)

type AgentHandler struct {
	db           *gorm.DB
	orchestrator *sandbox.Orchestrator
	compileDeps  agentruntime.CompileDeps
	registry     *registry.Registry
	enqueuer     enqueue.TaskEnqueuer
	taskCleaner  enqueue.TaskCleaner
	memoryBanks  *hindsight.BankProvisioner
}

func NewAgentHandler(db *gorm.DB, orchestrator *sandbox.Orchestrator, compileDeps agentruntime.CompileDeps, reg *registry.Registry) *AgentHandler {
	return &AgentHandler{
		db:           db,
		orchestrator: orchestrator,
		compileDeps:  compileDeps,
		registry:     reg,
	}
}

func (h *AgentHandler) SetEnqueuer(enq enqueue.TaskEnqueuer) {
	h.enqueuer = enq
	if cleaner, ok := enq.(enqueue.TaskCleaner); ok {
		h.taskCleaner = cleaner
	}
}

func (h *AgentHandler) SetMemoryProvisioner(banks *hindsight.BankProvisioner) {
	h.memoryBanks = banks
}

type agentProviderChoice struct {
	cred  *model.Credential
	model string
}

func pickAgentCredential(db *gorm.DB) (*agentProviderChoice, error) {
	return pickSystemCredentialByModel(db, agentruntime.DefaultAgentModel)
}

func pickSystemCredentialByModel(db *gorm.DB, modelID string) (*agentProviderChoice, error) {
	cred, err := pickActiveSystemCredentialForModel(context.Background(), db, registry.Global(), modelID)
	if err != nil {
		return nil, err
	}
	return &agentProviderChoice{cred: cred, model: modelID}, nil
}
