package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

// agentActor resolves the requesting actor's org role and user id for
// team-scoped agent authorization. trusted is true when there is no human actor
// to authorize on: an API-key caller, or a request with no user context at all
// (system/internal calls; the agent-write routes are behind auth middleware that
// guarantees an authenticated principal upstream, so a plain member always
// resolves to a user + role here). trusted callers bypass the team-membership
// check, mirroring access.Resolve treating an empty actor as "no human, fall
// back". On a DB error it writes a 500 and returns ok=false.
func (h *AgentHandler) agentActor(ctx context.Context, w http.ResponseWriter, orgID uuid.UUID) (userID *uuid.UUID, role string, trusted, ok bool) {
	if isAPIKeyRequest(ctx) {
		return nil, "", true, true
	}
	uid, hasUser := currentRequestUserID(ctx)
	if !hasUser {
		return nil, "", true, true
	}
	resolvedRole, err := orgRoleForUser(ctx, h.db, orgID, uid)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to resolve access"})
		return nil, "", false, false
	}
	return uid, resolvedRole, false, true
}

func (h *AgentHandler) teamExistsInOrg(ctx context.Context, orgID, teamID uuid.UUID) bool {
	var count int64
	_ = h.db.WithContext(ctx).Model(&model.Team{}).
		Where("id = ? AND org_id = ? AND archived_at IS NULL", teamID, orgID).
		Count(&count).Error
	return count > 0
}

// resolveAndAuthorizeAgentTeam parses the optional team_id for agent creation,
// validates it is an active team in the org, and authorizes the actor to create
// an agent in it. Members must belong to the team; org managers and API-key
// callers may target any team. A missing team_id is manager-only: there is no
// team to authorize a plain member against, so unassigned agents stay a manager
// action (mirrors the update/delete gate for team_id IS NULL). Writes an error
// response and returns ok=false when not allowed.
func (h *AgentHandler) resolveAndAuthorizeAgentTeam(ctx context.Context, w http.ResponseWriter, orgID uuid.UUID, raw *string) (*uuid.UUID, bool) {
	userID, role, trusted, ok := h.agentActor(ctx, w, orgID)
	if !ok {
		return nil, false
	}
	value := cleanStringPtr(raw)
	if value == "" {
		if trusted || isOrgManager(role) {
			return nil, true
		}
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "team_id is required: pick a team you belong to for this agent"})
		return nil, false
	}
	teamID, err := uuid.Parse(value)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "team_id must be a uuid"})
		return nil, false
	}
	if !h.teamExistsInOrg(ctx, orgID, teamID) {
		writeJSON(w, http.StatusUnprocessableEntity, errorResponse{Error: "team_id must be an active team in this org"})
		return nil, false
	}
	if trusted || canManageTeamResource(ctx, h.db, orgID, userID, role, teamID) {
		return &teamID, true
	}
	writeJSON(w, http.StatusForbidden, errorResponse{Error: "you must be a member of the team to create an agent in it"})
	return nil, false
}

// authorizeAgentMutation gates update/archive of an existing agent on the actor
// being able to manage the agent's owning team. API-key callers are trusted
// org-wide. Writes a 403/500 and returns false when not allowed.
func (h *AgentHandler) authorizeAgentMutation(ctx context.Context, w http.ResponseWriter, orgID uuid.UUID, agent *model.Agent) bool {
	userID, role, trusted, ok := h.agentActor(ctx, w, orgID)
	if !ok {
		return false
	}
	if trusted {
		return true
	}
	if canManageTeamResource(ctx, h.db, orgID, userID, role, agent.TeamID) {
		return true
	}
	writeJSON(w, http.StatusForbidden, errorResponse{Error: "you must be a member of the agent's team to manage it"})
	return false
}

func agentIDFromRequest(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	agentID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid agent id"})
		return uuid.Nil, false
	}
	return agentID, true
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

func cloneModelJSON(value model.JSON) model.JSON {
	if len(value) == 0 {
		return model.JSON{}
	}
	out := make(model.JSON, len(value))
	for key, item := range value {
		out[key] = cloneModelJSONValue(item)
	}
	return out
}

func cloneModelJSONValue(value any) any {
	switch typed := value.(type) {
	case model.JSON:
		return cloneModelJSON(typed)
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			out[key] = cloneModelJSONValue(item)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for index, item := range typed {
			out[index] = cloneModelJSONValue(item)
		}
		return out
	default:
		return typed
	}
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

func normalizeAgentSandboxSizeForRequest(w http.ResponseWriter, value *string) (string, bool) {
	size := cleanStringPtr(value)
	if size == "" {
		return model.DefaultAgentSandboxSize, true
	}
	if !model.ValidTemplateSize(size) {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "sandbox_size must be nano, small, medium, or large"})
		return "", false
	}
	normalized := model.NormalizeTemplateSize(size)
	if normalized == "xlarge" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "xlarge sandboxes are temporarily unavailable"})
		return "", false
	}
	return normalized, true
}

func (h *AgentHandler) validateAgentSandboxTemplateForRequest(
	ctx context.Context,
	w http.ResponseWriter,
	orgID uuid.UUID,
	templateID *uuid.UUID,
) bool {
	if templateID == nil {
		return true
	}
	var tmpl model.SandboxTemplate
	err := h.db.WithContext(ctx).
		Where("id = ? AND (org_id = ? OR org_id IS NULL)", *templateID, orgID).
		First(&tmpl).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "sandbox template not found"})
		return false
	}
	if err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "load agent sandbox template", "org_id", orgID, "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to validate sandbox template"})
		return false
	}
	return true
}

func normalizeAgentSandboxImageForRequest(w http.ResponseWriter, value *string) (string, bool) {
	image := cleanStringPtr(value)
	if image == "" {
		return model.SandboxImageDefault, true
	}
	if !model.ValidSandboxImage(image) {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "sandbox_image must be default or developer"})
		return "", false
	}
	return model.NormalizeSandboxImage(image), true
}
