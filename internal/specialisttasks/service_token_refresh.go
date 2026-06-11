package specialisttasks

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/employeeruntime"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/specialists"
)

const (
	// specialistProxyTokenRefreshLead refreshes a specialist proxy token once it
	// is within this window of expiry. Specialist tokens are minted with the same
	// 24h TTL as employee tokens but were never refreshed (P1-18); a long-running
	// specialist task would silently lose LLM and MCP access after ~24h.
	specialistProxyTokenRefreshLead = 4 * time.Hour
	// specialistProxyTokenRevokeGrace mirrors the employee refresh grace window:
	// only tokens older than this are revoked so a token minted by a concurrent
	// path is not pulled out from under the runtime.
	specialistProxyTokenRevokeGrace = 10 * time.Minute
)

// refreshSpecialistProxyTokenIfNeeded mints a fresh specialist proxy token and
// pushes an updated runtime config when the sandbox's current specialist token
// is near expiry. It is best-effort: a refresh failure is logged but does not
// block message delivery, since the existing token may still be valid.
func (s *Service) refreshSpecialistProxyTokenIfNeeded(ctx context.Context, employee *model.Employee, task *model.SpecialistTask, sb *model.Sandbox) {
	if s == nil || employee == nil || employee.OrgID == nil || sb == nil || task == nil {
		return
	}
	def, ok := s.catalog.BySlug(task.SpecialistSlug)
	if !ok {
		return
	}

	latest, err := s.latestSpecialistToken(ctx, *employee.OrgID, employee.ID, sb.ID, task.SpecialistSlug)
	if err != nil {
		logging.Capture(ctx, fmt.Errorf("specialist token refresh: load latest token: %w", err))
		return
	}
	// No live token (already expired/revoked) or comfortably far from expiry:
	// nothing to do. If the token is already gone the next message would fail
	// anyway; refreshing here would not help because the runtime is unreachable
	// without a working token only when it is still valid.
	if latest == nil {
		return
	}
	if time.Until(latest.ExpiresAt) > specialistProxyTokenRefreshLead {
		return
	}

	proxyToken, err := employeeruntime.MintSpecialistProxyToken(ctx, s.compileDeps, employee, sb.ID, task.SpecialistSlug)
	if err != nil {
		logging.Capture(ctx, fmt.Errorf("specialist token refresh: mint token: %w", err))
		return
	}

	if err := s.pushSpecialistConfig(ctx, employee, *def, sb, proxyToken); err != nil {
		s.revokeSpecialistToken(ctx, proxyToken.JTI)
		logging.Capture(ctx, fmt.Errorf("specialist token refresh: push config: %w", err))
		return
	}

	if err := s.revokeOlderSpecialistTokens(ctx, *employee.OrgID, employee.ID, sb.ID, task.SpecialistSlug, proxyToken.JTI); err != nil {
		logging.Capture(ctx, fmt.Errorf("specialist token refresh: revoke older tokens: %w", err))
	}
	logging.FromContext(ctx).InfoContext(ctx, "specialist proxy token refreshed",
		"employee_id", employee.ID, "sandbox_id", sb.ID, "specialist_slug", task.SpecialistSlug, "jti", proxyToken.JTI)
}

func (s *Service) pushSpecialistConfig(ctx context.Context, employee *model.Employee, def specialists.Definition, sb *model.Sandbox, proxyToken *employeeruntime.ProxyTokenResult) error {
	runtimeDef, err := employeeruntime.CompileSpecialistWithProxyToken(ctx, s.compileDeps, employee, def, proxyToken)
	if err != nil {
		return fmt.Errorf("compile specialist config: %w", err)
	}
	runtimeDef.OutboundChannels = employeeruntime.ControlPlaneOutboundChannels(s.compileDeps.Cfg, sb.ID)

	runtimeSecret, err := s.compileDeps.EncKey.DecryptString(sb.EncryptedRuntimeSecret)
	if err != nil {
		return fmt.Errorf("decrypt runtime secret: %w", err)
	}
	runtimeEnv, err := employeeruntime.BuildRuntimeEnvWithProxyToken(ctx, s.compileDeps, employee, sb, runtimeSecret, proxyToken)
	if err != nil {
		return fmt.Errorf("build runtime env: %w", err)
	}
	runtimeEnv[employeeruntime.EmployeeEnvRuntimeMode] = "specialist"

	client, err := s.orchestrator.GetRuntimeClient(ctx, sb)
	if err != nil {
		return fmt.Errorf("runtime client: %w", err)
	}
	if _, err := client.PutRuntimeConfig(ctx, employeeruntime.ConfigUpdateRequest{
		Definition: runtimeDef,
		RuntimeEnv: runtimeEnv,
	}); err != nil {
		return fmt.Errorf("push runtime config: %w", err)
	}
	return nil
}

func (s *Service) latestSpecialistToken(ctx context.Context, orgID, employeeID, sandboxID uuid.UUID, slug string) (*model.Token, error) {
	var tok model.Token
	err := s.db.WithContext(ctx).
		Where("org_id = ? AND meta->>? = ? AND meta->>? = ? AND meta->>? = ? AND meta->>? = ? AND meta->>? = ? AND meta->>? = ? AND revoked_at IS NULL",
			orgID,
			model.TokenMetaEmployeeID, employeeID.String(),
			model.TokenMetaType, model.TokenTypeEmployeeProxy,
			model.TokenMetaHarness, model.TokenHarnessEmployeeSandbox,
			model.TokenMetaRuntimeMode, model.TokenRuntimeModeSpecialist,
			model.TokenMetaSpecialistSlug, slug,
			model.TokenMetaSandboxID, sandboxID.String()).
		Order("created_at DESC").
		First(&tok).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &tok, nil
}

func (s *Service) revokeOlderSpecialistTokens(ctx context.Context, orgID, employeeID, sandboxID uuid.UUID, slug, keepJTI string) error {
	if keepJTI == "" {
		return nil
	}
	now := s.now().UTC()
	revokeBefore := now.Add(-specialistProxyTokenRevokeGrace)
	return s.db.WithContext(ctx).Model(&model.Token{}).
		Where("org_id = ? AND meta->>? = ? AND meta->>? = ? AND meta->>? = ? AND meta->>? = ? AND meta->>? = ? AND meta->>? = ? AND jti != ? AND revoked_at IS NULL AND created_at < ?",
			orgID,
			model.TokenMetaEmployeeID, employeeID.String(),
			model.TokenMetaType, model.TokenTypeEmployeeProxy,
			model.TokenMetaHarness, model.TokenHarnessEmployeeSandbox,
			model.TokenMetaRuntimeMode, model.TokenRuntimeModeSpecialist,
			model.TokenMetaSpecialistSlug, slug,
			model.TokenMetaSandboxID, sandboxID.String(),
			keepJTI, revokeBefore).
		Update("revoked_at", now).Error
}

func (s *Service) revokeSpecialistToken(ctx context.Context, jti string) {
	if jti == "" {
		return
	}
	if err := s.db.WithContext(ctx).Model(&model.Token{}).
		Where("jti = ? AND revoked_at IS NULL", jti).
		Update("revoked_at", s.now().UTC()).Error; err != nil {
		logging.Capture(ctx, fmt.Errorf("specialist token refresh: revoke failed token: %w", err))
	}
}
