package handler

import (
	"time"

	"github.com/usehivy/hivy/internal/model"
)

func sessionToResponse(session model.Session, participantCount, eventCount int64, lastActivity *time.Time) sessionResponse {
	last := session.UpdatedAt
	if lastActivity != nil && !lastActivity.IsZero() {
		last = *lastActivity
	}
	return sessionResponse{
		ID:                 session.ID.String(),
		TeamID:             session.TeamID.String(),
		AgentID:            session.AgentID.String(),
		SandboxID:          formatUUIDPtr(session.SandboxID),
		CreatedBy:          formatUUIDPtr(session.CreatedBy),
		Model:              session.Model,
		ImageModel:         session.ImageModel,
		VectorImageModel:   session.VectorImageModel,
		ReasoningEffort:    session.ReasoningEffort,
		Source:             session.Source,
		SourceResourceKey:  session.SourceResourceKey,
		Name:               session.Name,
		Status:             session.Status,
		AgentTurnStatus:    session.AgentTurnStatus,
		AgentTurnID:        session.AgentTurnID,
		AgentStreamID:      session.AgentStreamID,
		AgentTurnStartedAt: formatRuntimeTimePtr(session.AgentTurnStartedAt),
		LastTurnOutcome:    session.AgentTurnLastOutcome,
		ParticipantCount:   participantCount,
		EventCount:         eventCount,
		LastActivityAt:     formatRuntimeTime(last),
		CreatedAt:          formatRuntimeTime(session.CreatedAt),
		UpdatedAt:          formatRuntimeTime(session.UpdatedAt),
		EndedAt:            formatRuntimeTimePtr(session.EndedAt),
	}
}
