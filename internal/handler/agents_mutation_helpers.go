package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/model"
)

func agentIDFromRequest(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	agentID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid agent id"})
		return uuid.Nil, false
	}
	return agentID, true
}

func normalizeAgentAvailableModels(defaultModel string, requested *[]string) pq.StringArray {
	defaultModel = strings.TrimSpace(defaultModel)
	seen := map[string]bool{}
	out := make([]string, 0)
	if requested != nil {
		for _, modelID := range *requested {
			modelID = strings.TrimSpace(modelID)
			if modelID == "" || seen[modelID] {
				continue
			}
			seen[modelID] = true
			out = append(out, modelID)
		}
	}
	if len(out) == 0 {
		if defaultModel == "" {
			defaultModel = agentruntime.DefaultAgentModel
		}
		out = append(out, defaultModel)
	}
	return pq.StringArray(out)
}

func agentAllowsModel(agent *model.Agent, modelID string) bool {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return false
	}
	for _, allowed := range agent.AvailableModels {
		if strings.TrimSpace(allowed) == modelID {
			return true
		}
	}
	return len(agent.AvailableModels) == 0 && strings.TrimSpace(agent.Model) == modelID
}

func containsString(values []string, target string) bool {
	target = strings.TrimSpace(target)
	for _, value := range values {
		if strings.TrimSpace(value) == target {
			return true
		}
	}
	return false
}

func cleanStringPtr(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func optionalStringPtr(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func normalizeJSONPtr(value *model.JSON) model.JSON {
	if value == nil || *value == nil {
		return model.JSON{}
	}
	return *value
}

func normalizeMCPServersForRequest(w http.ResponseWriter, raw *json.RawMessage) (model.RawJSON, bool) {
	if raw == nil || len(*raw) == 0 || string(*raw) == "null" {
		return model.RawJSON("[]"), true
	}
	var servers []any
	if err := json.Unmarshal(*raw, &servers); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "mcp_servers must be an array"})
		return nil, false
	}
	clean, err := json.Marshal(servers)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid mcp_servers"})
		return nil, false
	}
	return model.RawJSON(clean), true
}

func normalizeSandboxToolsForRequest(w http.ResponseWriter, value *[]string) ([]string, bool) {
	if value == nil {
		return nil, true
	}
	out := make([]string, 0, len(*value))
	seen := map[string]bool{}
	for _, item := range *value {
		item = strings.TrimSpace(item)
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	if invalid := model.ValidateSandboxTools(out); invalid != "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: fmt.Sprintf("invalid sandbox tool %q", invalid)})
		return nil, false
	}
	return out, true
}

func normalizePermissionsForRequest(w http.ResponseWriter, value *model.JSON) (model.JSON, bool) {
	permissions := normalizeJSONPtr(value)
	for key := range permissions {
		if !model.IsValidPermissionKey(key) {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: fmt.Sprintf("invalid permission key %q", key)})
			return nil, false
		}
	}
	return permissions, true
}

func parseOptionalUUIDForRequest(w http.ResponseWriter, value *string, field string) (*uuid.UUID, bool) {
	raw := cleanStringPtr(value)
	if raw == "" {
		return nil, true
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: field + " must be a uuid"})
		return nil, false
	}
	return &id, true
}

func isValidAgentSandboxStrategy(strategy string) bool {
	return strategy == agentStrategyAlwaysOn || strategy == agentStrategyPerSession
}
