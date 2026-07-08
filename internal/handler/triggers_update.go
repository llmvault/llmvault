package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/access"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/slackapp"
)

type updateTriggerRequest struct {
	Name                 *string `json:"name,omitempty"`
	Provider             *string `json:"provider,omitempty"`
	ConnectionID         *string `json:"connection_id,omitempty"`
	ExternalResourceKey  *string `json:"external_resource_key,omitempty"`
	ExternalResourceName *string `json:"external_resource_name,omitempty"`
	AgentID              *string `json:"agent_id,omitempty"`
	ChannelID            *string `json:"channel_id,omitempty"`
	TriggerKey           *string `json:"trigger_key,omitempty"`
	TriggerValue         *string `json:"trigger_value,omitempty"`
	Enabled              *bool   `json:"enabled,omitempty"`
	Instructions         *string `json:"instructions,omitempty"`
}

// Update handles PATCH /v1/triggers/{id}.
// @Summary Update trigger
// @Description Updates a provider automation trigger.
// @Tags triggers
// @Accept json
// @Produce json
// @Param id path string true "Trigger ID"
// @Param request body updateTriggerRequest true "Trigger update"
// @Success 200 {object} triggerGetResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/triggers/{id} [patch]
func (h *TriggerHandler) Update(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok || org == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}
	id, ok := triggerIDFromString(chi.URLParam(r, "id"))
	if !ok {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "trigger not found"})
		return
	}
	var req updateTriggerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}
	trigger, status, message, err := h.update(r, org.ID, id, req)
	if err != nil {
		writeJSON(w, status, errorResponse{Error: message})
		return
	}
	writeJSON(w, http.StatusOK, triggerGetResponse{Trigger: h.triggerResponse(trigger)})
}

func (h *TriggerHandler) update(r *http.Request, orgID, id uuid.UUID, req updateTriggerRequest) (model.AgentTrigger, int, string, error) {
	current, err := h.loadTrigger(r, orgID, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return model.AgentTrigger{}, http.StatusNotFound, "trigger not found", err
		}
		return model.AgentTrigger{}, http.StatusInternalServerError, "failed to load trigger", err
	}
	if current.TriggerType == "http" {
		return h.updateHTTP(r, orgID, id, current, req)
	}
	// The provider update path historically skipped the access + manage gates
	// that Create enforces. Gate reading/mutating this trigger on the actor
	// being able to access it, matching updateHTTP and triggers create.
	actor, err := access.Resolve(r.Context(), h.db, orgID, middleware.UserID(r.Context()))
	if err != nil || !h.actorCanAccessTrigger(r.Context(), actor, current) {
		return model.AgentTrigger{}, http.StatusNotFound, "trigger not found", fmt.Errorf("trigger not accessible")
	}
	parsed, template, err := parseTriggerUpdate(current, req)
	if err != nil {
		return model.AgentTrigger{}, http.StatusBadRequest, err.Error(), err
	}
	conn, err := loadTriggerConnection(h.db.WithContext(r.Context()), orgID, parsed.connectionID, parsed.provider)
	if err != nil {
		status, message := triggerCreateError(err)
		return model.AgentTrigger{}, status, message, err
	}
	rawChannelID := ""
	if req.ChannelID != nil {
		rawChannelID = *req.ChannelID
	}
	channelID, status, message, err := h.resolveProviderTriggerChannel(
		r, orgID, parsed.provider, conn, template, parsed.resourceKey, parsed.resourceName, parsed.agentID, rawChannelID,
	)
	if err != nil {
		return model.AgentTrigger{}, status, message, err
	}
	// Re-binding the trigger to this channel is a manage-the-team action, not
	// merely use-the-channel (Create enforces this too).
	if mStatus, mMessage, mErr := requireChannelBindingManage(r, h.db, orgID, channelID); mErr != nil {
		return model.AgentTrigger{}, mStatus, mMessage, mErr
	}
	name := current.Name
	if req.Name != nil {
		name = strings.TrimSpace(*req.Name)
	}
	err = h.db.WithContext(r.Context()).Transaction(func(tx *gorm.DB) error {
		if err := validateTriggerAgent(r.Context(), tx, orgID, parsed.agentID, channelID, template); err != nil {
			return err
		}
		return tx.Model(&model.AgentTrigger{}).
			Where("id = ? AND org_id = ?", id, orgID).
			Updates(map[string]any{
				"agent_id":      parsed.agentID,
				"name":          name,
				"channel_id":    channelID,
				"connection_id": conn.ID,
				"trigger_keys":  pq.StringArray(template.triggerKeys),
				"trigger_key":   parsed.triggerKey,
				"trigger_value": parsed.triggerValue,
				"source_slug":   triggerSourceSlug(parsed.provider, parsed.triggerKey, parsed.resourceKey, parsed.triggerValue),
				"instructions":  parsed.instructions,
				"trigger_type":  "webhook",
				"enabled":       parsed.enabled,
				"conditions":    nil,
				"secret_key":    "",
			}).Error
	})
	if err != nil {
		status, message := triggerCreateError(err)
		return model.AgentTrigger{}, status, message, err
	}
	updated, err := h.loadTrigger(r, orgID, id)
	if err != nil {
		return model.AgentTrigger{}, http.StatusInternalServerError, "failed to load trigger", err
	}
	return updated, http.StatusOK, "", nil
}

// updateHTTP updates an inbound HTTP ("webhook") trigger: enable/disable and
// instructions only. The channel/agent/secret are set at create time.
func (h *TriggerHandler) updateHTTP(r *http.Request, orgID, id uuid.UUID, current model.AgentTrigger, req updateTriggerRequest) (model.AgentTrigger, int, string, error) {
	actor, err := access.Resolve(r.Context(), h.db, orgID, middleware.UserID(r.Context()))
	if err != nil || !h.actorCanAccessTrigger(r.Context(), actor, current) {
		return model.AgentTrigger{}, http.StatusNotFound, "trigger not found", fmt.Errorf("trigger not accessible")
	}
	updates := map[string]any{}
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			return model.AgentTrigger{}, http.StatusBadRequest, "name is required", fmt.Errorf("empty name")
		}
		updates["name"] = name
	}
	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}
	if req.Instructions != nil {
		instructions := strings.TrimSpace(*req.Instructions)
		if instructions == "" {
			return model.AgentTrigger{}, http.StatusBadRequest, "instructions are required", fmt.Errorf("empty instructions")
		}
		updates["instructions"] = instructions
	}
	if len(updates) > 0 {
		if err := h.db.WithContext(r.Context()).Model(&model.AgentTrigger{}).
			Where("id = ? AND org_id = ?", id, orgID).
			Updates(updates).Error; err != nil {
			return model.AgentTrigger{}, http.StatusInternalServerError, "failed to update trigger", err
		}
	}
	updated, err := h.loadTrigger(r, orgID, id)
	if err != nil {
		return model.AgentTrigger{}, http.StatusInternalServerError, "failed to load trigger", err
	}
	return updated, http.StatusOK, "", nil
}

type parsedTriggerUpdate struct {
	provider     string
	connectionID uuid.UUID
	resourceKey  string
	resourceName string
	agentID      uuid.UUID
	triggerKey   string
	triggerValue string
	enabled      bool
	instructions string
}

func parseTriggerUpdate(current model.AgentTrigger, req updateTriggerRequest) (parsedTriggerUpdate, triggerTemplate, error) {
	parsed := parsedTriggerUpdate{
		provider:     defaultString(triggerProvider(current), slackapp.Provider),
		connectionID: uuid.Nil,
		resourceKey:  "",
		resourceName: "",
		agentID:      current.AgentID,
		triggerKey:   current.TriggerKey,
		triggerValue: current.TriggerValue,
		enabled:      current.Enabled,
		instructions: current.Instructions,
	}
	if current.ConnectionID != nil {
		parsed.connectionID = *current.ConnectionID
	}
	if current.Channel != nil {
		parsed.resourceKey = current.Channel.ExternalResourceKey
		parsed.resourceName = current.Channel.ExternalResourceName
	}
	if req.Provider != nil {
		parsed.provider = strings.ToLower(strings.TrimSpace(*req.Provider))
	}
	if req.ConnectionID != nil {
		id, err := uuid.Parse(strings.TrimSpace(*req.ConnectionID))
		if err != nil || id == uuid.Nil {
			return parsedTriggerUpdate{}, triggerTemplate{}, fmt.Errorf("connection_id must be a uuid")
		}
		parsed.connectionID = id
	}
	if req.ExternalResourceKey != nil {
		parsed.resourceKey = strings.TrimSpace(*req.ExternalResourceKey)
	}
	if req.ExternalResourceName != nil {
		parsed.resourceName = strings.TrimSpace(*req.ExternalResourceName)
	}
	if req.AgentID != nil {
		id, err := uuid.Parse(strings.TrimSpace(*req.AgentID))
		if err != nil || id == uuid.Nil {
			return parsedTriggerUpdate{}, triggerTemplate{}, fmt.Errorf("agent_id must be a uuid")
		}
		parsed.agentID = id
	}
	if req.TriggerKey != nil {
		parsed.triggerKey = strings.TrimSpace(*req.TriggerKey)
	}
	if req.TriggerValue != nil {
		parsed.triggerValue = normalizeTriggerValue(*req.TriggerValue)
	}
	if req.Enabled != nil {
		parsed.enabled = *req.Enabled
	}
	if req.Instructions != nil {
		parsed.instructions = strings.TrimSpace(*req.Instructions)
	}
	return validateParsedTriggerUpdate(parsed)
}

func validateParsedTriggerUpdate(parsed parsedTriggerUpdate) (parsedTriggerUpdate, triggerTemplate, error) {
	if !triggerProviderSupported(parsed.provider) {
		return parsed, triggerTemplate{}, fmt.Errorf("provider is not supported")
	}
	template, ok := resolveTriggerTemplate(parsed.provider, defaultTriggerKey(parsed.provider, parsed.triggerKey))
	if !ok {
		return parsed, triggerTemplate{}, fmt.Errorf("trigger_key is not supported")
	}
	parsed.triggerKey = template.key
	if parsed.connectionID == uuid.Nil {
		return parsed, triggerTemplate{}, fmt.Errorf("connection_id must be a uuid")
	}
	if parsed.resourceKey == "" {
		return parsed, triggerTemplate{}, fmt.Errorf("external_resource_key is required")
	}
	if template.valueFromResource {
		parsed.triggerValue = template.triggerValue("", parsed.resourceKey)
	}
	if parsed.triggerValue == "" {
		return parsed, triggerTemplate{}, fmt.Errorf("trigger_value is required")
	}
	if parsed.agentID == uuid.Nil {
		return parsed, triggerTemplate{}, fmt.Errorf("agent_id must be a uuid")
	}
	if parsed.instructions == "" {
		return parsed, triggerTemplate{}, fmt.Errorf("instructions are required")
	}
	return parsed, template, nil
}
