package integrations

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/mcp/catalog"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/nango"
)

// SyncResult summarizes one complete Nango-to-Hivy reconciliation.
type SyncResult struct {
	Discovered  int
	Created     int
	Updated     int
	Unchanged   int
	Unavailable int
}

// SyncConfigured treats Nango as the source of truth for configured
// integrations and stores a local projection used by connections and MCP.
func SyncConfigured(ctx context.Context, db *gorm.DB, client *nango.Client, cat *catalog.Catalog) (SyncResult, error) {
	if db == nil || client == nil {
		return SyncResult{}, fmt.Errorf("database and Nango client are required")
	}
	if cat == nil {
		cat = catalog.Global()
	}
	configured, err := client.ListIntegrations(ctx)
	if err != nil {
		return SyncResult{}, err
	}
	result := SyncResult{Discovered: len(configured)}
	seen := make([]string, 0, len(configured))
	type syncEvent struct {
		integration nango.Integration
		state       string
	}
	events := make([]syncEvent, 0, len(configured))
	var missing []model.Integration
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// API and worker processes bootstrap concurrently. Serialize the local
		// projection so both can start without racing on the unique key.
		if err := tx.WithContext(ctx).Exec("SELECT pg_advisory_xact_lock(hashtext('hivy_nango_integrations_sync'))").Error; err != nil {
			return fmt.Errorf("lock Nango integration sync: %w", err)
		}
		for _, remote := range configured {
			seen = append(seen, remote.UniqueKey)
			state, syncErr := syncConfiguredIntegration(ctx, tx, client, cat, remote)
			if syncErr != nil {
				return fmt.Errorf("sync Nango integration %s: %w", remote.UniqueKey, syncErr)
			}
			switch state {
			case "created":
				result.Created++
			case "updated":
				result.Updated++
			default:
				result.Unchanged++
			}
			events = append(events, syncEvent{integration: remote, state: state})
		}

		q := tx.WithContext(ctx).Model(&model.Integration{}).Where("deleted_at IS NULL")
		if len(seen) > 0 {
			q = q.Where("unique_key NOT IN ?", seen)
		}
		if err := q.Find(&missing).Error; err != nil {
			return fmt.Errorf("load integrations missing from Nango: %w", err)
		}
		for i := range missing {
			if err := tx.WithContext(ctx).Model(&missing[i]).Update("deleted_at", gorm.Expr("CURRENT_TIMESTAMP")).Error; err != nil {
				return fmt.Errorf("mark integration %s unavailable: %w", missing[i].UniqueKey, err)
			}
			result.Unavailable++
		}
		return nil
	})
	if err != nil {
		return SyncResult{Discovered: len(configured)}, err
	}
	for _, event := range events {
		remote := event.integration
		logging.FromContext(ctx).InfoContext(ctx, "nango integration synced",
			"provider_config_key", remote.UniqueKey,
			"provider", localProvider(remote),
			"nango_provider", remote.Provider,
			"display_name", remote.DisplayName,
			"state", event.state,
		)
	}
	for i := range missing {
		logging.FromContext(ctx).WarnContext(ctx, "nango integration unavailable",
			"provider_config_key", missing[i].UniqueKey,
			"provider", missing[i].Provider,
			"display_name", missing[i].DisplayName,
		)
	}
	return result, nil
}

func syncConfiguredIntegration(ctx context.Context, db *gorm.DB, client *nango.Client, cat *catalog.Catalog, remote nango.Integration) (string, error) {
	template, _ := client.GetProviderTemplate(remote.Provider)
	configInput := map[string]any{"data": map[string]any{
		"unique_key":   remote.UniqueKey,
		"provider":     remote.Provider,
		"display_name": remote.DisplayName,
		"logo":         remote.Logo,
	}}
	config := model.JSON(nango.BuildConfig(configInput, template, client.CallbackURL()))
	provider := localProvider(remote)
	botHandle := githubBotHandle(remote.UniqueKey)
	supportsRAG := providerSupportsRAG(cat, provider)

	var existing model.Integration
	err := db.WithContext(ctx).Where("unique_key = ?", remote.UniqueKey).First(&existing).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", err
	}
	created := errors.Is(err, gorm.ErrRecordNotFound)
	row := model.Integration{
		UniqueKey: remote.UniqueKey, Provider: provider, DisplayName: remote.DisplayName,
		BotHandle: botHandle, NangoConfig: config, SupportsRAGSource: supportsRAG,
	}
	if created {
		if err := db.WithContext(ctx).Create(&row).Error; err != nil {
			return "", err
		}
		return "created", nil
	}
	unchanged := existing.Provider == row.Provider &&
		existing.DisplayName == row.DisplayName &&
		existing.BotHandle == row.BotHandle &&
		existing.SupportsRAGSource == row.SupportsRAGSource &&
		existing.DeletedAt == nil && jsonEqual(existing.NangoConfig, row.NangoConfig)
	if unchanged {
		return "unchanged", nil
	}
	updates := map[string]any{
		"provider": provider, "display_name": remote.DisplayName,
		"bot_handle": botHandle, "nango_config": config,
		"supports_rag_source": supportsRAG, "deleted_at": nil,
	}
	if err := db.WithContext(ctx).Model(&existing).Updates(updates).Error; err != nil {
		return "", err
	}
	return "updated", nil
}

func localProvider(remote nango.Integration) string {
	if remote.UniqueKey == "github-app-code-reviews" {
		return remote.UniqueKey
	}
	return remote.Provider
}

func githubBotHandle(providerConfigKey string) string {
	switch strings.TrimSpace(providerConfigKey) {
	case "github-app":
		return "usehivy"
	case "github-app-code-reviews":
		return "usehivy-reviews"
	default:
		return ""
	}
}

func providerSupportsRAG(cat *catalog.Catalog, provider string) bool {
	actions, ok := cat.GetProvider(provider)
	if !ok {
		return false
	}
	for _, resource := range actions.Resources {
		if resource.RAGScopable {
			return true
		}
	}
	return false
}

func jsonEqual(a, b model.JSON) bool {
	av, err := a.Value()
	if err != nil {
		return false
	}
	bv, err := b.Value()
	return err == nil && av == bv
}
