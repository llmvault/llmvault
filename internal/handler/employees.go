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
	employeeHarness         = "employee-sandbox"
	hivyEmployeeName        = "Hivy"
	hivyEmployeeDescription = "Hivy is the organization's managed AI employee."
	hivyEmployeeAvatarURL   = "/assets/hivy-avatar.png"
)

var defaultEmployeeSkills = []string{"drive"}

type EmployeeHandler struct {
	db           *gorm.DB
	orchestrator *sandbox.Orchestrator
	compileDeps  agentruntime.CompileDeps
	registry     *registry.Registry
	enqueuer     enqueue.TaskEnqueuer
	taskCleaner  enqueue.TaskCleaner
	memoryBanks  *hindsight.BankProvisioner
}

func NewEmployeeHandler(db *gorm.DB, orchestrator *sandbox.Orchestrator, compileDeps agentruntime.CompileDeps, reg *registry.Registry) *EmployeeHandler {
	return &EmployeeHandler{
		db:           db,
		orchestrator: orchestrator,
		compileDeps:  compileDeps,
		registry:     reg,
	}
}

func (h *EmployeeHandler) SetEnqueuer(enq enqueue.TaskEnqueuer) {
	h.enqueuer = enq
	if cleaner, ok := enq.(enqueue.TaskCleaner); ok {
		h.taskCleaner = cleaner
	}
}

func (h *EmployeeHandler) SetMemoryProvisioner(banks *hindsight.BankProvisioner) {
	h.memoryBanks = banks
}

type employeeProviderChoice struct {
	cred  *model.Credential
	model string
}

func pickEmployeeCredential(db *gorm.DB) (*employeeProviderChoice, error) {
	return pickSystemCredentialByModel(db, agentruntime.DefaultEmployeeModel)
}

func pickSystemCredentialByModel(db *gorm.DB, modelID string) (*employeeProviderChoice, error) {
	cred, err := pickActiveSystemCredentialForModel(context.Background(), db, registry.Global(), modelID)
	if err != nil {
		return nil, err
	}
	return &employeeProviderChoice{cred: cred, model: modelID}, nil
}
