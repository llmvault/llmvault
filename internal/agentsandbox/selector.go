package agentsandbox

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

type Selector struct {
	DB                *gorm.DB
	AgentRuntimeImage string
}

func (s Selector) MainRuntime(ctx context.Context, orgID, agentID uuid.UUID) (*model.Sandbox, error) {
	var sandbox model.Sandbox
	if err := s.baseQuery(ctx, orgID, agentID).
		Order("created_at DESC").
		First(&sandbox).Error; err != nil {
		return nil, err
	}
	return &sandbox, nil
}

func (s Selector) MainRuntimeByID(ctx context.Context, orgID, agentID, sandboxID uuid.UUID) (*model.Sandbox, error) {
	var sandbox model.Sandbox
	if err := s.baseQuery(ctx, orgID, agentID).
		Where("id = ?", sandboxID).
		First(&sandbox).Error; err != nil {
		return nil, err
	}
	return &sandbox, nil
}

func (s Selector) MainRuntimeMap(ctx context.Context, orgID uuid.UUID, agentIDs []uuid.UUID) (map[uuid.UUID]model.Sandbox, error) {
	out := make(map[uuid.UUID]model.Sandbox, len(agentIDs))
	if len(agentIDs) == 0 {
		return out, nil
	}
	var sandboxes []model.Sandbox
	if err := s.baseQuery(ctx, orgID, uuid.Nil).
		Where("agent_id IN ?", agentIDs).
		Order("agent_id ASC, created_at DESC").
		Find(&sandboxes).Error; err != nil {
		return out, err
	}
	for _, sandbox := range sandboxes {
		if sandbox.AgentID == nil {
			continue
		}
		if _, exists := out[*sandbox.AgentID]; exists {
			continue
		}
		out[*sandbox.AgentID] = sandbox
	}
	return out, nil
}

// activeSandboxStatuses are the statuses the selector treats as an existing main
// runtime. 'creating'/'starting'/'stopped' rows must be returned so concurrent
// syncs reuse an in-flight or idle sandbox instead of minting a duplicate.
var activeSandboxStatuses = []string{"running", "creating", "starting", "stopped"}

func (s Selector) baseQuery(ctx context.Context, orgID, agentID uuid.UUID) *gorm.DB {
	q := s.DB.WithContext(ctx).
		Where("org_id = ? AND status IN ?", orgID, activeSandboxStatuses)
	if agentID != uuid.Nil {
		q = q.Where("agent_id = ?", agentID)
	}
	return q
}
