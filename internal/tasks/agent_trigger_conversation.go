package tasks

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

func (h *AgentTriggerDispatchHandler) findOrCreateTriggerSession(ctx context.Context, agent *model.Agent, trigger model.AgentTrigger, resourceKey string) (*model.Session, error) {
	if agent.TeamID == uuid.Nil {
		return nil, fmt.Errorf("trigger agent has no team")
	}
	var session model.Session
	err := h.db.WithContext(ctx).
		Where("org_id = ? AND agent_id = ? AND team_id = ? AND source = ? AND source_resource_key = ? AND status = ?",
			*agent.OrgID, agent.ID, agent.TeamID, triggerConversationSource, resourceKey, "active").
		First(&session).Error
	if err == nil {
		return &session, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("load trigger session: %w", err)
	}

	var generation int64
	if err := h.db.WithContext(ctx).Model(&model.Session{}).
		Where("org_id = ? AND agent_id = ? AND team_id = ? AND source = ? AND source_resource_key = ?",
			*agent.OrgID, agent.ID, agent.TeamID, triggerConversationSource, resourceKey).
		Count(&generation).Error; err != nil {
		return nil, fmt.Errorf("count trigger sessions: %w", err)
	}
	session = model.Session{
		ID:                stableTriggerSessionID(trigger.ID, agent.TeamID, resourceKey, generation),
		OrgID:             *agent.OrgID,
		TeamID:            agent.TeamID,
		AgentID:           agent.ID,
		Model:             agent.Model,
		ReasoningEffort:   sessionReasoningEffort(*agent),
		Source:            triggerConversationSource,
		SourceID:          &trigger.ID,
		SourceResourceKey: resourceKey,
		Status:            "active",
		Name:              "Trigger: " + resourceKey,
		IntegrationScopes: model.JSON{},
	}
	if err := h.db.WithContext(ctx).Create(&session).Error; err != nil {
		if isSessionDuplicateKey(err) {
			var winner model.Session
			findErr := h.db.WithContext(ctx).
				Where("org_id = ? AND agent_id = ? AND team_id = ? AND source = ? AND source_resource_key = ? AND status = ?",
					*agent.OrgID, agent.ID, agent.TeamID, triggerConversationSource, resourceKey, "active").
				First(&winner).Error
			if findErr == nil {
				return &winner, nil
			}
		}
		return nil, fmt.Errorf("create trigger session: %w", err)
	}
	// Best-effort auto-naming, same as web/Slack; replaces the placeholder name.
	if h.enqueuer != nil {
		if task, opts, err := NewSessionNameTask(session.ID); err != nil {
			logging.FromContext(ctx).WarnContext(ctx, "build trigger session naming task",
				"session_id", session.ID, "error", err)
		} else if _, err := h.enqueuer.Enqueue(task, opts...); err != nil {
			logging.FromContext(ctx).WarnContext(ctx, "enqueue trigger session naming",
				"session_id", session.ID, "error", err)
		}
	}
	return &session, nil
}

// stableTriggerSessionID is deterministic so concurrent deliveries collapse to
// one session. The generation (count of prior sessions for the same agent/team key, any
// status) salts the id so an archived session's row never blocks a new one.
func stableTriggerSessionID(triggerID, teamID uuid.UUID, resourceKey string, generation int64) uuid.UUID {
	seed := "hivy:trigger-session:" + triggerID.String() + ":" + teamID.String() + ":" + resourceKey
	if generation > 0 {
		seed = fmt.Sprintf("%s:gen:%d", seed, generation)
	}
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte(seed))
}

func isSessionDuplicateKey(err error) bool {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
