package handler

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/usehivy/hivy/internal/employeeruntime"
	"github.com/usehivy/hivy/internal/employeesandbox"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/tasks"
	"gorm.io/gorm"
)

func (h *EmployeeHandler) ensureEmployeeSandbox(ctx context.Context, agent *model.Employee) (*model.Sandbox, error) {
	if agent == nil || agent.OrgID == nil {
		return nil, fmt.Errorf("agent must have org_id")
	}
	// Concurrent syncs (org PATCH, env-var writes, webhooks) all reach here during
	// onboarding; without serialisation each runs check-then-create and provisions
	// its own billing sandbox. A per-employee advisory lock lets only one provision;
	// the others block, then reuse the winner's row.
	var sb *model.Sandbox
	txErr := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", employeeSandboxLockKey(agent.ID)).Error; err != nil {
			return fmt.Errorf("acquire employee sandbox lock: %w", err)
		}
		ensured, err := h.ensureEmployeeSandboxLocked(ctx, agent)
		if err != nil {
			return err
		}
		sb = ensured
		return nil
	})
	if txErr != nil {
		return nil, txErr
	}
	return sb, nil
}

func (h *EmployeeHandler) ensureEmployeeSandboxLocked(ctx context.Context, agent *model.Employee) (*model.Sandbox, error) {
	sb, err := h.mainEmployeeRuntimeSelector().MainRuntime(ctx, *agent.OrgID, agent.ID)
	if err == nil {
		// The selector also returns creating/starting/stopped rows; route them
		// through EnsureSandboxActive so an in-flight/idle sandbox is reused (and
		// woken), not abandoned alongside a new one.
		active, ensureErr := h.orchestrator.EnsureSandboxActive(ctx, sb)
		if ensureErr != nil {
			return nil, fmt.Errorf("ensure existing employee sandbox active: %w", ensureErr)
		}
		sb = active
		if h.orchestrator.NeedsURLRefresh(sb) {
			if err := h.orchestrator.RefreshEmployeeSandboxURL(ctx, sb); err != nil {
				return nil, err
			}
		}
		return sb, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("load employee sandbox: %w", err)
	}
	secrets, prepErr := employeeruntime.PrepareStartup(ctx, h.compileDeps, agent)
	if prepErr != nil {
		return nil, prepErr
	}
	created, err := h.orchestrator.CreateEmployeeSandbox(ctx, agent, secrets)
	if err != nil {
		return nil, err
	}
	if err := employeeruntime.AttachProxyTokenToSandbox(ctx, h.compileDeps, agent, created.ID, secrets.ProxyTokenJTI); err != nil {
		return nil, fmt.Errorf("tag employee proxy token sandbox: %w", err)
	}
	return created, nil
}

// employeeSandboxLockKey hashes an employee UUID to a bigint for pg_advisory_xact_lock.
// Collisions only serialise unrelated employees, which is harmless.
func employeeSandboxLockKey(employeeID uuid.UUID) int64 {
	return int64(binary.BigEndian.Uint64(employeeID[:8])) // #nosec G115 -- hash truncation; sign bit is part of the hash distribution
}

func (h *EmployeeHandler) SyncOrgHivyEmployee(ctx context.Context, orgID uuid.UUID) error {
	if h == nil || h.db == nil || h.orchestrator == nil || h.compileDeps.EncKey == nil {
		return fmt.Errorf("employee sandbox sync not configured")
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
			logging.CaptureWithFields(ctx, fmt.Errorf("sync org Hivy employee: ensure memory bank: %w", err), map[string]any{
				"org_id":      orgID.String(),
				"employee_id": agent.ID.String(),
			})
		}
	}
	if _, _, err := h.SyncEmployee(ctx, agent); err != nil {
		return err
	}
	return nil
}

func (h *EmployeeHandler) SyncEmployee(ctx context.Context, agent *model.Employee) (*model.Sandbox, *employeeruntime.SyncResponse, error) {
	if h == nil || h.db == nil || h.orchestrator == nil || h.compileDeps.EncKey == nil {
		return nil, nil, fmt.Errorf("employee sandbox sync not configured")
	}
	sb, err := h.ensureEmployeeSandbox(ctx, agent)
	if err != nil {
		return nil, nil, fmt.Errorf("ensure employee sandbox: %w", err)
	}
	resp, err := h.runEmployeeSync(ctx, agent, sb)
	if err != nil {
		return nil, nil, fmt.Errorf("sync employee sandbox: %w", err)
	}
	return sb, resp, nil
}

func (h *EmployeeHandler) runEmployeeSync(ctx context.Context, agent *model.Employee, sb *model.Sandbox) (*employeeruntime.SyncResponse, error) {
	if agent != nil && agent.OrgID != nil {
		if err := attachEmployeeRequiredSkillsForAgent(ctx, h.db, *agent.OrgID, agent); err != nil {
			return nil, fmt.Errorf("reconcile employee skills: %w", err)
		}
	}
	apiKey, err := h.compileDeps.EncKey.DecryptString(sb.EncryptedRuntimeSecret)
	if err != nil {
		return nil, fmt.Errorf("decrypt runtime secret: %w", err)
	}
	client := employeeruntime.NewClient(sb.RuntimeURL, apiKey)
	if err := client.Healthz(ctx); err != nil {
		return nil, fmt.Errorf("employee runtime healthz: %w", err)
	}
	proxyToken, err := employeeruntime.MintProxyToken(ctx, h.compileDeps, agent, sb.ID)
	if err != nil {
		return nil, fmt.Errorf("mint proxy token: %w", err)
	}
	runtimeEnv, err := employeeruntime.BuildRuntimeEnvWithProxyToken(ctx, h.compileDeps, agent, sb, apiKey, proxyToken)
	if err != nil {
		return nil, fmt.Errorf("load runtime env: %w", err)
	}
	def, err := employeeruntime.CompileWithProxyToken(ctx, h.compileDeps, agent, proxyToken)
	if err != nil {
		return nil, fmt.Errorf("compile: %w", err)
	}
	def.OutboundChannels = employeeruntime.ControlPlaneOutboundChannels(h.compileDeps.Cfg, sb.ID)
	schedules, err := employeeruntime.BuildRuntimeSchedules(ctx, h.db, agent, sb)
	if err != nil {
		return nil, fmt.Errorf("load runtime schedules: %w", err)
	}

	resp, err := client.PutRuntimeConfig(ctx, employeeruntime.ConfigUpdateRequest{
		Definition: def,
		RuntimeEnv: runtimeEnv,
		Schedules:  schedules,
	})
	if err != nil {
		return nil, err
	}

	if err := client.Readyz(ctx); err != nil {
		return nil, fmt.Errorf("employee runtime readyz: %w", err)
	}
	h.scheduleEmployeeProxyTokenRefresh(ctx, agent, sb)
	if agent.Status != "active" {
		if agent.OrgID == nil {
			return nil, fmt.Errorf("mark employee active: missing org_id")
		}
		if err := h.db.WithContext(ctx).Model(&model.Employee{}).
			Where("id = ? AND org_id = ?", agent.ID, *agent.OrgID).
			Update("status", "active").Error; err != nil {
			return nil, fmt.Errorf("mark employee active: %w", err)
		}
		agent.Status = "active"
	}

	return resp, nil
}

func (h *EmployeeHandler) scheduleExistingEmployeeProxyTokenRefresh(ctx context.Context, agent *model.Employee) {
	if h == nil || h.db == nil || agent == nil || agent.OrgID == nil {
		return
	}
	sb, err := h.mainEmployeeRuntimeSelector().MainRuntime(ctx, *agent.OrgID, agent.ID)
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			logging.Capture(ctx, fmt.Errorf("load employee sandbox for proxy token refresh schedule: %w", err))
		}
		return
	}
	h.scheduleEmployeeProxyTokenRefresh(ctx, agent, sb)
}

func (h *EmployeeHandler) scheduleEmployeeProxyTokenRefresh(ctx context.Context, agent *model.Employee, sb *model.Sandbox) {
	if err := tasks.ScheduleEmployeeProxyTokenRefresh(ctx, h.db, h.enqueuer, agent, sb); err != nil {
		logging.Capture(ctx, fmt.Errorf("schedule employee proxy token refresh: %w", err))
	}
}

func (h *EmployeeHandler) loadRuntimeEnv(ctx context.Context, agent *model.Employee, sb *model.Sandbox, runtimeSecret string) (map[string]string, error) {
	return employeeruntime.BuildRuntimeEnv(ctx, h.compileDeps, agent, sb, runtimeSecret)
}

func (h *EmployeeHandler) mainEmployeeRuntimeSelector() employeesandbox.Selector {
	selector := employeesandbox.Selector{DB: h.db}
	if h != nil && h.compileDeps.Cfg != nil {
		selector.EmployeeRuntimeImage = h.compileDeps.Cfg.SandboxesRuntimeBaseImage
		selector.SpecialistRuntimeImage = h.compileDeps.Cfg.SandboxesRuntimeSpecialistImage
	}
	return selector
}

func toSyncResponseDTO(resp *employeeruntime.SyncResponse) syncEmployeeResponse {
	out := syncEmployeeResponse{}
	if resp == nil {
		return out
	}
	out.Applied = resp.Applied
	out.Deleted = resp.Deleted
	out.ReposCloned = resp.ReposCloned
	out.RestartTriggered = resp.RestartTriggered
	out.Errors = resp.Errors
	return out
}
