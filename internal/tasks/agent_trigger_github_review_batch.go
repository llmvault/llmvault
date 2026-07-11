package tasks

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/model"
)

const (
	reviewBatchBufferingStatus = "buffering"
	reviewBatchKeyField        = "_review_batch_key"
	reviewBatchEventIDsField   = "_review_batch_event_ids"
)

func githubReviewBatchKey(repo, prNumber string) string {
	return "github:pull_request_review.submitted:" + strings.TrimSpace(repo) + ":" + strings.TrimSpace(prNumber)
}

func (h *AgentTriggerDispatchHandler) enqueueGitHubReviewBatch(
	ctx context.Context,
	session *model.Session,
	compiled compiledTriggerMessage,
	eventID string,
	batchKey string,
) (uuid.UUID, error) {
	var queueID uuid.UUID
	var appendedOrgID uuid.UUID
	var appendedEventID string
	var appendedEventAt time.Time
	createdEvent := false
	err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var locked model.Session
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&locked, "id = ?", session.ID).Error; err != nil {
			return fmt.Errorf("lock review batch session: %w", err)
		}
		messagePayload := model.JSON{}
		maps.Copy(messagePayload, compiled.Raw)
		messagePayload["text"] = compiled.Text
		event, eventWasCreated, err := ensureAutomatedSessionEventCreated(tx, locked, eventID, triggerConversationSource, messagePayload)
		if err != nil {
			return err
		}
		createdEvent = eventWasCreated
		appendedOrgID = locked.OrgID
		appendedEventID = event.ID.String()
		appendedEventAt = event.EventAt

		var existingDelivery model.SessionMessageQueue
		if err := tx.Where("session_id = ? AND (session_event_id = ? OR jsonb_exists(message_payload->'_review_batch_event_ids', ?))", locked.ID, event.ID, eventID).First(&existingDelivery).Error; err == nil {
			queueID = existingDelivery.ID
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("load existing review delivery: %w", err)
		}

		var batch model.SessionMessageQueue
		err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("session_id = ? AND status = ? AND message_payload->>? = ?", locked.ID, reviewBatchBufferingStatus, reviewBatchKeyField, batchKey).
			First(&batch).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("load review batch queue: %w", err)
		}
		if err == nil {
			ids := stringSlice(batch.MessagePayload[reviewBatchEventIDsField])
			for _, id := range ids {
				if id == eventID {
					queueID = batch.ID
					return nil
				}
			}
			ids = append(ids, eventID)
			batch.MessagePayload[reviewBatchEventIDsField] = ids
			batch.MessageText = appendReviewBatchText(batch.MessageText, compiled.Text)
			batch.MessagePayload["text"] = batch.MessageText
			if err := tx.Model(&model.SessionMessageQueue{}).Where("id = ?", batch.ID).Updates(map[string]any{
				"message_text":    batch.MessageText,
				"message_payload": batch.MessagePayload,
				"updated_at":      time.Now().UTC(),
			}).Error; err != nil {
				return fmt.Errorf("append review batch queue: %w", err)
			}
			queueID = batch.ID
			return nil
		}

		seq, err := nextSessionQueueSequence(tx, locked.ID)
		if err != nil {
			return err
		}
		messagePayload[reviewBatchKeyField] = batchKey
		messagePayload[reviewBatchEventIDsField] = []string{eventID}
		queue := model.SessionMessageQueue{
			OrgID:           locked.OrgID,
			SessionID:       locked.ID,
			SessionEventID:  &event.ID,
			MessageText:     compiled.Text,
			MessagePayload:  messagePayload,
			SequenceNumber:  seq,
			Status:          reviewBatchBufferingStatus,
			Model:           locked.Model,
			ReasoningEffort: locked.ReasoningEffort,
		}
		if err := tx.Create(&queue).Error; err != nil {
			return fmt.Errorf("create review batch queue: %w", err)
		}
		queueID = queue.ID
		return nil
	})
	if err != nil {
		return uuid.Nil, err
	}
	if createdEvent {
		publishSessionEventsAppended(ctx, h.sessionEventNotices, appendedOrgID, session.ID, appendedEventID, appendedEventAt)
	}
	return queueID, nil
}

func appendReviewBatchText(existing, next string) string {
	next = strings.TrimSpace(next)
	if existing == "" {
		return next
	}
	if end := strings.Index(next, "</system_message>"); end >= 0 {
		next = strings.TrimSpace(next[end+len("</system_message>"):])
	}
	return strings.TrimSpace(existing) + "\n\n---\n\nAdditional review submitted in the same 30-second batch:\n\n" + next
}
