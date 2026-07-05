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
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/access"
	"github.com/usehivy/hivy/internal/connectionaccess"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/slackapp"
)

type TriggerHandler struct {
	db                  *gorm.DB
	externalProvisioner ChannelExternalProvisioner
	webhookBaseURL      string
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

// WithTriggerWebhookBaseURL sets the public API base used to build HTTP trigger
// URLs (e.g. cfg.APIWebhookBaseURL). Falls back to a default when empty.
func WithTriggerWebhookBaseURL(base string) TriggerHandlerOption {
	return func(h *TriggerHandler) {
		h.webhookBaseURL = strings.TrimSpace(base)
	}
}

// httpTriggerWebhookURL builds the public endpoint that invokes an HTTP trigger,
// mirroring the route served at POST /incoming/triggers/{id}.
func (h *TriggerHandler) httpTriggerWebhookURL(id uuid.UUID) string {
	base := h.webhookBaseURL
	if base == "" {
		base = "https://api.usehivy.com"
	}
	return strings.TrimRight(base, "/") + "/incoming/triggers/" + id.String()
}

type createTriggerRequest struct {
	// TriggerType selects the create path: "" / "webhook" (provider triggers,
	// default) or "http" (inbound webhook triggers).
	TriggerType          string `json:"trigger_type"`
	// Name is a required human label for the trigger.
	Name                 string `json:"name"`
	Provider             string `json:"provider"`
	ConnectionID         string `json:"connection_id"`
	ExternalResourceKey  string `json:"external_resource_key"`
	ExternalResourceName string `json:"external_resource_name"`
	AgentID              string `json:"agent_id"`
	// ChannelID is the Hivy channel the trigger runs in. Required for HTTP
	// triggers; the caller must have access to it.
	ChannelID    string `json:"channel_id"`
	TriggerKey   string `json:"trigger_key"`
	TriggerValue string `json:"trigger_value"`
	Instructions string `json:"instructions"`
	// SecretKey is an optional shared secret for HTTP triggers. Stored bcrypt-
	// hashed; never returned.
	SecretKey string `json:"secret_key"`
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
	if strings.TrimSpace(req.Name) == "" {
		return model.AgentTrigger{}, "", http.StatusBadRequest, "name is required", fmt.Errorf("missing name")
	}
	if strings.EqualFold(strings.TrimSpace(req.TriggerType), "http") {
		return h.createHTTP(r, orgID, req)
	}
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
	// The channel is where the agent's session runs. When the caller picks one
	// (e.g. GitHub, where the channel is independent of the repo), use it after
	// an access check. Otherwise auto-create it from the resource (e.g. Slack,
	// where the channel IS the event source).
	channelID, status, message, err := h.resolveProviderTriggerChannel(
		r, orgID, provider, conn, template, resourceKey, resourceName, agentID, req.ChannelID,
	)
	if err != nil {
		return model.AgentTrigger{}, provider, status, message, err
	}

	var trigger model.AgentTrigger
	err = h.db.WithContext(r.Context()).Transaction(func(tx *gorm.DB) error {
		if err := validateTriggerAgent(r.Context(), tx, orgID, agentID, channelID, template); err != nil {
			return err
		}
		trigger = model.AgentTrigger{
			OrgID:        orgID,
			AgentID:      agentID,
			Name:         strings.TrimSpace(req.Name),
			TriggerType:  "webhook",
			ChannelID:    &channelID,
			ConnectionID: &conn.ID,
			TriggerKeys:  pq.StringArray(template.triggerKeys),
			TriggerKey:   triggerKey,
			TriggerValue: triggerValue,
			Enabled:      true,
			SourceSlug:   triggerSourceSlug(provider, triggerKey, resourceKey, triggerValue),
			Instructions: strings.TrimSpace(req.Instructions),
		}
		return tx.Create(&trigger).Error
	})
	if err != nil {
		status, message := triggerCreateError(err)
		return model.AgentTrigger{}, provider, status, message, err
	}
	return trigger, provider, http.StatusCreated, "", nil
}

// resolveProviderTriggerChannel returns the channel a provider trigger's session
// runs in. A caller-supplied channel_id is used after an access check; otherwise
// the channel is auto-created from the provider resource.
func (h *TriggerHandler) resolveProviderTriggerChannel(r *http.Request, orgID uuid.UUID, provider string, conn model.Connection, template triggerTemplate, resourceKey, resourceName string, agentID uuid.UUID, rawChannelID string) (uuid.UUID, int, string, error) {
	if raw := strings.TrimSpace(rawChannelID); raw != "" {
		cid, err := uuid.Parse(raw)
		if err != nil || cid == uuid.Nil {
			return uuid.Nil, http.StatusBadRequest, "channel_id must be a uuid", fmt.Errorf("invalid channel id")
		}
		actor, err := access.Resolve(r.Context(), h.db, orgID, middleware.UserID(r.Context()))
		if err != nil {
			return uuid.Nil, http.StatusForbidden, "forbidden", err
		}
		allowed, err := actor.CanUseChannelID(r.Context(), h.db, cid)
		if err != nil {
			return uuid.Nil, http.StatusInternalServerError, "failed to check channel access", err
		}
		if !allowed {
			return uuid.Nil, http.StatusForbidden, "you do not have access to this channel", fmt.Errorf("channel access denied")
		}
		return cid, http.StatusOK, "", nil
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
		return uuid.Nil, status, message, err
	}
	return channel.ID, http.StatusOK, "", nil
}

// createHTTP creates an inbound HTTP ("webhook") trigger. Unlike provider
// triggers it has no connection/resource — just an agent, a channel the caller
// can access, instructions, and an optional shared secret.
func (h *TriggerHandler) createHTTP(r *http.Request, orgID uuid.UUID, req createTriggerRequest) (model.AgentTrigger, string, int, string, error) {
	if strings.TrimSpace(req.Instructions) == "" {
		return model.AgentTrigger{}, "", http.StatusBadRequest, "instructions are required", fmt.Errorf("missing instructions")
	}
	agentID, err := uuid.Parse(strings.TrimSpace(req.AgentID))
	if err != nil || agentID == uuid.Nil {
		return model.AgentTrigger{}, "", http.StatusBadRequest, "agent_id must be a uuid", fmt.Errorf("invalid agent id")
	}
	channelID, err := uuid.Parse(strings.TrimSpace(req.ChannelID))
	if err != nil || channelID == uuid.Nil {
		return model.AgentTrigger{}, "", http.StatusBadRequest, "channel_id must be a uuid", fmt.Errorf("invalid channel id")
	}

	actor, err := access.Resolve(r.Context(), h.db, orgID, middleware.UserID(r.Context()))
	if err != nil {
		return model.AgentTrigger{}, "", http.StatusForbidden, "forbidden", err
	}
	allowed, err := actor.CanUseChannelID(r.Context(), h.db, channelID)
	if err != nil {
		return model.AgentTrigger{}, "", http.StatusInternalServerError, "failed to check channel access", err
	}
	if !allowed {
		return model.AgentTrigger{}, "", http.StatusForbidden, "you do not have access to this channel", fmt.Errorf("channel access denied")
	}

	var trigger model.AgentTrigger
	err = h.db.WithContext(r.Context()).Transaction(func(tx *gorm.DB) error {
		var agent model.Agent
		if err := tx.Where("id = ? AND org_id = ?", agentID, orgID).First(&agent).Error; err != nil {
			return err
		}
		trigger = model.AgentTrigger{
			OrgID:        orgID,
			AgentID:      agentID,
			Name:         strings.TrimSpace(req.Name),
			TriggerType:  "http",
			ChannelID:    &channelID,
			Enabled:      true,
			Instructions: strings.TrimSpace(req.Instructions),
		}
		if secret := strings.TrimSpace(req.SecretKey); secret != "" {
			hash, hashErr := bcrypt.GenerateFromPassword([]byte(secret), bcrypt.DefaultCost)
			if hashErr != nil {
				return fmt.Errorf("hash trigger secret: %w", hashErr)
			}
			trigger.SecretKey = string(hash)
		}
		return tx.Create(&trigger).Error
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return model.AgentTrigger{}, "", http.StatusNotFound, "agent not found", err
		}
		return model.AgentTrigger{}, "", http.StatusInternalServerError, "failed to create trigger", err
	}
	return trigger, "", http.StatusCreated, "", nil
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
