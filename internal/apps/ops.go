package apps

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

// AppStatus is the composite status view: the app row, its active version,
// and (when deployed) appd's live health report.
type AppStatus struct {
	App           *model.App
	ActiveVersion *model.AppVersion
	// Health is appd's GET /health payload verbatim; nil when the app has no
	// sandbox or the probe failed (HealthError carries the reason).
	Health      json.RawMessage
	HealthError string
}

// Status loads the app and asks its appd for a live health report. A dead or
// missing sandbox degrades to HealthError rather than failing the call.
func (s *Service) Status(ctx context.Context, orgID, appID uuid.UUID) (*AppStatus, error) {
	app, err := s.GetApp(ctx, orgID, appID)
	if err != nil {
		return nil, err
	}
	status := &AppStatus{App: app}
	if app.ActiveVersionID != nil {
		var version model.AppVersion
		err := s.db.WithContext(ctx).
			Where("id = ? AND app_id = ?", *app.ActiveVersionID, app.ID).
			First(&version).Error
		if err == nil {
			status.ActiveVersion = &version
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("load active version: %w", err)
		}
	}

	baseURL, secret, err := s.appdEndpoint(ctx, app)
	if err != nil {
		status.HealthError = err.Error()
		return status, nil
	}
	health, err := s.appd.health(ctx, baseURL, secret)
	if err != nil {
		status.HealthError = err.Error()
		return status, nil
	}
	status.Health = health
	return status, nil
}

// Logs passes the query through to appd GET /logs and returns the payload
// verbatim (lines, count, app process state, journal).
func (s *Service) Logs(ctx context.Context, orgID, appID uuid.UUID, query LogsQuery) (json.RawMessage, error) {
	app, err := s.GetApp(ctx, orgID, appID)
	if err != nil {
		return nil, err
	}
	baseURL, secret, err := s.appdEndpoint(ctx, app)
	if err != nil {
		return nil, err
	}
	return s.appd.logs(ctx, baseURL, secret, query)
}

// Rollback activates a previously published version: appd repoints the
// current symlink when the release is still on disk; a sandbox that lost the
// release (404) gets a full re-deploy of that version instead.
func (s *Service) Rollback(ctx context.Context, orgID, appID, versionID uuid.UUID) (*model.AppVersion, error) {
	app, err := s.GetApp(ctx, orgID, appID)
	if err != nil {
		return nil, err
	}
	var version model.AppVersion
	err = s.db.WithContext(ctx).
		Where("id = ? AND app_id = ? AND org_id = ? AND archived_at IS NULL", versionID, app.ID, orgID).
		First(&version).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load app version: %w", err)
	}

	baseURL, secret, endpointErr := s.appdEndpoint(ctx, app)
	if endpointErr != nil {
		return nil, endpointErr
	}
	if _, err := s.appd.rollback(ctx, baseURL, secret, version.BundleSHA256); err != nil {
		var appdErr *AppdError
		if errors.As(err, &appdErr) && appdErr.StatusCode == http.StatusNotFound {
			// Release pruned from disk — re-deploy the version from S3.
			if deployErr := s.Deploy(ctx, app, &version); deployErr != nil {
				return nil, deployErr
			}
			return &version, nil
		}
		return nil, err
	}

	if err := s.db.WithContext(ctx).Model(&model.App{}).
		Where("id = ? AND org_id = ?", app.ID, orgID).
		Updates(map[string]any{
			"status":            model.AppStatusRunning,
			"active_version_id": version.ID,
		}).Error; err != nil {
		return nil, fmt.Errorf("record rollback: %w", err)
	}
	return &version, nil
}

// EnvSync re-derives the app's full environment (platform HIVY_ keys plus
// the channel's current env vars) and pushes it via appd POST /env, which
// rewrites the env file and restarts the app.
func (s *Service) EnvSync(ctx context.Context, orgID, appID uuid.UUID) error {
	app, err := s.GetApp(ctx, orgID, appID)
	if err != nil {
		return err
	}
	secret, err := s.decryptAppSecret(app)
	if err != nil {
		return err
	}
	env, err := s.buildAppEnv(ctx, app, secret)
	if err != nil {
		return err
	}
	baseURL, _, err := s.appdEndpoint(ctx, app)
	if err != nil {
		return err
	}
	return s.appd.pushEnv(ctx, baseURL, secret, env)
}
