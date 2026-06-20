package agentruntime

import "github.com/usehivy/hivy/internal/model"

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
