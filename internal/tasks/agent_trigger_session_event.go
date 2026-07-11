package tasks

import (
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/runtimeevents"
)

// ensureAutomatedSessionEvent idempotently records an automated (trigger,
// schedule, PR-routed) message on the session transcript so it is visible in
// the session UI and available to the session-naming task. The backend row is
// the source of truth: the runtime-stream projector drops the runtime's echo
// of user.message.received, so no duplicate can appear. On conflict with
// idx_session_events_idem the existing row is returned.
func ensureAutomatedSessionEvent(tx *gorm.DB, session model.Session, eventID, source string, payload model.JSON) (model.SessionEvent, error) {
	event, _, err := ensureAutomatedSessionEventCreated(tx, session, eventID, source, payload)
	return event, err
}

func ensureAutomatedSessionEventCreated(tx *gorm.DB, session model.Session, eventID, source string, payload model.JSON) (model.SessionEvent, bool, error) {
	event := model.SessionEvent{
		OrgID:            session.OrgID,
		SessionID:        session.ID,
		AgentID:          session.AgentID,
		SandboxID:        session.SandboxID,
		RuntimeSessionID: session.ID.String(),
		EventID:          eventID,
		EventType:        runtimeevents.EventUserMessageReceived,
		Source:           source,
		SequenceNumber:   0,
		Durability:       "durable",
		Payload:          payload,
		EventAt:          time.Now().UTC(),
	}
	result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&event)
	if result.Error != nil {
		return model.SessionEvent{}, false, fmt.Errorf("create automated session event: %w", result.Error)
	}
	if result.RowsAffected == 1 {
		return event, true, nil
	}
	if err := tx.Where("session_id = ? AND event_id = ?", session.ID, eventID).First(&event).Error; err != nil {
		return model.SessionEvent{}, false, fmt.Errorf("load automated session event: %w", err)
	}
	return event, false, nil
}
