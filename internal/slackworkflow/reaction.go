package slackworkflow

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/slackapp"
)

func ClaimReactionTrigger(ctx context.Context, db *gorm.DB, orgID, connectionID, triggerID, agentID uuid.UUID, event slackapp.ReactionAddedEvent) (ClaimResult, error) {
	if db == nil {
		return ClaimResult{}, fmt.Errorf("db is required")
	}
	if triggerID == uuid.Nil {
		return ClaimResult{}, fmt.Errorf("trigger id is required")
	}
	if agentID == uuid.Nil {
		return ClaimResult{}, fmt.Errorf("agent id is required")
	}
	if strings.TrimSpace(event.EventID) == "" {
		return ClaimResult{}, fmt.Errorf("slack reaction event id is required")
	}
	row := model.SlackThreadEvent{
		OrgID:          orgID,
		ConnectionID:   connectionID,
		AgentID:        &agentID,
		TriggerID:      &triggerID,
		SlackTeamID:    event.TeamID,
		SlackChannelID: event.ItemChannel,
		ThreadTS:       event.ItemTS,
		MessageTS:      event.ItemTS,
		MessageAt:      event.MessageAt,
		EventID:        event.EventID,
		EventType:      event.EventType,
		Direction:      model.SlackThreadEventDirectionInbound,
		SenderID:       event.UserID,
		Status:         model.SlackThreadEventStatusReceived,
		Raw:            model.JSON(event.Raw),
		ReceivedAt:     time.Now().UTC(),
	}
	result := db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:     []clause.Column{{Name: "connection_id"}, {Name: "event_id"}},
			TargetWhere: clause.Where{Exprs: []clause.Expression{gorm.Expr("event_id <> ?", "")}},
			DoNothing:   true,
		}).
		Create(&row)
	if result.Error != nil {
		return ClaimResult{}, fmt.Errorf("claim slack reaction trigger: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		var existing model.SlackThreadEvent
		err := db.WithContext(ctx).
			Where("connection_id = ? AND event_id = ?", connectionID, event.EventID).
			First(&existing).Error
		if err != nil {
			return ClaimResult{}, fmt.Errorf("load claimed slack reaction trigger: %w", err)
		}
		return ClaimResult{
			Event:     existing,
			Accepted:  existing.Status != model.SlackThreadEventStatusIgnored,
			Duplicate: true,
			Reason:    existing.IgnoreReason,
		}, nil
	}
	return ClaimResult{Event: row, Accepted: true}, nil
}
