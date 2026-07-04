package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/connectionaccess"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/slackapp"
)

type TriggerHandler struct {
	db                  *gorm.DB
	externalProvisioner ChannelExternalProvisioner
}

type TriggerHandlerOption func(*TriggerHandler)

func NewTriggerHandler(db *gorm.DB, opts ...TriggerHandlerOption) *TriggerHandler {
	h := &TriggerHandler{db: db}
	for _, opt := range opts {
		if opt != nil {
			opt(h)
		}
	}
	return h
}

func WithTriggerExternalProvisioner(p ChannelExternalProvisioner) TriggerHandlerOption {
	return func(h *TriggerHandler) {
		h.externalProvisioner = p
	}
}

type createTriggerRequest struct {
	Provider             string `json:"provider"`
	ConnectionID         string `json:"connection_id"`
	ExternalResourceKey  string `json:"external_resource_key"`
	ExternalResourceName string `json:"external_resource_name"`
	AgentID              string `json:"agent_id"`
	TriggerKey           string `json:"trigger_key"`
	TriggerValue         string `json:"trigger_value"`
	Instructions         string `json:"instructions"`
}

type createTriggerResponse struct {
	Trigger agentTriggerResponse `json:"trigger"`
}

// Create handles POST /v1/triggers.
// @Summary Create trigger
// @Description Creates a provider automation trigger for an agent.
// @Tags triggers
// @Accept json
// @Produce json
// @Param request body createTriggerRequest true "Trigger configuration"
// @Success 201 {object} createTriggerResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/triggers [post]
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
	writeJSON(w, http.StatusCreated, createTriggerResponse{Trigger: triggerToResponse(trigger, provider)})
}

func (h *TriggerHandler) create(r *http.Request, orgID uuid.UUID, req createTriggerRequest) (model.AgentTrigger, string, int, string, error) {
	provider := strings.ToLower(strings.TrimSpace(req.Provider))
	if provider == "" {
		provider = slackapp.Provider
	}
	if !triggerProviderSupported(provider) {
		return model.AgentTrigger{}, "", http.StatusBadRequest, "provider is not supported", fmt.Errorf("unsupported provider")
	}
	triggerKey := defaultTriggerKey(provider, strings.TrimSpace(req.TriggerKey))
	template, ok := resolveTriggerTemplate(provider, triggerKey)
	if !ok {
		return model.AgentTrigger{}, "", http.StatusBadRequest, "trigger_key is not supported", fmt.Errorf("unsupported trigger key")
	}
	if strings.TrimSpace(req.Instructions) == "" {
		return model.AgentTrigger{}, "", http.StatusBadRequest, "instructions are required", fmt.Errorf("missing instructions")
	}
	connectionID, err := uuid.Parse(strings.TrimSpace(req.ConnectionID))
	if err != nil || connectionID == uuid.Nil {
		return model.AgentTrigger{}, "", http.StatusBadRequest, "connection_id must be a uuid", fmt.Errorf("invalid connection id")
	}
	resourceKey := strings.TrimSpace(req.ExternalResourceKey)
	if resourceKey == "" {
		return model.AgentTrigger{}, "", http.StatusBadRequest, "external_resource_key is required", fmt.Errorf("missing external resource key")
	}
	triggerValue := template.triggerValue(req.TriggerValue, resourceKey)
	if triggerValue == "" {
		return model.AgentTrigger{}, "", http.StatusBadRequest, "trigger_value is required", fmt.Errorf("missing trigger value")
	}
	resourceName := strings.TrimSpace(req.ExternalResourceName)
	agentID, err := uuid.Parse(strings.TrimSpace(req.AgentID))
	if err != nil || agentID == uuid.Nil {
		return model.AgentTrigger{}, "", http.StatusBadRequest, "agent_id must be a uuid", fmt.Errorf("invalid agent id")
	}

	conn, err := loadTriggerConnection(h.db.WithContext(r.Context()), orgID, connectionID, provider)
	if err != nil {
		status, message := triggerCreateError(err)
		return model.AgentTrigger{}, provider, status, message, err
	}
	channel, err := findOrAutoCreateExternalChannel(r.Context(), h.db, h.externalProvisioner, orgID, externalChannelAutoCreateRequest{
		Provider:       provider,
		Connection:     conn,
		ResourceType:   template.resourceType,
		ResourceKey:    resourceKey,
		ResourceName:   resourceName,
		DefaultAgentID: agentID,
	})
	if err != nil {
		status, message := triggerCreateError(err)
		return model.AgentTrigger{}, provider, status, message, err
	}

	var trigger model.AgentTrigger
	err = h.db.WithContext(r.Context()).Transaction(func(tx *gorm.DB) error {
		if err := validateTriggerAgent(r.Context(), tx, orgID, agentID, channel.ID, template); err != nil {
			return err
		}
		trigger = model.AgentTrigger{
			OrgID:        orgID,
			AgentID:      agentID,
			TriggerType:  "webhook",
			ChannelID:    &channel.ID,
			ConnectionID: &conn.ID,
			TriggerKeys:  pq.StringArray(template.triggerKeys),
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

func validateTriggerAgent(ctx context.Context, db *gorm.DB, orgID, agentID, channelID uuid.UUID, template triggerTemplate) error {
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
	return validateTriggerAgentPlugin(ctx, db, orgID, agentID, template)
}

// validateTriggerAgentPlugin enforces that the target agent can actually use
// the provider the trigger's playbook depends on. It reuses the same resolver
// the runtime uses to authenticate provider tooling, so a passing check means
// the agent has the plugin installed and a usable connection.
func validateTriggerAgentPlugin(ctx context.Context, db *gorm.DB, orgID, agentID uuid.UUID, template triggerTemplate) error {
	if len(template.requiredProviders) == 0 {
		return nil
	}
	_, err := connectionaccess.ResolveAgentProviderAny(ctx, db, orgID, agentID, template.requiredProviders...)
	if err == nil {
		return nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		label := template.requiredPluginLabel
		if label == "" {
			label = "required"
		}
		return fmt.Errorf("agent is missing the required %s plugin", label)
	}
	return fmt.Errorf("check agent plugin access: %w", err)
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
	var provisionErr *ChannelExternalProvisionError
	if errors.As(err, &provisionErr) {
		return externalProvisionResponse(provisionErr)
	}
	switch {
	case strings.Contains(err.Error(), "not found"):
		return http.StatusNotFound, err.Error()
	case strings.Contains(err.Error(), "not available"):
		return http.StatusForbidden, err.Error()
	case strings.Contains(err.Error(), "missing the required"):
		return http.StatusBadRequest, err.Error()
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
