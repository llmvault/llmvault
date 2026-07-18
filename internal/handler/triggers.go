package handler

import (
	"encoding/json"
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
	db             *gorm.DB
	webhookBaseURL string
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
	TriggerType string `json:"trigger_type"`
	// Name is a required human label for the trigger.
	Name                 string `json:"name"`
	Provider             string `json:"provider"`
	ConnectionID         string `json:"connection_id"`
	ExternalResourceKey  string `json:"external_resource_key"`
	ExternalResourceName string `json:"external_resource_name"`
	AgentID              string `json:"agent_id"`
	TriggerKey           string `json:"trigger_key"`
	TriggerValue         string `json:"trigger_value"`
	Instructions         string `json:"instructions"`
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
	if strings.EqualFold(strings.TrimSpace(req.TriggerType), "email") {
		return h.createEmail(r, orgID, req)
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
	if status, message, err := requireAgentBindingManage(r, h.db, orgID, agentID); err != nil {
		return model.AgentTrigger{}, provider, status, message, err
	}

	var trigger model.AgentTrigger
	ctx := r.Context()
	err = h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := validateTriggerAgent(ctx, tx, orgID, agentID, template); err != nil {
			return err
		}
		trigger = model.AgentTrigger{
			OrgID:        orgID,
			AgentID:      agentID,
			Name:         strings.TrimSpace(req.Name),
			TriggerType:  "webhook",
			ConnectionID: &conn.ID,
			ResourceType: template.resourceType,
			ResourceKey:  resourceKey,
			ResourceName: resourceName,
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
