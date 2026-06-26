package handler

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/oklog/ulid/v2"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/runtimeevents"
	"github.com/usehivy/hivy/internal/tasks"
)

type modelUsageInput struct {
	Operation       string
	OrgID           uuid.UUID
	ProviderID      string
	Model           string
	RequestPath     string
	TokenJTI        string
	UserID          string
	Session         *model.Session
	SandboxID       *uuid.UUID
	Credential      *model.Credential
	InputTokens     int
	OutputTokens    int
	CachedTokens    int
	ReasoningTokens int
	TotalTokens     int
	Cost            float64
	UpstreamStatus  int
	TotalMs         int
	Metadata        model.JSON
	Tags            []string
	CreatedAt       time.Time
}

func buildModelUsagePayload(input modelUsageInput) tasks.ModelUsageWritePayload {
	now := input.CreatedAt
	if now.IsZero() {
		now = time.Now().UTC()
	}
	cost := input.Cost
	if cost < 0 {
		cost = 0
	}
	totalTokens := nonNegative(input.TotalTokens)
	if totalTokens == 0 {
		totalTokens = nonNegative(input.InputTokens) + nonNegative(input.OutputTokens)
	}
	generationID := "gen_" + ulid.Make().String()
	credID := uuid.Nil
	isSystem := true
	if input.Credential != nil {
		credID = input.Credential.ID
		isSystem = input.Credential.OrgID == nil
	}
	orgID := uuid.Nil
	if input.OrgID != uuid.Nil {
		orgID = input.OrgID
	}
	if input.Session != nil {
		orgID = input.Session.OrgID
	}
	if orgID == uuid.Nil && input.Credential != nil && input.Credential.OrgID != nil {
		orgID = *input.Credential.OrgID
	}

	payload := tasks.ModelUsageWritePayload{
		Generation: model.Generation{
			ID:              generationID,
			OrgID:           orgID,
			CredentialID:    credID,
			TokenJTI:        defaultString(strings.TrimSpace(input.TokenJTI), "model_usage:"+input.Operation),
			ProviderID:      strings.TrimSpace(input.ProviderID),
			Model:           strings.TrimSpace(input.Model),
			RequestPath:     strings.TrimSpace(input.RequestPath),
			IsStreaming:     false,
			InputTokens:     nonNegative(input.InputTokens),
			OutputTokens:    nonNegative(input.OutputTokens),
			CachedTokens:    nonNegative(input.CachedTokens),
			ReasoningTokens: nonNegative(input.ReasoningTokens),
			Cost:            cost,
			TotalMs:         nonNegative(input.TotalMs),
			UpstreamStatus:  defaultStatus(input.UpstreamStatus),
			UserID:          strings.TrimSpace(input.UserID),
			Tags:            pq.StringArray(modelUsageTags(input.Operation, input.Tags)),
			CreatedAt:       now,
			IsSystem:        isSystem,
		},
	}
	if input.Session == nil {
		return payload
	}

	eventPayload := model.JSON{
		"session_id":    input.Session.ID.String(),
		"provider":      strings.TrimSpace(input.ProviderID),
		"model":         strings.TrimSpace(input.Model),
		"operation":     strings.TrimSpace(input.Operation),
		"generation_id": generationID,
		"usage": model.JSON{
			"prompt_tokens":     nonNegative(input.InputTokens),
			"completion_tokens": nonNegative(input.OutputTokens),
			"total_tokens":      totalTokens,
			"cached_tokens":     nonNegative(input.CachedTokens),
			"reasoning_tokens":  nonNegative(input.ReasoningTokens),
			"cost":              cost,
		},
	}
	for key, value := range input.Metadata {
		if strings.TrimSpace(key) != "" {
			eventPayload[key] = value
		}
	}

	sandboxID := input.SandboxID
	if sandboxID == nil {
		sandboxID = input.Session.SandboxID
	}
	payload.SessionEvent = &model.SessionEvent{
		OrgID:            input.Session.OrgID,
		SessionID:        input.Session.ID,
		AgentID:          input.Session.AgentID,
		SandboxID:        sandboxID,
		RuntimeSessionID: input.Session.ID.String(),
		EventID:          "model_usage:" + generationID,
		EventType:        runtimeevents.EventModelUsage,
		ActorUserID:      modelUsageActorUserID(input.UserID),
		Source:           defaultString(strings.TrimSpace(input.Session.Source), "system"),
		Durability:       "durable",
		Payload:          eventPayload,
		EventAt:          now,
	}
	return payload
}

func dispatchModelUsage(ctx context.Context, db *gorm.DB, enq enqueue.TaskEnqueuer, payload tasks.ModelUsageWritePayload) {
	if enq != nil {
		if err := tasks.EnqueueModelUsageWrite(ctx, enq, payload); err != nil {
			logging.Capture(ctx, fmt.Errorf("enqueue model usage write: %w", err))
		}
		return
	}
	if db != nil {
		if err := tasks.WriteModelUsage(ctx, db, payload); err != nil {
			logging.Capture(ctx, fmt.Errorf("write model usage: %w", err))
		}
	}
}

func modelUsageActorUserID(raw string) *uuid.UUID {
	id, err := uuid.Parse(strings.TrimSpace(raw))
	if err != nil || id == uuid.Nil {
		return nil
	}
	return &id
}

func modelUsageTags(operation string, extra []string) []string {
	tags := []string{"model_usage"}
	if op := strings.TrimSpace(operation); op != "" {
		tags = append(tags, op)
	}
	for _, tag := range extra {
		if value := strings.TrimSpace(tag); value != "" {
			tags = append(tags, value)
		}
	}
	return tags
}

func defaultStatus(status int) int {
	if status == 0 {
		return http.StatusOK
	}
	return status
}

func nonNegative(value int) int {
	if value < 0 {
		return 0
	}
	return value
}
