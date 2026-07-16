package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/access"
	"github.com/usehivy/hivy/internal/channelagents"
	"github.com/usehivy/hivy/internal/connectionaccess"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

// requireChannelBindingManage verifies the caller may bind a team resource
// (trigger/schedule) to channelID under the team-primary model. Creating such a
// binding is a manage-the-channel's-team action, not merely use-the-channel:
// API-key callers are trusted org-wide; a human actor must be an org manager or
// an active member of the channel's owning team. A channel with no team is
// denied (fail-closed). Returns (status, message, err); err is nil and status
// 200 on success.
func requireChannelBindingManage(r *http.Request, db *gorm.DB, orgID, channelID uuid.UUID) (int, string, error) {
	if isAPIKeyRequest(r.Context()) {
		return http.StatusOK, "", nil
	}
	actor, err := access.Resolve(r.Context(), db, orgID, middleware.UserID(r.Context()))
	if err != nil {
		return http.StatusForbidden, "forbidden", err
	}
	if actor == nil {
		return http.StatusForbidden, "you must manage this channel's team to bind resources to it", fmt.Errorf("no actor")
	}
	if actor.IsOrgManager() {
		return http.StatusOK, "", nil
	}
	var channel model.Channel
	err = db.WithContext(r.Context()).
		Where("id = ? AND org_id = ?", channelID, orgID).
		First(&channel).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return http.StatusForbidden, "you do not have access to this channel", fmt.Errorf("channel not found")
	}
	if err != nil {
		return http.StatusInternalServerError, "failed to load channel", err
	}
	ok, err := actor.CanManageTeamResource(r.Context(), db, channel.TeamID)
	if err != nil {
		return http.StatusInternalServerError, "failed to check team membership", err
	}
	if !ok {
		return http.StatusForbidden, "you must manage this channel's team to bind resources to it", fmt.Errorf("not a team member")
	}
	return http.StatusOK, "", nil
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
		acts, err := channelagents.ActsInChannel(r.Context(), h.db, orgID, cid, agentID)
		if err != nil {
			return uuid.Nil, http.StatusInternalServerError, "failed to check channel agents", err
		}
		if !acts {
			return uuid.Nil, http.StatusUnprocessableEntity, "agent does not belong to this channel's team", fmt.Errorf("agent not in team")
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
	// The channel was resolved/created with this agent as its default. Under the
	// team-primary model the agent is usable by virtue of team ownership (external
	// channels are unbounded), so no assignment row is needed for the trigger to
	// fire.
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
	acts, err := channelagents.ActsInChannel(r.Context(), h.db, orgID, channelID, agentID)
	if err != nil {
		return model.AgentTrigger{}, "", http.StatusInternalServerError, "failed to check channel agents", err
	}
	if !acts {
		return model.AgentTrigger{}, "", http.StatusUnprocessableEntity, "agent does not belong to this channel's team", fmt.Errorf("agent not in team")
	}
	if mStatus, mMessage, mErr := requireChannelBindingManage(r, h.db, orgID, channelID); mErr != nil {
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
	// Team-primary rule: an agent may run in a channel iff it belongs to the
	// channel's owning team. Replaces the cut agent_channels allowlist.
	allowed, err := channelagents.ActsInChannel(ctx, db, orgID, channelID, agentID)
	if err != nil {
		return err
	}
	if !allowed {
		return fmt.Errorf("agent is not available in this channel")
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
