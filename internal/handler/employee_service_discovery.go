package handler

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

const (
	serviceDiscoveryRuntimeJobPrefix = "system:service-discovery"
	serviceDiscoveryCreatedBySession = "system:service-discovery"
	serviceDiscoveryIntervalSeconds  = int64(24 * 60 * 60)
)

func serviceDiscoveryProviderSupported(provider string) bool {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "glitchtip", "linear", "notion", "railway", "slack", "vercel":
		return true
	default:
		return false
	}
}

func serviceDiscoveryRuntimeJobID(provider string, connectionID uuid.UUID) string {
	return fmt.Sprintf("%s:%s:%s", serviceDiscoveryRuntimeJobPrefix, strings.ToLower(provider), connectionID.String())
}

func serviceDiscoveryIntervalPtr() *int64 {
	value := serviceDiscoveryIntervalSeconds
	return &value
}

func (h *EmployeeHandler) EnsureServiceDiscoveryScheduleForConnection(ctx context.Context, orgID uuid.UUID, conn model.Connection) error {
	if h == nil || h.db == nil {
		return fmt.Errorf("employee service discovery not configured")
	}
	provider := strings.ToLower(strings.TrimSpace(conn.Integration.Provider))
	if !serviceDiscoveryProviderSupported(provider) {
		return nil
	}
	agent, err := ensureHivyEmployee(ctx, h.db, orgID)
	if err != nil {
		return fmt.Errorf("ensure Hivy employee: %w", err)
	}
	if err := attachEmployeeRequiredSkillsForAgent(ctx, h.db, orgID, agent); err != nil {
		return fmt.Errorf("attach employee required skills: %w", err)
	}
	if h.memoryBanks != nil {
		if err := h.memoryBanks.EnsureOrgBank(ctx, orgID); err != nil {
			logging.CaptureWithFields(ctx, fmt.Errorf("service discovery: ensure memory bank: %w", err), map[string]any{
				"org_id":      orgID.String(),
				"employee_id": agent.ID.String(),
				"provider":    provider,
			})
		}
	}
	sb, err := h.ensureEmployeeSandbox(ctx, agent)
	if err != nil {
		return fmt.Errorf("ensure employee sandbox: %w", err)
	}
	if err := upsertServiceDiscoverySchedule(ctx, h.db, orgID, agent.ID, sb.ID, conn); err != nil {
		return err
	}
	if _, err := h.runEmployeeSync(ctx, agent, sb); err != nil {
		return fmt.Errorf("push employee runtime config: %w", err)
	}
	return nil
}

func (h *EmployeeHandler) DisableServiceDiscoveryScheduleForConnection(ctx context.Context, orgID uuid.UUID, conn model.Connection) error {
	if h == nil || h.db == nil {
		return fmt.Errorf("employee service discovery not configured")
	}
	provider := strings.ToLower(strings.TrimSpace(conn.Integration.Provider))
	if !serviceDiscoveryProviderSupported(provider) {
		return nil
	}
	now := time.Now().UTC()
	if err := h.db.WithContext(ctx).Model(&model.EmployeeSchedule{}).
		Where("org_id = ? AND connection_id = ? AND is_system = ?", orgID, conn.ID, true).
		Updates(map[string]any{
			"status":       "cancelled",
			"cancelled_at": now,
			"next_run_at":  nil,
			"updated_at":   now,
		}).Error; err != nil {
		return fmt.Errorf("cancel service discovery schedule: %w", err)
	}
	agent, err := ensureHivyEmployee(ctx, h.db, orgID)
	if err != nil {
		return fmt.Errorf("ensure Hivy employee: %w", err)
	}
	sb, err := h.mainEmployeeRuntimeSelector().MainRuntime(ctx, orgID, agent.ID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return fmt.Errorf("load employee sandbox: %w", err)
	}
	if _, err := h.runEmployeeSync(ctx, agent, sb); err != nil {
		return fmt.Errorf("push employee runtime config: %w", err)
	}
	return nil
}

func upsertServiceDiscoverySchedule(ctx context.Context, db *gorm.DB, orgID, employeeID, sandboxID uuid.UUID, conn model.Connection) error {
	provider := strings.ToLower(strings.TrimSpace(conn.Integration.Provider))
	prompt := serviceDiscoveryPrompt(provider, conn)
	if prompt == "" {
		return nil
	}
	now := time.Now().UTC()
	nextRunAt := now
	runtimeJobID := serviceDiscoveryRuntimeJobID(provider, conn.ID)
	displayName := strings.TrimSpace(conn.Integration.DisplayName)
	if displayName == "" {
		displayName = provider
	}
	schedule := model.EmployeeSchedule{
		OrgID:            orgID,
		EmployeeID:       employeeID,
		SandboxID:        sandboxID,
		RuntimeJobID:     runtimeJobID,
		IsSystem:         true,
		Provider:         provider,
		ConnectionID:     &conn.ID,
		Metadata:         model.JSON{"purpose": "service_discovery", "connection_id": conn.ID.String()},
		Status:           "active",
		Channel:          "system",
		Description:      fmt.Sprintf("%s service discovery", displayName),
		TaskPrompt:       prompt,
		IntervalSeconds:  serviceDiscoveryIntervalPtr(),
		NextRunAt:        &nextRunAt,
		CreatedBySession: serviceDiscoveryCreatedBySession,
		RuntimeCreatedAt: &now,
	}
	err := db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "employee_id"}, {Name: "runtime_job_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"sandbox_id",
			"is_system",
			"provider",
			"connection_id",
			"metadata",
			"status",
			"channel",
			"description",
			"task_prompt",
			"interval_seconds",
			"cancelled_at",
			"updated_at",
		}),
	}).Create(&schedule).Error
	if err != nil {
		return fmt.Errorf("upsert service discovery schedule: %w", err)
	}
	return nil
}
