package employeeruntime

import (
	"context"
	"fmt"

	"github.com/usehivy/hivy/internal/model"
)

func PushEmployeeRuntimeConfigForSandbox(ctx context.Context, deps CompileDeps, sb *model.Sandbox) error {
	if deps.DB == nil {
		return fmt.Errorf("employee runtime config push: db is required")
	}
	if deps.EncKey == nil {
		return fmt.Errorf("employee runtime config push: encryption key is required")
	}
	if sb == nil || sb.EmployeeID == nil || sb.OrgID == nil {
		return fmt.Errorf("employee runtime config push: sandbox must have employee_id and org_id")
	}
	var agent model.Employee
	if err := deps.DB.WithContext(ctx).
		Where("id = ? AND org_id = ? AND status <> ?", *sb.EmployeeID, *sb.OrgID, "archived").
		First(&agent).Error; err != nil {
		return fmt.Errorf("load employee for runtime config push: %w", err)
	}
	return PushEmployeeRuntimeConfig(ctx, deps, &agent, sb)
}

func PushEmployeeRuntimeConfig(ctx context.Context, deps CompileDeps, agent *model.Employee, sb *model.Sandbox) error {
	if deps.EncKey == nil {
		return fmt.Errorf("employee runtime config push: encryption key is required")
	}
	if agent == nil || agent.OrgID == nil {
		return fmt.Errorf("employee runtime config push: agent must have org_id")
	}
	if sb == nil {
		return fmt.Errorf("employee runtime config push: sandbox is required")
	}
	runtimeSecret, err := deps.EncKey.DecryptString(sb.EncryptedRuntimeSecret)
	if err != nil {
		return fmt.Errorf("decrypt runtime secret: %w", err)
	}
	configUpdate, _, err := BuildEmployeeRuntimeConfigUpdate(ctx, deps, agent, sb, runtimeSecret)
	if err != nil {
		return fmt.Errorf("build employee runtime config: %w", err)
	}
	client := NewClient(sb.RuntimeURL, runtimeSecret)
	if err := client.Healthz(ctx); err != nil {
		return fmt.Errorf("employee runtime healthz: %w", err)
	}
	if _, err := client.PutRuntimeConfig(ctx, configUpdate); err != nil {
		return fmt.Errorf("employee runtime put config: %w", err)
	}
	if err := client.Readyz(ctx); err != nil {
		return fmt.Errorf("employee runtime readyz: %w", err)
	}
	return nil
}
