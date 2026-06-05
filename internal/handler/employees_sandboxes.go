package handler

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/employeesandbox"
	"github.com/usehivy/hivy/internal/model"
)

func loadMainEmployeeRuntimeSandboxPerAgent(ctx context.Context, db *gorm.DB, orgID uuid.UUID, agentIDs []uuid.UUID, employeeImage, specialistImage string) map[uuid.UUID]*employeeSandboxSummary {
	out := make(map[uuid.UUID]*employeeSandboxSummary, len(agentIDs))
	if len(agentIDs) == 0 {
		return out
	}
	sandboxes, err := employeesandbox.Selector{
		DB:                     db,
		EmployeeRuntimeImage:   employeeImage,
		SpecialistRuntimeImage: specialistImage,
	}.MainRuntimeMap(ctx, orgID, agentIDs)
	if err != nil {
		return out
	}
	for employeeID, sandbox := range sandboxes {
		out[employeeID] = sandboxSummary(sandbox)
	}
	return out
}

func sandboxSummary(sandbox model.Sandbox) *employeeSandboxSummary {
	createdAt := sandbox.CreatedAt.UTC().Format(time.RFC3339)
	summary := &employeeSandboxSummary{
		ID:           sandbox.ID.String(),
		Status:       sandbox.Status,
		ExternalID:   sandbox.ExternalID,
		ErrorMessage: sandbox.ErrorMessage,
		CreatedAt:    createdAt,
		snapshotID:   sandbox.SnapshotID,
	}
	if sandbox.LastActiveAt != nil {
		lastActiveAt := sandbox.LastActiveAt.UTC().Format(time.RFC3339)
		summary.LastActiveAt = &lastActiveAt
	}
	return summary
}
