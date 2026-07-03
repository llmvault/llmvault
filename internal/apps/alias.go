package apps

import (
	"context"
	"fmt"
	"strings"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/sandbox"
)

// aliasStem returns the alias hostname stem to claim/release for an app: the
// stored alias, falling back to the slug for rows created before the stem was
// persisted.
func aliasStem(app *model.App) string {
	if alias := strings.TrimSpace(app.Alias); alias != "" {
		return alias
	}
	return app.Slug
}

// claimAlias binds the app's alias to the given sandbox on the app port when
// the provider supports aliases, storing the returned public URL in
// apps.alias_url. Providers without AliasProvider (docker/local) skip silently,
// leaving alias_url empty so AppURL falls back to the sandbox endpoint. A claim
// failure fails the deploy: a production app without its stable URL is not
// deployed.
func (s *Service) claimAlias(ctx context.Context, app *model.App, sb *model.Sandbox) error {
	provider, ok := s.provider.(sandbox.AliasProvider)
	if !ok {
		return nil
	}
	alias := aliasStem(app)
	url, err := provider.ClaimAlias(ctx, alias, sb.ExternalID, appPort)
	if err != nil {
		return fmt.Errorf("claim app alias %q: %w", alias, err)
	}
	if err := s.db.WithContext(ctx).Model(&model.App{}).
		Where("id = ? AND org_id = ?", app.ID, app.OrgID).
		Update("alias_url", url).Error; err != nil {
		return fmt.Errorf("store app alias url: %w", err)
	}
	app.AliasURL = url
	return nil
}

// releaseAliasBestEffort unbinds the app's alias when the provider supports it.
// Best effort by contract: the archive already committed, and a stale mapping
// is harmless (the sandbox is stopped and the next claim repoints).
func (s *Service) releaseAliasBestEffort(ctx context.Context, app *model.App) {
	provider, ok := s.provider.(sandbox.AliasProvider)
	if !ok {
		return
	}
	alias := aliasStem(app)
	if alias == "" {
		return
	}
	if err := provider.ReleaseAlias(ctx, alias); err != nil {
		logging.FromContext(ctx).WarnContext(ctx, "release app alias", "app_id", app.ID, "alias", alias, "error", err)
	}
}
