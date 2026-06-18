package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	skillpkg "github.com/usehivy/hivy/internal/skills"
)

const syncLockKey int64 = 2026061401

type SyncResult struct {
	PluginsCreated  int
	PluginsUpdated  int
	PluginsArchived int
	SkillsCreated   int
	SkillsUpdated   int
	SkillsArchived  int
}

func SyncLocal(ctx context.Context, db *gorm.DB, dir string) (*SyncResult, error) {
	if strings.TrimSpace(dir) == "" {
		dir = "global/plugins"
	}
	resolved, err := resolveDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("plugin dir %q not found", dir)
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

	result := &SyncResult{}
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", syncLockKey).Error; err != nil {
			return fmt.Errorf("lock plugin sync: %w", err)
		}
		seenPlugins := map[string]bool{}
		for _, manifest := range manifests {
			seenPlugins[manifest.Slug] = true
			if err := syncOne(ctx, tx, manifest, result); err != nil {
				return err
			}
		}
		var existing []model.Plugin
		if err := tx.Where("status = ?", model.PluginStatusActive).Find(&existing).Error; err != nil {
			return fmt.Errorf("list active plugins for archive: %w", err)
		}
		for _, plugin := range existing {
			if seenPlugins[plugin.Slug] {
				continue
			}
			if err := archivePlugin(ctx, tx, plugin.ID); err != nil {
				return err
			}
			result.PluginsArchived++
		}
		if err := ReconcileAutoInstalled(ctx, tx); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	logging.FromContext(ctx).InfoContext(ctx, "plugins synced",
		"plugins_created", result.PluginsCreated,
		"plugins_updated", result.PluginsUpdated,
		"plugins_archived", result.PluginsArchived,
		"skills_created", result.SkillsCreated,
		"skills_updated", result.SkillsUpdated,
		"skills_archived", result.SkillsArchived,
	)
	return result, nil
}

func syncOne(ctx context.Context, tx *gorm.DB, manifest Manifest, result *SyncResult) error {
	skills, err := loadSkills(ctx, manifest)
	if err != nil {
		return err
	}
	hash, err := sourceHash(manifest, skills)
	if err != nil {
		return fmt.Errorf("hash plugin %q: %w", manifest.Slug, err)
	}
	raw := model.RawJSON(manifest.raw)
	if len(raw) == 0 {
		raw = model.RawJSON("{}")
	}
	version := strings.TrimSpace(manifest.PluginVersion)
	if version == "" {
		version = "1"
	}
	developer := strings.TrimSpace(manifest.Developer)
	if developer == "" {
		developer = "Hivy"
	}
	status := model.PluginStatusActive
	if manifest.Enabled != nil && !*manifest.Enabled {
		status = model.PluginStatusArchived
	}

	var plugin model.Plugin
	err = tx.Where("slug = ?", manifest.Slug).First(&plugin).Error
	created := false
	if err == gorm.ErrRecordNotFound {
		plugin = model.Plugin{
			ID:         uuid.New(),
			Slug:       manifest.Slug,
			Name:       manifest.Name,
			SourceHash: hash,
		}
		created = true
	} else if err != nil {
		return fmt.Errorf("load plugin %q: %w", manifest.Slug, err)
	}
	updates := map[string]any{
		"name":        manifest.Name,
		"description": manifest.Description,
		"category":    manifest.Category,
		"icon":        manifest.Icon,
		"icon_color":  manifest.IconColor,
		"developer":   developer,
		"version":     version,
		"status":      status,
		"source_hash": hash,
		"manifest":    raw,
	}
	if created {
		plugin.Description = manifest.Description
		plugin.Category = manifest.Category
		plugin.Icon = manifest.Icon
		plugin.IconColor = manifest.IconColor
		plugin.Developer = developer
		plugin.Version = version
		plugin.Status = status
		plugin.Manifest = raw
		if err := tx.Create(&plugin).Error; err != nil {
			return fmt.Errorf("create plugin %q: %w", manifest.Slug, err)
		}
		result.PluginsCreated++
	} else if plugin.SourceHash != hash || plugin.Status != status {
		if err := tx.Model(&plugin).Updates(updates).Error; err != nil {
			return fmt.Errorf("update plugin %q: %w", manifest.Slug, err)
		}
		result.PluginsUpdated++
	} else {
		if err := tx.Model(&plugin).Updates(map[string]any{
			"name":        manifest.Name,
			"description": manifest.Description,
			"category":    manifest.Category,
			"icon":        manifest.Icon,
			"icon_color":  manifest.IconColor,
			"developer":   developer,
			"version":     version,
			"manifest":    raw,
		}).Error; err != nil {
			return fmt.Errorf("touch plugin %q: %w", manifest.Slug, err)
		}
	}

	if err := replacePluginIntegrations(tx, plugin.ID, manifest.RequiredConnections); err != nil {
		return fmt.Errorf("sync plugin integrations %q: %w", manifest.Slug, err)
	}
	return syncPluginSkills(ctx, tx, plugin, skills, result)
}

func replacePluginIntegrations(tx *gorm.DB, pluginID uuid.UUID, reqs []ConnectionRequirement) error {
	if err := tx.Where("plugin_id = ?", pluginID).Delete(&model.PluginIntegration{}).Error; err != nil {
		return err
	}
	for _, req := range reqs {
		required := true
		if req.Required != nil {
			required = *req.Required
		}
		kind := strings.TrimSpace(req.Kind)
		if kind == "" {
			kind = model.PluginIntegrationKindIntegration
		}
		row := model.PluginIntegration{
			PluginID: pluginID,
			Provider: strings.TrimSpace(req.Provider),
			Kind:     kind,
			Required: required,
		}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
	}
	return nil
}

func syncPluginSkills(ctx context.Context, tx *gorm.DB, plugin model.Plugin, loaded []loadedSkill, result *SyncResult) error {
	seen := map[string]bool{}
	for _, item := range loaded {
		seen[item.Slug] = true
		desc := strings.TrimSpace(item.Manifest.Description)
		humanDesc := strings.TrimSpace(item.Manifest.HumanDescription)
		bundle := &skillpkg.Bundle{
			ID:          item.Slug,
			Title:       item.Manifest.Name,
			Description: desc,
			Content:     item.Content,
			Manifest: map[string]any{
				"name":        item.Manifest.Name,
				"description": desc,
			},
			Files:                        item.Files,
			References:                   referencesFromFiles(item.Files),
			RequiredEnvironmentVariables: normalizeStrings(item.Manifest.RequiredEnvironmentVariables),
		}
		raw, err := json.Marshal(bundle)
		if err != nil {
			return fmt.Errorf("marshal skill bundle %q: %w", item.Slug, err)
		}
		var skill model.Skill
		err = tx.Where("plugin_id = ? AND slug = ?", plugin.ID, item.Slug).First(&skill).Error
		if err == gorm.ErrRecordNotFound {
			skill = model.Skill{
				ID:               uuid.New(),
				PluginID:         &plugin.ID,
				Slug:             item.Slug,
				Name:             item.Manifest.Name,
				Description:      stringPtr(desc),
				HumanDescription: stringPtr(humanDesc),
				Category:         item.Manifest.Category,
				SourceType:       model.SkillSourceInline,
				RepoRef:          "main",
				Bundle:           model.RawJSON(raw),
				Tags:             pq.StringArray(normalizeStrings(item.Manifest.Tags)),
				Status:           model.SkillStatusPublished,
				PublishedAt:      timePtr(time.Now()),
				HydratedAt:       timePtr(time.Now()),
			}
			if err := tx.Create(&skill).Error; err != nil {
				return fmt.Errorf("create skill %q: %w", item.Slug, err)
			}
			result.SkillsCreated++
		} else if err != nil {
			return fmt.Errorf("load skill %q: %w", item.Slug, err)
		} else {
			now := time.Now()
			updates := map[string]any{
				"name":              item.Manifest.Name,
				"description":       stringPtr(desc),
				"human_description": stringPtr(humanDesc),
				"category":          item.Manifest.Category,
				"source_type":       model.SkillSourceInline,
				"repo_url":          nil,
				"repo_subpath":      nil,
				"repo_ref":          "main",
				"bundle":            model.RawJSON(raw),
				"tags":              pq.StringArray(normalizeStrings(item.Manifest.Tags)),
				"integration_ids":   pq.StringArray{},
				"hidden":            false,
				"status":            model.SkillStatusPublished,
				"hydrated_at":       &now,
				"hydration_error":   nil,
			}
			if err := tx.Model(&skill).Updates(updates).Error; err != nil {
				return fmt.Errorf("update skill %q: %w", item.Slug, err)
			}
			result.SkillsUpdated++
		}
	}

	var existing []model.Skill
	if err := tx.Where("plugin_id = ? AND status <> ?", plugin.ID, model.SkillStatusArchived).Find(&existing).Error; err != nil {
		return fmt.Errorf("list plugin skills for archive: %w", err)
	}
	for _, skill := range existing {
		if seen[skill.Slug] {
			continue
		}
		if err := tx.Model(&skill).Update("status", model.SkillStatusArchived).Error; err != nil {
			return fmt.Errorf("archive removed skill %q: %w", skill.Slug, err)
		}
		result.SkillsArchived++
	}
	return nil
}
