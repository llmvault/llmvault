package agentschedule

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/channelagents"
	"github.com/usehivy/hivy/internal/model"
)

// ResolveScheduleChannel resolves a raw channel_id argument to a Hivy channel
// UUID string. An empty value defaults to the agent's team #general (the
// team-scoped, Hivy-anchored default channel created by provisionTeamDefaults);
// a non-empty value must be a UUID of a non-archived channel in the org that the
// agent is allowed to use. No code path here ever creates a channel — channel
// provisioning is owned by team provisioning. Exported so other agent-facing
// tools (e.g. the HTTP trigger tool) share the exact same channel semantics as
// cron schedules.
func ResolveScheduleChannel(ctx context.Context, db *gorm.DB, orgID, agentID uuid.UUID, raw string) (string, error) {
	if strings.TrimSpace(raw) != "" {
		return validateScheduleChannel(ctx, db, orgID, agentID, raw)
	}
	return resolveTeamDefaultChannel(ctx, db, orgID, agentID)
}

// resolveTeamDefaultChannel returns the UUID of the agent's team #general (the
// default, Hivy-anchored channel). A schedule is always channel-bound and
// team-scoped; there is no system channel fallback. A team-less agent (only a
// legacy/org-level agent would be) has no default channel and must supply an
// explicit channel_id.
func resolveTeamDefaultChannel(ctx context.Context, db *gorm.DB, orgID, agentID uuid.UUID) (string, error) {
	var agent model.Agent
	err := db.WithContext(ctx).
		Select("id", "team_id").
		Where("id = ? AND org_id = ?", agentID, orgID).
		First(&agent).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", fmt.Errorf("agent not found")
	}
	if err != nil {
		return "", fmt.Errorf("load agent: %w", err)
	}
	if agent.TeamID == nil {
		return "", fmt.Errorf("agent has no team; a schedule requires an explicit channel")
	}
	var channel model.Channel
	err = db.WithContext(ctx).
		Select("id").
		Where("org_id = ? AND team_id = ? AND is_default = ? AND archived_at IS NULL", orgID, *agent.TeamID, true).
		First(&channel).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", fmt.Errorf("agent's team has no default channel; a schedule requires an explicit channel")
	}
	if err != nil {
		return "", fmt.Errorf("load team default channel: %w", err)
	}
	return channel.ID.String(), nil
}

func validateScheduleChannel(ctx context.Context, db *gorm.DB, orgID, agentID uuid.UUID, raw string) (string, error) {
	parsed, err := uuid.Parse(strings.TrimSpace(raw))
	if err != nil || parsed == uuid.Nil {
		return "", fmt.Errorf("channel_id must be a uuid")
	}
	var channel model.Channel
	err = db.WithContext(ctx).
		Where("id = ? AND org_id = ? AND archived_at IS NULL", parsed, orgID).
		First(&channel).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", fmt.Errorf("channel_id not found")
	}
	if err != nil {
		return "", fmt.Errorf("load channel: %w", err)
	}
	allowed, err := channelagents.ActsInChannel(ctx, db, orgID, channel.ID, agentID)
	if err != nil {
		return "", err
	}
	if !allowed {
		return "", fmt.Errorf("agent is not available in this channel")
	}
	return channel.ID.String(), nil
}
