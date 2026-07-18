package handler

import (
	"context"
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
)

// requireAgentBindingManage verifies that the caller can manage the target
// agent's team before configuring one of its automations.
func requireAgentBindingManage(r *http.Request, db *gorm.DB, orgID, agentID uuid.UUID) (int, string, error) {
	if isAPIKeyRequest(r.Context()) {
		return http.StatusOK, "", nil
	}
	actor, err := access.Resolve(r.Context(), db, orgID, middleware.UserID(r.Context()))
	if err != nil {
		return http.StatusForbidden, "forbidden", err
	}
	if actor == nil {
		return http.StatusForbidden, "you must manage this agent's team to configure automations", fmt.Errorf("no actor")
	}
	if actor.IsOrgManager() {
		return http.StatusOK, "", nil
	}
	var agent model.Agent
	err = db.WithContext(r.Context()).
		Where("id = ? AND org_id = ? AND status <> ?", agentID, orgID, "archived").
		First(&agent).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return http.StatusNotFound, "agent not found", err
	}
	if err != nil {
		return http.StatusInternalServerError, "failed to load agent", err
	}
	ok, err := actor.CanManageTeamResource(r.Context(), db, agent.TeamID)
	if err != nil {
		return http.StatusInternalServerError, "failed to check team membership", err
	}
	if !ok {
		return http.StatusForbidden, "you must manage this agent's team to configure automations", fmt.Errorf("not a team member")
	}
	return http.StatusOK, "", nil
}

// createHTTP creates an inbound HTTP ("webhook") trigger. Unlike provider
// triggers it has no connection/resource — just an agent, instructions, and
// an optional shared secret.
func (h *TriggerHandler) createHTTP(r *http.Request, orgID uuid.UUID, req createTriggerRequest) (model.AgentTrigger, string, int, string, error) {
	return h.createInboundTrigger(r, orgID, req, "http")
}

// createEmail creates an email-received automation using the shared
// agent_triggers table. Resend routing is global; this record only controls
// whether received mail is injected into an agent session.
func (h *TriggerHandler) createEmail(r *http.Request, orgID uuid.UUID, req createTriggerRequest) (model.AgentTrigger, string, int, string, error) {
	return h.createInboundTrigger(r, orgID, req, "email")
}

func (h *TriggerHandler) createInboundTrigger(r *http.Request, orgID uuid.UUID, req createTriggerRequest, triggerType string) (model.AgentTrigger, string, int, string, error) {
	if strings.TrimSpace(req.Instructions) == "" {
		return model.AgentTrigger{}, "", http.StatusBadRequest, "instructions are required", fmt.Errorf("missing instructions")
	}
	agentID, err := uuid.Parse(strings.TrimSpace(req.AgentID))
	if err != nil || agentID == uuid.Nil {
		return model.AgentTrigger{}, "", http.StatusBadRequest, "agent_id must be a uuid", fmt.Errorf("invalid agent id")
	}
	if mStatus, mMessage, mErr := requireAgentBindingManage(r, h.db, orgID, agentID); mErr != nil {
		return model.AgentTrigger{}, "", mStatus, mMessage, mErr
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
			TriggerType:  triggerType,
			Enabled:      true,
			Instructions: strings.TrimSpace(req.Instructions),
		}
		if triggerType == "email" {
			trigger.TriggerKey = "email.received"
			trigger.TriggerKeys = pq.StringArray{"email.received"}
			trigger.SourceSlug = "email/inbound"
		}
		if triggerType == "http" && (strings.TrimSpace(req.SecretKey) != "") {
			secret := strings.TrimSpace(req.SecretKey)
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

func validateTriggerAgent(ctx context.Context, db *gorm.DB, orgID, agentID uuid.UUID, template triggerTemplate) error {
	var count int64
	if err := db.Model(&model.Agent{}).
		Where("id = ? AND org_id = ? AND status <> ?", agentID, orgID, "archived").
		Count(&count).Error; err != nil {
		return fmt.Errorf("load agent: %w", err)
	}
	if count == 0 {
		return fmt.Errorf("agent not found")
	}
	return validateTriggerAgentConnection(ctx, db, orgID, agentID, template)
}

// validateTriggerAgentConnection enforces that the target agent can actually use
// the provider the trigger's playbook depends on. It reuses the same resolver
// the runtime uses to authenticate provider tooling, so a passing check means
// the agent has a concrete team-granted connection.
func validateTriggerAgentConnection(ctx context.Context, db *gorm.DB, orgID, agentID uuid.UUID, template triggerTemplate) error {
	if len(template.requiredProviders) == 0 {
		return nil
	}
	_, err := connectionaccess.ResolveAgentProviderAny(ctx, db, orgID, agentID, template.requiredProviders...)
	if err == nil {
		return nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		label := template.requiredConnectionLabel
		if label == "" {
			label = "required"
		}
		return fmt.Errorf("agent is missing the required %s connection", label)
	}
	return fmt.Errorf("check agent connection access: %w", err)
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
	if trigger.ConnectionID != nil {
		response.ConnectionID = trigger.ConnectionID.String()
	}
	return response
}
