// Package apps is the orchestration layer for agent-built apps (apps plan
// §3/§4): create, publish (drive keys → immutable org object layout →
// app_versions row), deploy into an app sandbox running hivy-appd, plus the
// thin appd passthroughs (status/logs/rollback/env sync). REST handlers and
// MCP tools are both shells over this service, exactly like internal/sheets.
package apps

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/sandbox"
	"github.com/usehivy/hivy/internal/storage"
)

// ObjectStore is what the publish pipeline needs from S3: read the agent's
// drive objects, stream-copy into the immutable org layout, and presign the
// bundle for the sandbox pull. *storage.S3Presigner satisfies it.
type ObjectStore interface {
	storage.Streamer
	storage.Reader
	PresignGet(ctx context.Context, key string) (string, error)
}

// ObjectKeyAuthorizer is the sheets admission rule for object keys
// (Service.AuthorizeObjectKeys): org-owned pub/o/{orgID}/ keys pass, agent
// drive keys pub/e/{agentID}/ pass only when the agent belongs to the org.
type ObjectKeyAuthorizer interface {
	AuthorizeObjectKeys(ctx context.Context, orgID uuid.UUID, keys []string) error
}

// Sentinel errors, mapped onto HTTP statuses by the REST layer.
var (
	ErrNotFound = errors.New("apps: not found")
	// ErrSlugTaken means an active app in the org already uses the slug
	// derived from the requested name (unique index idx_apps_org_slug_active).
	ErrSlugTaken = errors.New("apps: an app with this name already exists")
	// ErrNotDeployed means the app has no running sandbox to serve traffic.
	ErrNotDeployed = errors.New("apps: app is not deployed")
)

// ValidationError is a caller mistake (bad input, sheet not in channel).
type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return "apps: " + e.Message }

func validationErrorf(format string, args ...any) error {
	return &ValidationError{Message: fmt.Sprintf(format, args...)}
}

// Service orchestrates the app lifecycle. The sandbox dependency is the
// provider interface so tests can stub it; DB rows for app sandboxes live in
// the shared sandboxes table (agent_id NULL) so the apps.sandbox_id FK and
// resource accounting reuse the existing machinery.
type Service struct {
	db       *gorm.DB
	cfg      *config.Config
	encKey   *crypto.SymmetricKey
	provider sandbox.Provider
	store    ObjectStore
	keys     ObjectKeyAuthorizer

	// authPublicKeyPEM is the PKIX PEM of the platform auth public key with
	// newlines escaped as literal `\n` (env files reject raw newlines; the
	// template's hivycore un-escapes them).
	authPublicKeyPEM string

	appd *appdClient
}

// NewService wires the apps orchestration service. authPublicKey must be the
// RSA public key matching the auth signing key (serve.go derives it).
func NewService(db *gorm.DB, cfg *config.Config, encKey *crypto.SymmetricKey, provider sandbox.Provider, store ObjectStore, keys ObjectKeyAuthorizer, authPublicKeyPEM string) *Service {
	return &Service{
		db:               db,
		cfg:              cfg,
		encKey:           encKey,
		provider:         provider,
		store:            store,
		keys:             keys,
		authPublicKeyPEM: authPublicKeyPEM,
		appd:             newAppdClient(),
	}
}

// WithHTTPClient overrides the appd client transport (tests).
func (s *Service) WithHTTPClient(client *http.Client) *Service {
	s.appd.http = client
	return s
}

// WithBootRetryWindow bounds how long Deploy retries connection-refused
// while a freshly created sandbox boots (default 60s; tests shrink it).
func (s *Service) WithBootRetryWindow(window time.Duration) *Service {
	s.appd.bootRetryWindow = window
	return s
}

// GetApp loads one active app org-scoped.
func (s *Service) GetApp(ctx context.Context, orgID, appID uuid.UUID) (*model.App, error) {
	var app model.App
	err := s.db.WithContext(ctx).
		Where("id = ? AND org_id = ? AND archived_at IS NULL", appID, orgID).
		First(&app).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load app: %w", err)
	}
	return &app, nil
}

// ListApps returns the channel's active apps, newest activity first.
func (s *Service) ListApps(ctx context.Context, orgID, channelID uuid.UUID) ([]model.App, error) {
	var apps []model.App
	err := s.db.WithContext(ctx).
		Where("org_id = ? AND channel_id = ? AND archived_at IS NULL", orgID, channelID).
		Order("updated_at DESC").
		Find(&apps).Error
	if err != nil {
		return nil, fmt.Errorf("list apps: %w", err)
	}
	return apps, nil
}

// ListVersions returns the app's active version history, newest first.
func (s *Service) ListVersions(ctx context.Context, orgID, appID uuid.UUID, limit int) ([]model.AppVersion, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var versions []model.AppVersion
	err := s.db.WithContext(ctx).
		Where("app_id = ? AND org_id = ? AND archived_at IS NULL", appID, orgID).
		Order("created_at DESC").
		Limit(limit).
		Find(&versions).Error
	if err != nil {
		return nil, fmt.Errorf("list app versions: %w", err)
	}
	return versions, nil
}

// ArchiveApp soft-archives the app and best-effort stops its sandbox: a
// provider failure is logged into the returned warning, never blocks the
// archive.
func (s *Service) ArchiveApp(ctx context.Context, orgID, appID uuid.UUID) error {
	app, err := s.GetApp(ctx, orgID, appID)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	result := s.db.WithContext(ctx).Model(&model.App{}).
		Where("id = ? AND org_id = ? AND archived_at IS NULL", app.ID, orgID).
		Updates(map[string]any{"archived_at": now, "status": model.AppStatusStopped})
	if result.Error != nil {
		return fmt.Errorf("archive app: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	s.stopSandboxBestEffort(ctx, app)
	s.releaseAliasBestEffort(ctx, app)
	return nil
}

func (s *Service) stopSandboxBestEffort(ctx context.Context, app *model.App) {
	sb, err := s.appSandbox(ctx, app)
	if err != nil || sb == nil || sb.ExternalID == "" {
		return
	}
	if err := s.provider.StopSandbox(ctx, sb.ExternalID); err != nil {
		// Best effort by contract: the archive already committed.
		_ = err
	} else {
		_ = s.db.WithContext(ctx).Model(sb).Update("status", "stopped").Error
	}
}

// appSandbox loads the app's sandbox row; nil when the app was never deployed.
func (s *Service) appSandbox(ctx context.Context, app *model.App) (*model.Sandbox, error) {
	if app.SandboxID == nil {
		return nil, nil
	}
	var sb model.Sandbox
	err := s.db.WithContext(ctx).Where("id = ?", *app.SandboxID).First(&sb).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load app sandbox: %w", err)
	}
	return &sb, nil
}

// decryptAppSecret returns the app's plaintext runtime secret.
func (s *Service) decryptAppSecret(app *model.App) (string, error) {
	if s.encKey == nil {
		return "", fmt.Errorf("apps: sandbox encryption key is not configured")
	}
	secret, err := s.encKey.DecryptString(app.EncryptedAppSecret)
	if err != nil {
		return "", fmt.Errorf("decrypt app secret: %w", err)
	}
	return secret, nil
}

// controlRewrite maps a provider endpoint to a URL this server can reach
// (docker compose → host.docker.internal).
func (s *Service) controlRewrite(raw string) string {
	providerID := ""
	if s.provider != nil {
		providerID = s.provider.ID()
	}
	return sandbox.RuntimeControlURL(s.cfg, providerID, raw)
}

// endpointURL resolves the sandbox endpoint for a container port, rewritten
// for reachability from this server.
func (s *Service) endpointURL(ctx context.Context, sb *model.Sandbox, port int) (string, error) {
	if sb == nil || strings.TrimSpace(sb.ExternalID) == "" {
		return "", ErrNotDeployed
	}
	raw, err := s.provider.GetEndpoint(ctx, sb.ExternalID, port)
	if err != nil {
		return "", fmt.Errorf("get sandbox endpoint (port %d): %w", port, err)
	}
	return s.controlRewrite(raw), nil
}

// SandboxEndpointURL resolves a provider endpoint for a container port on an
// arbitrary sandbox row (the preview-env handler uses it for the BUILDER
// agent's sandbox), rewritten for reachability like every other endpoint.
func (s *Service) SandboxEndpointURL(ctx context.Context, sb *model.Sandbox, port int) (string, error) {
	return s.endpointURL(ctx, sb, port)
}

// AppURL is the app's public 8080 endpoint (the SPA / auth callback origin).
// The stable alias URL wins when set (microsandbox production); otherwise it
// falls back to the plain sandbox endpoint (docker/local, and previews).
// ErrNotDeployed when the app has no running deployment.
func (s *Service) AppURL(ctx context.Context, app *model.App) (string, error) {
	if app.Status != model.AppStatusRunning {
		return "", ErrNotDeployed
	}
	if url := strings.TrimSpace(app.AliasURL); url != "" {
		return url, nil
	}
	sb, err := s.appSandbox(ctx, app)
	if err != nil {
		return "", err
	}
	return s.endpointURL(ctx, sb, appPort)
}

// appdEndpoint resolves the appd control endpoint (7080) plus the bearer.
func (s *Service) appdEndpoint(ctx context.Context, app *model.App) (baseURL, secret string, err error) {
	sb, err := s.appSandbox(ctx, app)
	if err != nil {
		return "", "", err
	}
	baseURL, err = s.endpointURL(ctx, sb, appdPort)
	if err != nil {
		return "", "", err
	}
	secret, err = s.decryptAppSecret(app)
	if err != nil {
		return "", "", err
	}
	return baseURL, secret, nil
}
