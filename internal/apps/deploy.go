package apps

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/lib/pq"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/sandbox"
)

// appSandboxSize is the size app sandboxes request: the micro tier (small,
// backend-plus-SPA workloads). Docker uses its own defaults when a dimension is
// 0; microsandbox floors explicit resources to nano on the control plane.
const appSandboxSize = "micro"

// Deploy activates a published version: ensure the app sandbox, presign the
// bundle, and drive appd /deploy (download, verify, extract, restart) onto it.
// It fails closed with a hard rollback — on any error deploySandbox has already
// restored the pre-deploy state (a newly created sandbox is torn down; a
// redeploy is rolled back to the previously active version) and set the terminal
// status, so a failed publish leaves no half-applied deploy behind. The returned
// error is classified InfraError vs AppdError so callers can tell our fault from
// the app bundle's.
func (s *Service) Deploy(ctx context.Context, app *model.App, version *model.AppVersion) error {
	if s.provider == nil {
		return fmt.Errorf("apps: sandbox provider is not configured")
	}
	if version == nil || version.AppID != app.ID {
		return validationErrorf("version does not belong to this app")
	}

	secret, err := s.decryptAppSecret(app)
	if err != nil {
		return err
	}
	env, err := s.buildAppEnv(ctx, app, secret)
	if err != nil {
		return err
	}
	if url := s.appActivityURL(app); url != "" {
		env[envAppActivityURL] = url
	}

	if err := s.setAppStatus(ctx, app, model.AppStatusDeploying); err != nil {
		return err
	}

	if deployErr := s.deploySandbox(ctx, app, version, secret, env); deployErr != nil {
		return classifyDeployFault(deployErr)
	}

	if err := s.db.WithContext(ctx).Model(&model.App{}).
		Where("id = ? AND org_id = ?", app.ID, app.OrgID).
		Updates(map[string]any{
			"status":            model.AppStatusRunning,
			"active_version_id": version.ID,
			"template_version":  version.TemplateVersion,
		}).Error; err != nil {
		return fmt.Errorf("record deployed version: %w", err)
	}
	app.Status = model.AppStatusRunning
	app.ActiveVersionID = &version.ID
	app.TemplateVersion = version.TemplateVersion
	return nil
}

// deploySandbox ensures the app sandbox, activates the release, and on any
// failure restores the pre-deploy state and sets the terminal status (see
// rollbackFailedDeploy).
func (s *Service) deploySandbox(ctx context.Context, app *model.App, version *model.AppVersion, secret string, env map[string]string) error {
	sb, created, err := s.ensureAppSandbox(ctx, app, env)
	if err != nil {
		// ensureAppSandbox already deleted any half-created sandbox; nothing is
		// deployed to roll back.
		s.setAppStatusBestEffort(context.WithoutCancel(ctx), app, model.AppStatusFailed)
		return err
	}
	if err := s.activateRelease(ctx, app, version, secret, env, sb, created); err != nil {
		s.rollbackFailedDeploy(ctx, app, secret, env, sb, created)
		return err
	}
	return nil
}

// activateRelease presigns the bundle, drives appd to download/verify/extract
// and restart onto it, then claims (or repoints) the app's stable alias at the
// sandbox. Any step failing returns an error; the caller compensates.
func (s *Service) activateRelease(ctx context.Context, app *model.App, version *model.AppVersion, secret string, env map[string]string, sb *model.Sandbox, created bool) error {
	bundleURL, err := s.store.PresignGet(ctx, version.BundleObjectKey)
	if err != nil {
		return fmt.Errorf("presign bundle: %w", err)
	}
	appdURL, err := s.endpointURL(ctx, sb, appdPort)
	if err != nil {
		return err
	}

	req := AppdDeployRequest{
		BundleURL: bundleURL,
		SHA256:    version.BundleSHA256,
		VersionID: version.ID.String(),
		// Always carry the env: on first boot appd writes the unit env file
		// from it, and on redeploys it doubles as the env push (POST /env
		// semantics) without a second restart.
		Env: env,
	}
	if _, err := s.appd.deployWithBootRetry(ctx, appdURL, secret, req); err != nil {
		if created {
			return fmt.Errorf("appd deploy (new sandbox %s): %w", sb.ID, err)
		}
		return fmt.Errorf("appd deploy (sandbox %s): %w", sb.ID, err)
	}

	// Claim (or repoint) the app's stable alias at this sandbox and push the
	// route to the gateway (silent no-op on providers without alias support).
	// A claim failure fails the deploy, which the caller rolls back.
	if err := s.claimAlias(ctx, app, sb); err != nil {
		return err
	}
	return nil
}

// rollbackFailedDeploy restores the pre-deploy state after activateRelease
// fails and sets the terminal status: a sandbox created for this attempt is torn
// down (app back to no deployment, ready for a clean retry); a redeploy into a
// pre-existing sandbox is rolled back to the previously active version, keeping
// the app serving what it served before. It runs on a cancellation-detached
// context so a client that gave up mid-publish still gets a clean rollback.
func (s *Service) rollbackFailedDeploy(ctx context.Context, app *model.App, secret string, env map[string]string, sb *model.Sandbox, created bool) {
	cctx := context.WithoutCancel(ctx)
	logger := logging.FromContext(ctx)
	if created {
		s.cleanupFailedAppSandbox(cctx, sb, sb.ExternalID)
		if err := s.db.WithContext(cctx).Model(&model.App{}).
			Where("id = ? AND org_id = ?", app.ID, app.OrgID).
			Updates(map[string]any{"sandbox_id": nil, "status": model.AppStatusFailed}).Error; err != nil {
			logger.ErrorContext(cctx, "reset app after failed first deploy", "app_id", app.ID, "error", err)
		}
		app.SandboxID = nil
		app.Status = model.AppStatusFailed
		return
	}
	if s.restorePreviousVersion(cctx, app, secret, env, sb) {
		s.setAppStatusBestEffort(cctx, app, model.AppStatusRunning)
		return
	}
	s.setAppStatusBestEffort(cctx, app, model.AppStatusFailed)
}

// restorePreviousVersion drives appd back to the app's active version after a
// failed redeploy, returning true when the app is again serving a known-good
// release. It tries appd /rollback (a symlink swap to the already-extracted
// release), and re-deploys from storage if that release was evicted (404).
func (s *Service) restorePreviousVersion(ctx context.Context, app *model.App, secret string, env map[string]string, sb *model.Sandbox) bool {
	logger := logging.FromContext(ctx)
	if app.ActiveVersionID == nil {
		return false
	}
	var prev model.AppVersion
	if err := s.db.WithContext(ctx).
		Where("id = ? AND app_id = ?", *app.ActiveVersionID, app.ID).
		First(&prev).Error; err != nil {
		logger.ErrorContext(ctx, "rollback: load previous version", "app_id", app.ID, "error", err)
		return false
	}
	appdURL, err := s.endpointURL(ctx, sb, appdPort)
	if err != nil {
		logger.ErrorContext(ctx, "rollback: resolve appd endpoint", "app_id", app.ID, "error", err)
		return false
	}
	if _, err := s.appd.rollback(ctx, appdURL, secret, prev.BundleSHA256); err != nil {
		var appdErr *AppdError
		if errors.As(err, &appdErr) && appdErr.StatusCode == http.StatusNotFound {
			// Previous release pruned from disk — re-extract it from storage.
			if rerr := s.activateRelease(ctx, app, &prev, secret, env, sb, false); rerr != nil {
				logger.ErrorContext(ctx, "rollback: re-deploy previous version", "app_id", app.ID, "error", rerr)
				return false
			}
			return true
		}
		logger.ErrorContext(ctx, "rollback: appd rollback", "app_id", app.ID, "error", err)
		return false
	}
	return true
}

// ensureAppSandbox returns the app's sandbox, creating one when missing:
// a shared sandboxes row (agent_id NULL, runtime secret = the app secret) and
// a provider sandbox from the app image with the env passed at creation.
func (s *Service) ensureAppSandbox(ctx context.Context, app *model.App, env map[string]string) (*model.Sandbox, bool, error) {
	if existing, err := s.appSandbox(ctx, app); err != nil {
		return nil, false, err
	} else if existing != nil && existing.ExternalID != "" {
		return existing, false, nil
	}

	sb := model.Sandbox{
		OrgID:                  &app.OrgID,
		ProviderID:             s.provider.ID(),
		EncryptedRuntimeSecret: app.EncryptedAppSecret,
		Status:                 "creating",
		ExposedPorts:           pq.Int64Array{appPort},
	}
	imageRef := AppImageRef(s.cfg)
	sb.SnapshotID = &imageRef
	if err := s.db.WithContext(ctx).Create(&sb).Error; err != nil {
		return nil, false, fmt.Errorf("save app sandbox row: %w", err)
	}

	size, _ := model.TemplateSizeSpec(appSandboxSize)
	info, err := s.provider.CreateSandbox(ctx, sandbox.CreateSandboxOpts{
		Name:        appSandboxName(app),
		TemplateRef: imageRef,
		EnvVars:     env,
		CPU:         size.CPU,
		Memory:      size.Memory,
		Disk:        size.Disk,
		Labels: map[string]string{
			"org_id":     app.OrgID.String(),
			"sandbox_id": sb.ID.String(),
			"app_id":     app.ID.String(),
			"harness":    "app-sandbox",
		},
		ExposedPorts: []int{appPort},
		// App sandboxes run hivy-appd on appdPort, which serves /health (not the
		// agent-runtime /healthz). Probe it explicitly so the wake health check
		// after an idle sleep succeeds instead of 404ing on /healthz.
		HealthCheck: &sandbox.SandboxHealthCheck{Port: appdPort, Path: "/health", ExpectedStatus: 200},
	})
	if err != nil {
		if delErr := s.db.WithContext(ctx).Where("id = ?", sb.ID).Delete(&model.Sandbox{}).Error; delErr != nil {
			logging.FromContext(ctx).ErrorContext(ctx, "delete orphaned app sandbox row", "sandbox_id", sb.ID, "error", delErr)
		}
		return nil, false, fmt.Errorf("provider create app sandbox: %w", err)
	}

	runtimeURL, err := s.provider.GetEndpoint(ctx, info.ExternalID, appdPort)
	if err != nil {
		s.cleanupFailedAppSandbox(ctx, &sb, info.ExternalID)
		return nil, false, fmt.Errorf("get appd endpoint: %w", err)
	}

	now := time.Now()
	if err := s.db.WithContext(ctx).Model(&sb).Updates(map[string]any{
		"external_id":    info.ExternalID,
		"runtime_url":    runtimeURL,
		"status":         "running",
		"last_active_at": now,
	}).Error; err != nil {
		s.cleanupFailedAppSandbox(ctx, &sb, info.ExternalID)
		return nil, false, fmt.Errorf("update app sandbox row: %w", err)
	}
	sb.ExternalID = info.ExternalID
	sb.RuntimeURL = runtimeURL
	sb.Status = "running"

	if err := s.db.WithContext(ctx).Model(&model.App{}).
		Where("id = ? AND org_id = ?", app.ID, app.OrgID).
		Update("sandbox_id", sb.ID).Error; err != nil {
		return nil, false, fmt.Errorf("link app sandbox: %w", err)
	}
	app.SandboxID = &sb.ID
	return &sb, true, nil
}

func (s *Service) cleanupFailedAppSandbox(ctx context.Context, sb *model.Sandbox, externalID string) {
	cleanupCtx := context.WithoutCancel(ctx)
	if externalID != "" {
		if err := s.provider.DeleteSandbox(cleanupCtx, externalID); err != nil {
			logging.FromContext(ctx).ErrorContext(ctx, "delete failed app sandbox upstream", "external_id", externalID, "error", err)
		}
	}
	if err := s.db.WithContext(cleanupCtx).Where("id = ?", sb.ID).Delete(&model.Sandbox{}).Error; err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "delete failed app sandbox row", "sandbox_id", sb.ID, "error", err)
	}
}

func (s *Service) setAppStatus(ctx context.Context, app *model.App, status string) error {
	if err := s.db.WithContext(ctx).Model(&model.App{}).
		Where("id = ? AND org_id = ?", app.ID, app.OrgID).
		Update("status", status).Error; err != nil {
		return fmt.Errorf("set app status %q: %w", status, err)
	}
	app.Status = status
	return nil
}

func appSandboxName(app *model.App) string {
	id := app.ID.String()
	if len(id) > 8 {
		id = id[:8]
	}
	return "app-" + app.Slug + "-" + id
}
