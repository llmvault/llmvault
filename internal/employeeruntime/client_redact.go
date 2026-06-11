package employeeruntime

import "encoding/json"

const redactedValue = "<redacted>"

// redactConfigUpdateRequest returns a copy of the runtime config payload with
// every secret removed, suitable for debug logging. It redacts the runtime
// secret, any RuntimeEnv value whose key is sensitive (per the env catalog /
// name heuristics), and MCP server Authorization headers carried in the agent
// definition. The original request is never mutated.
func redactConfigUpdateRequest(body ConfigUpdateRequest) map[string]any {
	out := map[string]any{}

	if body.RuntimeSecret != "" {
		out["runtime_secret"] = redactedValue
	}

	if len(body.RuntimeEnv) > 0 {
		env := make(map[string]string, len(body.RuntimeEnv))
		for k, v := range body.RuntimeEnv {
			if v != "" && IsSensitiveEnvKey(k) {
				env[k] = redactedValue
			} else {
				env[k] = v
			}
		}
		out["runtime_env"] = env
	}

	if body.Definition != nil {
		out["definition"] = redactDefinition(*body.Definition)
	} else {
		out["definition"] = nil
	}

	if len(body.Schedules) > 0 {
		out["schedules"] = body.Schedules
	}

	return out
}

// redactDefinition round-trips the definition through JSON into a generic map
// and redacts MCP server Authorization headers (which may carry tokens). Using
// a generic map keeps the redaction robust against the untyped McpServers slice.
func redactDefinition(def AgentDefinition) any {
	raw, err := json.Marshal(def)
	if err != nil {
		return map[string]any{"_redaction_error": "marshal failed"}
	}
	var generic map[string]any
	if err := json.Unmarshal(raw, &generic); err != nil {
		return map[string]any{"_redaction_error": "unmarshal failed"}
	}
	if servers, ok := generic["mcp_servers"].([]any); ok {
		for _, s := range servers {
			redactMCPHeaders(s)
		}
	}
	return generic
}

func redactMCPHeaders(server any) {
	m, ok := server.(map[string]any)
	if !ok {
		return
	}
	headers, ok := m["headers"].(map[string]any)
	if !ok {
		return
	}
	for key := range headers {
		if isSensitiveHeaderKey(key) {
			headers[key] = redactedValue
		}
	}
}

func isSensitiveHeaderKey(key string) bool {
	switch {
	case key == "":
		return false
	default:
		// Authorization, Proxy-Authorization, X-*-Token, etc.
		return looksSensitiveEnvKey(key)
	}
}
