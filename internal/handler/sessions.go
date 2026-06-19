package handler

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/registry"
	"github.com/usehivy/hivy/internal/sandbox"
	"github.com/usehivy/hivy/internal/storage"
	"github.com/usehivy/hivy/internal/tasks"
	"github.com/usehivy/hivy/internal/transcription"
)

type SessionHandler struct {
	db                       *gorm.DB
	enqueuer                 enqueue.TaskEnqueuer
	runtimeEncKey            *crypto.SymmetricKey
	orchestrator             *sandbox.Orchestrator
	compileDeps              agentruntime.CompileDeps
	transcriptionKMS         *crypto.KeyWrapper
	transcriptionReader      storage.Reader
	transcriptionTranscriber transcription.Transcriber
	transcriptionRegistry    *registry.Registry
}

func formatRuntimeTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func formatRuntimeTimePtr(t *time.Time) *string {
	if t == nil || t.IsZero() {
		return nil
	}
	formatted := formatRuntimeTime(*t)
	return &formatted
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func webSessionName(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return "New session"
	}
	const max = 80
	if len(text) <= max {
		return text
	}
	return strings.TrimSpace(text[:max])
}

func NewSessionHandler(db *gorm.DB, enqueuers ...enqueue.TaskEnqueuer) *SessionHandler {
	var enq enqueue.TaskEnqueuer
	if len(enqueuers) > 0 {
		enq = enqueuers[0]
	}
	return &SessionHandler{db: db, enqueuer: enq}
}

func (h *SessionHandler) WithRuntimeStreamKey(key *crypto.SymmetricKey) *SessionHandler {
	h.runtimeEncKey = key
	return h
}

func (h *SessionHandler) WithRuntimeDelivery(orchestrator *sandbox.Orchestrator, compileDeps agentruntime.CompileDeps) *SessionHandler {
	h.orchestrator = orchestrator
	h.compileDeps = compileDeps
	return h
}

func (h *SessionHandler) WithTranscription(kms *crypto.KeyWrapper, reader storage.Reader, transcriber transcription.Transcriber, reg *registry.Registry) *SessionHandler {
	h.transcriptionKMS = kms
	h.transcriptionReader = reader
	h.transcriptionTranscriber = transcriber
	if reg == nil {
		reg = registry.Global()
	}
	h.transcriptionRegistry = reg
	return h
}

func (h *SessionHandler) dispatchOrQueueSessionDelivery(ctx context.Context, sessionID uuid.UUID) (bool, error) {
	if h.orchestrator != nil && h.compileDeps.EncKey != nil {
		dispatcher := tasks.NewSessionMessageDeliverHandler(h.db, h.orchestrator, h.compileDeps, h.enqueuer).WithoutProvisioning()
		if _, err := dispatcher.DispatchNext(ctx, sessionID); err == nil {
			return false, nil
		} else if errors.Is(err, tasks.ErrSessionTurnActive) {
			return true, nil
		} else if !errors.Is(err, tasks.ErrSessionRuntimeNotReady) && !errors.Is(err, gorm.ErrRecordNotFound) {
			return true, h.enqueueSessionDelivery(ctx, sessionID)
		}
	}
	return true, h.enqueueSessionDelivery(ctx, sessionID)
}

type createSessionRequest struct {
	ChannelID       string                         `json:"channel_id"`
	AgentID         string                         `json:"agent_id,omitempty"`
	Text            string                         `json:"text,omitempty"`
	Message         string                         `json:"message,omitempty"`
	Name            string                         `json:"name,omitempty"`
	Model           string                         `json:"model,omitempty"`
	ModelDefinition *sessionModelDefinitionRequest `json:"model_definition,omitempty"`
	AccessMode      string                         `json:"access_mode,omitempty"`
	ReasoningEffort string                         `json:"reasoning_effort,omitempty"`
	Raw             model.JSON                     `json:"raw,omitempty"`
}

type sessionModelDefinitionRequest struct {
	ModelID         string `json:"model_id"`
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
}

type sendSessionMessageRequest struct {
	Text            string                         `json:"text"`
	Message         string                         `json:"message,omitempty"`
	User            string                         `json:"user,omitempty"`
	UserDisplayName string                         `json:"user_display_name,omitempty"`
	ModelDefinition *sessionModelDefinitionRequest `json:"model_definition,omitempty"`
	DynamicContext  model.JSON                     `json:"dynamic_context,omitempty"`
	Raw             model.JSON                     `json:"raw,omitempty"`
}

type updateSessionRequest struct {
	Name      *string `json:"name,omitempty"`
	ChannelID *string `json:"channel_id,omitempty"`
	AgentID   *string `json:"agent_id,omitempty"`
	Status    *string `json:"status,omitempty"`
}

type sessionMutationResponse struct {
	Session sessionResponse       `json:"session"`
	Event   *sessionEventResponse `json:"event,omitempty"`
	Queued  bool                  `json:"queued,omitempty"`
}

type sessionDetailResponse struct {
	Session      sessionResponse              `json:"session"`
	Participants []sessionParticipantResponse `json:"participants"`
}

type sessionResponse struct {
	ID                string  `json:"id"`
	ChannelID         string  `json:"channel_id"`
	AgentID           string  `json:"agent_id"`
	SandboxID         *string `json:"sandbox_id,omitempty"`
	CreatedBy         *string `json:"created_by,omitempty"`
	Model             string  `json:"model"`
	AccessMode        string  `json:"access_mode"`
	ReasoningEffort   string  `json:"reasoning_effort"`
	Source            string  `json:"source"`
	SourceResourceKey string  `json:"source_resource_key"`
	Name              string  `json:"name"`
	Status            string  `json:"status"`
	ParticipantCount  int64   `json:"participant_count"`
	EventCount        int64   `json:"event_count"`
	LastActivityAt    string  `json:"last_activity_at"`
	CreatedAt         string  `json:"created_at"`
	UpdatedAt         string  `json:"updated_at"`
	EndedAt           *string `json:"ended_at,omitempty"`
}

type sessionParticipantResponse struct {
	UserID    string  `json:"user_id"`
	Role      string  `json:"role"`
	InvitedBy *string `json:"invited_by,omitempty"`
	JoinedAt  *string `json:"joined_at,omitempty"`
	CreatedAt string  `json:"created_at"`
}

type sessionEventResponse struct {
	ID             string     `json:"id"`
	SessionID      string     `json:"session_id"`
	AgentID        string     `json:"agent_id"`
	SandboxID      *string    `json:"sandbox_id,omitempty"`
	EventID        string     `json:"event_id,omitempty"`
	EventType      string     `json:"event_type"`
	ActorUserID    *string    `json:"actor_user_id,omitempty"`
	Source         string     `json:"source"`
	SequenceNumber int64      `json:"sequence_number"`
	Payload        model.JSON `json:"payload"`
	EventAt        string     `json:"event_at"`
}

type sessionSandboxAccessResponse struct {
	SessionID      string   `json:"session_id"`
	SandboxID      string   `json:"sandbox_id"`
	SandboxBaseURL string   `json:"sandbox_base_url"`
	Token          string   `json:"token"`
	ExpiresAt      string   `json:"expires_at"`
	Scopes         []string `json:"scopes"`
}

func sessionIDFromRequest(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	sessionID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid session id"})
		return uuid.Nil, false
	}
	return sessionID, true
}

func currentSessionUserID(r *http.Request) (*uuid.UUID, bool) {
	raw := middleware.UserID(r.Context())
	if raw == "" {
		return nil, false
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return nil, false
	}
	return &id, true
}

func sessionToResponse(session model.Session, participantCount, eventCount int64, lastActivity *time.Time) sessionResponse {
	last := session.UpdatedAt
	if lastActivity != nil && !lastActivity.IsZero() {
		last = *lastActivity
	}
	return sessionResponse{
		ID:                session.ID.String(),
		ChannelID:         session.ChannelID.String(),
		AgentID:           session.AgentID.String(),
		SandboxID:         formatUUIDPtr(session.SandboxID),
		CreatedBy:         formatUUIDPtr(session.CreatedBy),
		Model:             session.Model,
		AccessMode:        session.AccessMode,
		ReasoningEffort:   session.ReasoningEffort,
		Source:            session.Source,
		SourceResourceKey: session.SourceResourceKey,
		Name:              session.Name,
		Status:            session.Status,
		ParticipantCount:  participantCount,
		EventCount:        eventCount,
		LastActivityAt:    formatRuntimeTime(last),
		CreatedAt:         formatRuntimeTime(session.CreatedAt),
		UpdatedAt:         formatRuntimeTime(session.UpdatedAt),
		EndedAt:           formatRuntimeTimePtr(session.EndedAt),
	}
}
