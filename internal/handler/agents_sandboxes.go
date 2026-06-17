package handler

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/agentsandbox"
	"github.com/usehivy/hivy/internal/model"
)

func loadMainAgentRuntimeSandboxPerAgent(ctx context.Context, db *gorm.DB, orgID uuid.UUID, agentIDs []uuid.UUID, agentImage string) map[uuid.UUID]*agentSandboxSummary {
	out := make(map[uuid.UUID]*agentSandboxSummary, len(agentIDs))
	if len(agentIDs) == 0 {
		return out
	}
	sandboxes, err := agentsandbox.Selector{
		DB:                db,
		AgentRuntimeImage: agentImage,
	}.MainRuntimeMap(ctx, orgID, agentIDs)
	if err != nil {
		return out
	}
	for agentID, sandbox := range sandboxes {
		out[agentID] = sandboxSummary(sandbox)
	}
	return out
}

func sandboxSummary(sandbox model.Sandbox) *agentSandboxSummary {
	createdAt := sandbox.CreatedAt.UTC().Format(time.RFC3339)
	summary := &agentSandboxSummary{
		ID:             sandbox.ID.String(),
		Status:         sandbox.Status,
		ExternalID:     sandbox.ExternalID,
		RuntimeVersion: agentRuntimeVersionLabelFromPtr(sandbox.SnapshotID),
		ErrorMessage:   sandbox.ErrorMessage,
		CreatedAt:      createdAt,
		snapshotID:     sandbox.SnapshotID,
	}
	if sandbox.LastActiveAt != nil {
		lastActiveAt := sandbox.LastActiveAt.UTC().Format(time.RFC3339)
		summary.LastActiveAt = &lastActiveAt
	}
	return summary
}
