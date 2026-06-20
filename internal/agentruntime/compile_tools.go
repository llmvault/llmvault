package agentruntime

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

const (
	defaultRuntimeMaxFileSizeBytes  = 5 * 1024 * 1024
	defaultSearchMaxOutputBytes     = 256 * 1024
	defaultSearchMaxResults         = 100
	defaultSearchTimeoutSeconds     = 10
	defaultBashTimeoutSeconds       = 60
	defaultLspTimeoutSeconds        = 15
	defaultRuntimeBashMaxOutputSize = 5 * 1024 * 1024
)

var runtimeToolOrder = []string{
	"bash",
	"read_file",
	"write_file",
	"file_search",
	"glob",
	"grep",
	"multi_grep",
	"apply_patch",
	"lsp",
	"cron",
	"subagent_task",
	"check_subagent_task_status",
	"check_bash_status",
	"wake",
	"skills_list",
	"skill_view",
	"skill_manage",
	"search_sessions",
	"request_user_input",
	"update_plan",
}

func buildRuntimeTools(ctx context.Context, db *gorm.DB, agent *model.Agent) ([]map[string]any, error) {
	selection, err := agentRuntimeToolSelection(ctx, db, agent)
	if err != nil {
		return nil, err
	}
	return buildRuntimeToolsFromSelection(selection)
}

func agentRuntimeToolSelection(ctx context.Context, db *gorm.DB, agent *model.Agent) (model.JSON, error) {
	if agent == nil {
		return model.JSON{}, nil
	}
	if len(agent.Tools) > 0 {
		return agent.Tools, nil
	}
	if agent.AgentCatalog != nil && len(agent.AgentCatalog.Tools) > 0 {
		return agent.AgentCatalog.Tools, nil
	}
	if db == nil || agent.AgentCatalogID == nil {
		return model.JSON{}, nil
	}
	var catalog model.AgentCatalog
	err := db.WithContext(ctx).
		Select("tools").
		Where("id = ? AND status = ?", *agent.AgentCatalogID, model.AgentCatalogStatusActive).
		First(&catalog).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("load catalog tools: %w", err)
	}
	return catalog.Tools, nil
}

func buildRuntimeToolsFromSelection(selection model.JSON) ([]map[string]any, error) {
	if len(selection) == 0 {
		return nil, nil
	}
	normalized, err := normalizeRuntimeToolSelection(selection)
	if err != nil {
		return nil, err
	}
	if len(normalized) == 0 {
		return nil, nil
	}
	out := make([]map[string]any, 0, len(normalized))
	for _, id := range runtimeToolOrder {
		value, ok := normalized[id]
		if !ok {
			continue
		}
		spec, err := runtimeToolSpec(id, value)
		if err != nil {
			return nil, err
		}
		out = append(out, spec)
	}
	return out, nil
}

func normalizeRuntimeToolSelection(selection model.JSON) (map[string]any, error) {
	keys := make([]string, 0, len(selection))
	for key := range selection {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := map[string]any{}
	for _, key := range keys {
		value := selection[key]
		if !runtimeToolSelectionEnabled(value) {
			continue
		}
		ids, known := expandRuntimeToolID(key)
		if !known {
			return nil, fmt.Errorf("unknown runtime tool %q", key)
		}
		for _, id := range ids {
			if id == "" {
				continue
			}
			if _, exists := out[id]; exists && !isCanonicalRuntimeToolKey(key, id) {
				continue
			}
			out[id] = value
		}
	}
	return out, nil
}

func runtimeToolSelectionEnabled(value any) bool {
	switch typed := value.(type) {
	case nil:
		return false
	case bool:
		return typed
	case map[string]any:
		if enabled, ok := typed["enabled"].(bool); ok && !enabled {
			return false
		}
		return true
	case model.JSON:
		if enabled, ok := typed["enabled"].(bool); ok && !enabled {
			return false
		}
		return true
	default:
		return true
	}
}

func expandRuntimeToolID(raw string) ([]string, bool) {
	key := strings.TrimSpace(raw)
	key = strings.TrimPrefix(key, "builtin.")
	if model.IsValidRuntimeBuiltInToolID(key) {
		return []string{key}, true
	}
	switch key {
	case "Read":
		return []string{"read_file"}, true
	case "write":
		return []string{"write_file"}, true
	case "edit", "multiedit":
		return []string{"write_file"}, true
	case "LS":
		return []string{"glob"}, true
	case "todowrite", "todoread":
		return []string{"update_plan"}, true
	case "skill":
		return []string{"skills_list", "skill_view", "skill_manage"}, true
	default:
		return nil, model.IsValidPermissionKey(key)
	}
}

func isCanonicalRuntimeToolKey(raw, id string) bool {
	key := strings.TrimSpace(raw)
	return key == id || strings.TrimPrefix(key, "builtin.") == id
}

func runtimeToolSpec(id string, value any) (map[string]any, error) {
	spec := map[string]any{"type": "builtin." + id}
	config, hasConfig, err := runtimeToolConfig(id, value)
	if err != nil {
		return nil, err
	}
	if hasConfig {
		spec["config"] = config
	}
	return spec, nil
}

func runtimeToolConfig(id string, value any) (map[string]any, bool, error) {
	defaults, configurable := defaultRuntimeToolConfig(id)
	overrides, hasOverrides, err := runtimeToolConfigOverrides(id, value)
	if err != nil {
		return nil, false, err
	}
	if !configurable {
		if hasOverrides && len(overrides) > 0 {
			return nil, false, fmt.Errorf("runtime tool %q does not accept config", id)
		}
		return nil, false, nil
	}
	for key, item := range overrides {
		defaults[key] = item
	}
	return defaults, true, nil
}

func runtimeToolConfigOverrides(id string, value any) (map[string]any, bool, error) {
	switch typed := value.(type) {
	case nil, bool:
		return map[string]any{}, false, nil
	case map[string]any:
		return cloneRuntimeToolConfig(typed), true, nil
	case model.JSON:
		return cloneRuntimeToolConfig(map[string]any(typed)), true, nil
	default:
		return nil, false, fmt.Errorf("runtime tool %q value must be a boolean or config object", id)
	}
}

func defaultRuntimeToolConfig(id string) (map[string]any, bool) {
	switch id {
	case "bash":
		return map[string]any{
			"workdir":          ".",
			"timeout_seconds":  defaultBashTimeoutSeconds,
			"max_output_bytes": defaultRuntimeBashMaxOutputSize,
			"deny_patterns": []any{
				"rm -rf /",
				"rm -rf ~",
				"mkfs",
				"dd if=",
				":(){:|:&};:",
				"shutdown",
				"reboot",
			},
			"env_passthrough": []any{"HOME", "PATH", "LANG", "LC_ALL"},
			"sandbox":         "process_isolated",
		}, true
	case "read_file":
		return map[string]any{
			"allowed_roots":       []any{},
			"max_file_size_bytes": defaultRuntimeMaxFileSizeBytes,
			"deny_globs":          []any{},
		}, true
	case "write_file":
		return map[string]any{
			"allowed_roots":       []any{},
			"max_file_size_bytes": defaultRuntimeMaxFileSizeBytes,
			"deny_globs":          []any{},
			"atomic":              true,
		}, true
	case "file_search", "glob", "grep", "multi_grep":
		return map[string]any{
			"allowed_roots":           []any{},
			"deny_globs":              []any{},
			"max_results":             defaultSearchMaxResults,
			"max_output_bytes":        defaultSearchMaxOutputBytes,
			"timeout_seconds":         defaultSearchTimeoutSeconds,
			"include_hidden":          false,
			"respect_gitignore":       true,
			"follow_symlinks":         false,
			"enable_content_indexing": true,
		}, true
	case "apply_patch":
		return map[string]any{
			"allowed_roots":       []any{},
			"max_file_size_bytes": defaultRuntimeMaxFileSizeBytes,
			"deny_globs":          []any{},
			"atomic":              true,
		}, true
	case "lsp":
		return map[string]any{
			"enabled":          true,
			"allowed_roots":    []any{},
			"timeout_seconds":  defaultLspTimeoutSeconds,
			"fallback_enabled": true,
			"servers":          []any{},
		}, true
	case "subagent_task":
		return map[string]any{
			"agents": []any{},
		}, true
	default:
		return nil, false
	}
}

func cloneRuntimeToolConfig(config map[string]any) map[string]any {
	out := make(map[string]any, len(config))
	for key, value := range config {
		out[key] = cloneRuntimeToolConfigValue(value)
	}
	return out
}

func cloneRuntimeToolConfigValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneRuntimeToolConfig(typed)
	case model.JSON:
		return cloneRuntimeToolConfig(map[string]any(typed))
	case []any:
		out := make([]any, len(typed))
		for index, item := range typed {
			out[index] = cloneRuntimeToolConfigValue(item)
		}
		return out
	default:
		return typed
	}
}
