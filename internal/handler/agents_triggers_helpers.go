package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/channelagents"
	"github.com/usehivy/hivy/internal/model"
)

// validateAgentTriggers checks per-type required fields on each trigger input
// and returns the first validation error formatted as "triggers[i]: ...".
// Returns "" when every trigger is well-formed (or the slice is empty).
func validateAgentTriggers(db *gorm.DB, orgID uuid.UUID, triggers []agentTriggerInput) string {
	for i, input := range triggers {
		triggerType := input.TriggerType
		if triggerType == "" {
			triggerType = "webhook"
		}
		switch triggerType {
		case "webhook":
			if input.ConnectionID == "" {
				return fmt.Sprintf("triggers[%d]: connection_id is required", i)
			}
			if _, err := uuid.Parse(input.ConnectionID); err != nil {
				return fmt.Sprintf("triggers[%d]: invalid connection_id", i)
			}
			if len(input.TriggerKeys) == 0 {
				return fmt.Sprintf("triggers[%d]: trigger_keys is required", i)
			}
			var conn model.Connection
			if err := db.Where("id = ? AND org_id = ?", input.ConnectionID, orgID).First(&conn).Error; err != nil {
				return fmt.Sprintf("triggers[%d]: connection not found", i)
			}
		case "http":
			// No required fields.
		case "email":
			// Email is delivered through the shared Resend webhook; the trigger
			// only supplies the agent, channel, and automation instructions.
		default:
			return fmt.Sprintf("triggers[%d]: invalid trigger_type %q", i, triggerType)
		}
	}
	return ""
}

func replaceAgentTriggers(tx *gorm.DB, orgID, agentID uuid.UUID, triggers []agentTriggerInput) error {
	existingSecrets := map[uuid.UUID]string{}
	var existing []model.AgentTrigger
	if err := tx.Where("agent_id = ?", agentID).Find(&existing).Error; err != nil {
		return err
	}
	for _, trigger := range existing {
		if trigger.SecretKey != "" {
			existingSecrets[trigger.ID] = trigger.SecretKey
		}
	}
	if err := deleteAgentTriggers(tx, agentID); err != nil {
		return err
	}
	return createAgentTriggersWithExistingSecrets(tx, orgID, agentID, triggers, existingSecrets)
}

// createAgentTriggers creates agent-owned AgentTrigger records inside an
// existing transaction. Connection IDs are connections IDs from the frontend.
func createAgentTriggers(tx *gorm.DB, orgID, agentID uuid.UUID, triggers []agentTriggerInput) error {
	return createAgentTriggersWithExistingSecrets(tx, orgID, agentID, triggers, nil)
}

func createAgentTriggersWithExistingSecrets(tx *gorm.DB, orgID, agentID uuid.UUID, triggers []agentTriggerInput, existingSecrets map[uuid.UUID]string) error {
	if len(triggers) == 0 {
		return nil
	}

	for _, input := range triggers {
		var existingID uuid.UUID
		if input.ID != "" {
			parsedID, parseErr := uuid.Parse(input.ID)
			if parseErr != nil {
				return fmt.Errorf("invalid trigger id %q: %w", input.ID, parseErr)
			}
			existingID = parsedID
		}

		triggerType := input.TriggerType
		if triggerType == "" {
			triggerType = "webhook"
		}

		trigger := model.AgentTrigger{
			OrgID:        orgID,
			AgentID:      agentID,
			Enabled:      true,
			TriggerType:  triggerType,
			SourceSlug:   strings.TrimSpace(input.SourceSlug),
			Instructions: input.Instructions,
		}
		if existingID != uuid.Nil {
			trigger.ID = existingID
		}
		channelID, channelErr := resolveAgentTriggerChannel(tx, orgID, agentID, input.ChannelID)
		if channelErr != nil {
			return channelErr
		}
		trigger.ChannelID = channelID

		switch triggerType {
		case "webhook":
			connectionID, parseErr := uuid.Parse(input.ConnectionID)
			if parseErr != nil {
				return fmt.Errorf("invalid connection_id %q: %w", input.ConnectionID, parseErr)
			}
			trigger.ConnectionID = &connectionID
			trigger.TriggerKeys = pq.StringArray(input.TriggerKeys)

		case "http":
			trigger.TriggerKeys = pq.StringArray(input.TriggerKeys)
			if input.SecretKey != "" {
				hash, hashErr := bcrypt.GenerateFromPassword([]byte(input.SecretKey), bcrypt.DefaultCost)
				if hashErr != nil {
					return fmt.Errorf("hash trigger secret: %w", hashErr)
				}
				trigger.SecretKey = string(hash)
			} else if existingID != uuid.Nil && existingSecrets != nil {
				trigger.SecretKey = existingSecrets[existingID]
			}

		case "email":
			trigger.TriggerKey = "email.received"
			trigger.TriggerKeys = pq.StringArray{"email.received"}
			trigger.SourceSlug = "email/inbound"

		default:
			return fmt.Errorf("invalid trigger_type %q", triggerType)
		}

		if input.Conditions != nil && len(input.Conditions.Conditions) > 0 {
			conditionsJSON, _ := json.Marshal(input.Conditions)
			trigger.Conditions = conditionsJSON
		}

		if err := tx.Create(&trigger).Error; err != nil {
			return fmt.Errorf("create agent trigger: %w", err)
		}
	}
	return nil
}

// deleteAgentTriggers removes all trigger records owned by an agent.
func deleteAgentTriggers(db *gorm.DB, agentID uuid.UUID) error {
	if err := db.Where("agent_id = ?", agentID).Delete(&model.AgentTrigger{}).Error; err != nil {
		return fmt.Errorf("delete agent triggers: %w", err)
	}
	return nil
}

// reassignAgentTriggersToDefault re-homes every trigger owned by the given
// agents onto the org's default (Hivy) agent and disables it. Archiving an agent
// otherwise orphans its triggers: both the automations list (triggerQuery) and
// the dispatcher (matchTriggers) inner-join on non-archived agents, so a trigger
// left pointing at an archived agent silently vanishes from the UI and stops
// firing while still reading enabled=true in the DB. Re-pointing to the default
// agent keeps it visible and manageable; disabling it stops it from firing until
// the user re-homes or re-enables it deliberately.
//
// When no default agent exists to receive them (e.g. it is itself archived), the
// triggers are disabled in place so they at least stop firing.
func reassignAgentTriggersToDefault(tx *gorm.DB, orgID uuid.UUID, fromAgentIDs []uuid.UUID) error {
	if len(fromAgentIDs) == 0 {
		return nil
	}

	updates := map[string]any{"enabled": false}
	var def model.Agent
	err := tx.
		Where("org_id = ? AND is_default = ? AND status <> ? AND parent_agent_id IS NULL", orgID, true, "archived").
		Order("created_at ASC").
		First(&def).Error
	switch {
	case err == nil:
		updates["agent_id"] = def.ID
	case errors.Is(err, gorm.ErrRecordNotFound):
		// No default agent to receive the triggers; disable them in place.
	default:
		return fmt.Errorf("load default agent for trigger reassignment: %w", err)
	}

	q := tx.Model(&model.AgentTrigger{}).Where("org_id = ? AND agent_id IN ?", orgID, fromAgentIDs)
	// Guard against re-homing the default agent's own triggers onto itself.
	if id, ok := updates["agent_id"].(uuid.UUID); ok {
		q = q.Where("agent_id <> ?", id)
	}
	if err := q.Updates(updates).Error; err != nil {
		return fmt.Errorf("reassign agent triggers to default: %w", err)
	}
	return nil
}

func resolveAgentTriggerChannel(db *gorm.DB, orgID, agentID uuid.UUID, raw string) (*uuid.UUID, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	channelID, err := uuid.Parse(raw)
	if err != nil || channelID == uuid.Nil {
		return nil, fmt.Errorf("channel_id must be a uuid")
	}
	var channel model.Channel
	err = db.
		Where("id = ? AND org_id = ? AND archived_at IS NULL", channelID, orgID).
		First(&channel).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("channel_id not found")
	}
	if err != nil {
		return nil, fmt.Errorf("load channel: %w", err)
	}
	acts, err := channelagents.ActsInChannel(context.Background(), db, orgID, channel.ID, agentID)
	if err != nil {
		return nil, err
	}
	if !acts {
		return nil, fmt.Errorf("agent does not belong to this channel's team")
	}
	return &channelID, nil
}
