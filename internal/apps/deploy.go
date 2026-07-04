package apps

import (
	"context"
	"fmt"
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

// Deploy activates a published version: ensure the app sandbox exists
// (created with the app image and the full env, secret included, so appd can
// authenticate), presign the bundle, and drive appd POST /deploy — which
// downloads, sha-verifies, extracts, rewrites the env file, and restarts the
// app. The env rides the deploy body for existing sandboxes, so env refresh
// and release activation land in one restart. Status transitions:
// deploying → running on appd success, → failed on any error.
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

	if err := s.setAppStatus(ctx, app, model.AppStatusDeploying); err != nil {
		return err
	}

	deployErr := s.deploySandbox(ctx, app, version, secret, env)
	if deployErr != nil {
		if statusErr := s.setAppStatus(ctx, app, model.AppStatusFailed); statusErr != nil {
			logging.FromContext(ctx).ErrorContext(ctx, "mark app deploy failed", "app_id", app.ID, "error", statusErr)
		}
		return deployErr
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

func (s *Service) deploySandbox(ctx context.Context, app *model.App, version *model.AppVersion, secret string, env map[string]string) error {
	sb, created, err := s.ensureAppSandbox(ctx, app, env)
	if err != nil {
		return err
	}

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

	// Claim (or repoint) the app's stable production alias at this sandbox
	// (silent no-op on providers without alias support; see claimAlias).
	if err := s.claimAlias(ctx, app, sb); err != nil {
		return err
	}
	return nil
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
