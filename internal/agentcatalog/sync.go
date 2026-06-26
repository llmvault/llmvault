package agentcatalog

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

const syncLockKey int64 = 2026061601

type SyncResult struct {
	Created  int
	Updated  int
	Archived int
}

func SyncLocal(ctx context.Context, db *gorm.DB, dir string) (*SyncResult, error) {
	if strings.TrimSpace(dir) == "" {
		dir = "global/agents"
	}
	resolved, err := resolveDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("agent catalog dir %q not found", dir)
		}
		return nil, err
	}
	manifests, err := loadManifests(resolved)
	if err != nil {
		return nil, err
	}
	if err := validateManifests(manifests); err != nil {
		return nil, err
	}
	if err := validatePluginReferences(ctx, db, manifests); err != nil {
		return nil, err
	}

	result := &SyncResult{}
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", syncLockKey).Error; err != nil {
			return fmt.Errorf("lock agent catalog sync: %w", err)
		}
		seen := map[string]bool{}
		for _, manifest := range manifests {
			seen[manifest.Slug] = true
			if err := syncOne(ctx, tx, manifest, result); err != nil {
				return err
			}
		}
		return archiveMissing(ctx, tx, seen, result)
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func validatePluginReferences(ctx context.Context, db *gorm.DB, manifests []Manifest) error {
	referenced := map[string]string{}
	for _, manifest := range manifests {
		for _, slug := range normalizeStrings(manifest.Plugins.Required) {
			referenced[slug] = manifest.Slug
		}
		for _, slug := range normalizeStrings(manifest.Plugins.Recommended) {
			referenced[slug] = manifest.Slug
		}
	}
	if len(referenced) == 0 {
		return nil
	}
	slugs := make([]string, 0, len(referenced))
	for slug := range referenced {
		slugs = append(slugs, slug)
	}
	sort.Strings(slugs)
	var active []string
	if err := db.WithContext(ctx).Model(&model.Plugin{}).
		Where("slug IN ? AND status = ?", slugs, model.PluginStatusActive).
		Pluck("slug", &active).Error; err != nil {
		return fmt.Errorf("validate agent catalog plugin references: %w", err)
	}
	found := map[string]bool{}
	for _, slug := range active {
		found[slug] = true
	}
	for _, slug := range slugs {
		if !found[slug] {
			return fmt.Errorf("agent %q references unknown or inactive plugin %q", referenced[slug], slug)
		}
	}
	return nil
}

func syncOne(ctx context.Context, tx *gorm.DB, manifest Manifest, result *SyncResult) error {
	hash, err := sourceHash(manifest)
	if err != nil {
		return fmt.Errorf("hash agent %q: %w", manifest.Slug, err)
	}
	raw := model.RawJSON(manifest.raw)
	if len(raw) == 0 {
		raw = model.RawJSON("{}")
	}
	status := model.AgentCatalogStatusActive
	if manifest.Enabled != nil && !*manifest.Enabled {
		status = model.AgentCatalogStatusArchived
	}
	var row model.AgentCatalog
	err = tx.WithContext(ctx).Where("slug = ?", manifest.Slug).First(&row).Error
	created := false
	if err == gorm.ErrRecordNotFound {
		row = model.AgentCatalog{ID: uuid.New(), Slug: manifest.Slug, SourceHash: hash}
		created = true
	} else if err != nil {
		return fmt.Errorf("load agent catalog %q: %w", manifest.Slug, err)
	}
	updates := catalogUpdates(manifest, raw, hash, status)
	if created {
		applyCatalogUpdates(&row, updates)
		if err := tx.WithContext(ctx).Create(&row).Error; err != nil {
			return fmt.Errorf("create agent catalog %q: %w", manifest.Slug, err)
		}
		result.Created++
	} else if row.SourceHash != hash || row.Status != status {
		if err := tx.WithContext(ctx).Model(&row).Updates(updates).Error; err != nil {
			return fmt.Errorf("update agent catalog %q: %w", manifest.Slug, err)
		}
		result.Updated++
	} else if err := tx.WithContext(ctx).Model(&row).Updates(updates).Error; err != nil {
		return fmt.Errorf("touch agent catalog %q: %w", manifest.Slug, err)
	}
	return nil
}

func archiveMissing(ctx context.Context, tx *gorm.DB, seen map[string]bool, result *SyncResult) error {
	var existing []model.AgentCatalog
	if err := tx.WithContext(ctx).Where("status = ?", model.AgentCatalogStatusActive).Find(&existing).Error; err != nil {
		return fmt.Errorf("list active agent catalog rows: %w", err)
	}
	for _, row := range existing {
		if seen[row.Slug] {
			continue
		}
		if err := tx.WithContext(ctx).Model(&row).
			Update("status", model.AgentCatalogStatusArchived).Error; err != nil {
			return fmt.Errorf("archive agent catalog %q: %w", row.Slug, err)
		}
		result.Archived++
	}
	return nil
}

func resolveDir(dir string) (string, error) {
	if filepath.IsAbs(dir) {
		return dir, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		candidate := filepath.Join(cwd, dir)
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate, nil
		}
		parent := filepath.Dir(cwd)
		if parent == cwd {
			break
		}
		cwd = parent
	}
	return filepath.Abs(dir)
}
