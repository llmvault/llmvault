package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/slackapp"
)

type TriggerHandler struct {
	db *gorm.DB
}

func NewTriggerHandler(db *gorm.DB) *TriggerHandler {
	return &TriggerHandler{db: db}
}

type createTriggerRequest struct {
	Provider     string `json:"provider"`
	ConnectionID string `json:"connection_id"`
	ChannelID    string `json:"channel_id"`
	AgentID      string `json:"agent_id"`
	TriggerKey   string `json:"trigger_key"`
	TriggerValue string `json:"trigger_value"`
	Instructions string `json:"instructions"`
}

func (h *TriggerHandler) Create(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok || org == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}
	var req createTriggerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	trigger, provider, status, message, err := h.create(r, org.ID, req)
	if err != nil {
		writeJSON(w, status, map[string]string{"error": message})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]agentTriggerResponse{
		"trigger": triggerToResponse(trigger, provider),
	})
}

func (h *TriggerHandler) create(r *http.Request, orgID uuid.UUID, req createTriggerRequest) (model.AgentTrigger, string, int, string, error) {
	provider := strings.ToLower(strings.TrimSpace(req.Provider))
	if provider == "" {
		provider = slackapp.Provider
	}
	if provider != slackapp.Provider {
		return model.AgentTrigger{}, "", http.StatusBadRequest, "provider is not supported", fmt.Errorf("unsupported provider")
	}
	triggerKey := strings.TrimSpace(req.TriggerKey)
	if triggerKey == "" {
		triggerKey = slackapp.EventReactionAdded
	}
	if triggerKey != slackapp.EventReactionAdded {
		return model.AgentTrigger{}, "", http.StatusBadRequest, "trigger_key is not supported", fmt.Errorf("unsupported trigger key")
	}
	triggerValue := normalizeTriggerValue(req.TriggerValue)
	if triggerValue == "" {
		return model.AgentTrigger{}, "", http.StatusBadRequest, "trigger_value is required", fmt.Errorf("missing trigger value")
	}
	if strings.TrimSpace(req.Instructions) == "" {
		return model.AgentTrigger{}, "", http.StatusBadRequest, "instructions are required", fmt.Errorf("missing instructions")
	}
	connectionID, err := uuid.Parse(strings.TrimSpace(req.ConnectionID))
	if err != nil || connectionID == uuid.Nil {
		return model.AgentTrigger{}, "", http.StatusBadRequest, "connection_id must be a uuid", fmt.Errorf("invalid connection id")
	}
	channelID, err := uuid.Parse(strings.TrimSpace(req.ChannelID))
	if err != nil || channelID == uuid.Nil {
		return model.AgentTrigger{}, "", http.StatusBadRequest, "channel_id must be a uuid", fmt.Errorf("invalid channel id")
	}
	agentID, err := uuid.Parse(strings.TrimSpace(req.AgentID))
	if err != nil || agentID == uuid.Nil {
		return model.AgentTrigger{}, "", http.StatusBadRequest, "agent_id must be a uuid", fmt.Errorf("invalid agent id")
	}

	var trigger model.AgentTrigger
	err = h.db.WithContext(r.Context()).Transaction(func(tx *gorm.DB) error {
		conn, err := loadTriggerConnection(tx, orgID, connectionID, provider)
		if err != nil {
			return err
		}
		channel, err := loadTriggerExternalChannel(tx, orgID, channelID, conn.ID, provider)
		if err != nil {
			return err
		}
		if err := validateTriggerAgent(tx, orgID, agentID, channel.ID); err != nil {
			return err
		}
		trigger = model.AgentTrigger{
			OrgID:        orgID,
			AgentID:      agentID,
			TriggerType:  "webhook",
			ChannelID:    &channel.ID,
			ConnectionID: &conn.ID,
			TriggerKeys:  pq.StringArray{triggerKey},
			TriggerKey:   triggerKey,
			TriggerValue: triggerValue,
			Enabled:      true,
			SourceSlug:   triggerSourceSlug(provider, triggerKey, channel.ExternalResourceKey, triggerValue),
			Instructions: strings.TrimSpace(req.Instructions),
		}
		if err := tx.Create(&trigger).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		status, message := triggerCreateError(err)
		return model.AgentTrigger{}, provider, status, message, err
	}
	return trigger, provider, http.StatusCreated, "", nil
}

func loadTriggerConnection(db *gorm.DB, orgID, connectionID uuid.UUID, provider string) (model.Connection, error) {
	var conn model.Connection
	err := db.
		Joins("JOIN integrations ON integrations.id = connections.integration_id").
		Where("connections.id = ? AND connections.org_id = ? AND connections.revoked_at IS NULL", connectionID, orgID).
		Where("integrations.provider = ?", provider).
		First(&conn).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.Connection{}, fmt.Errorf("connection not found")
	}
	if err != nil {
		return model.Connection{}, fmt.Errorf("load connection: %w", err)
	}
	return conn, nil
}

func loadTriggerExternalChannel(db *gorm.DB, orgID, channelID, connectionID uuid.UUID, provider string) (model.Channel, error) {
	var channel model.Channel
	err := db.
		Where("id = ? AND org_id = ? AND archived_at IS NULL", channelID, orgID).
		Where("origin = ? AND external_provider = ?", "external", provider).
		Where("external_connection_id = ? AND external_resource_type = ?", connectionID, "slack_channel").
		First(&channel).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.Channel{}, fmt.Errorf("external channel not found")
	}
	if err != nil {
		return model.Channel{}, fmt.Errorf("load external channel: %w", err)
	}
	return channel, nil
}

func validateTriggerAgent(db *gorm.DB, orgID, agentID, channelID uuid.UUID) error {
	var count int64
	if err := db.Model(&model.Agent{}).
		Where("id = ? AND org_id = ? AND status <> ?", agentID, orgID, "archived").
		Count(&count).Error; err != nil {
		return fmt.Errorf("load agent: %w", err)
	}
	if count == 0 {
		return fmt.Errorf("agent not found")
	}
	allowed, err := agentAllowedInTriggerChannel(db, orgID, agentID, channelID)
	if err != nil {
		return err
	}
	if !allowed {
		return fmt.Errorf("agent is not available in this channel")
	}
	return nil
}

func normalizeTriggerValue(value string) string {
	return strings.ToLower(strings.Trim(strings.TrimSpace(value), ":"))
}

func triggerSourceSlug(provider, key, resourceKey, value string) string {
	return strings.Join([]string{provider, key, resourceKey, value}, ":")
}

func triggerCreateError(err error) (int, string) {
	if isDuplicateKeyError(err) {
		return http.StatusConflict, "trigger already exists"
	}
	switch {
	case strings.Contains(err.Error(), "not found"):
		return http.StatusNotFound, err.Error()
	case strings.Contains(err.Error(), "not available"):
		return http.StatusForbidden, err.Error()
	default:
		return http.StatusInternalServerError, "failed to create trigger"
	}
}

func triggerToResponse(trigger model.AgentTrigger, provider string) agentTriggerResponse {
	response := agentTriggerResponse{
		ID:           trigger.ID.String(),
		TriggerType:  trigger.TriggerType,
		Provider:     provider,
		TriggerKeys:  []string(trigger.TriggerKeys),
		TriggerKey:   trigger.TriggerKey,
		TriggerValue: trigger.TriggerValue,
		Enabled:      trigger.Enabled,
		SourceSlug:   trigger.SourceSlug,
		Instructions: trigger.Instructions,
		SecretSet:    trigger.SecretKey != "",
	}
	if trigger.ChannelID != nil {
		response.ChannelID = trigger.ChannelID.String()
	}
	if trigger.ConnectionID != nil {
		response.ConnectionID = trigger.ConnectionID.String()
	}
	return response
}
