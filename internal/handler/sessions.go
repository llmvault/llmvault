package handler

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/precontext"
	"github.com/usehivy/hivy/internal/registry"
	"github.com/usehivy/hivy/internal/runtimestream"
	"github.com/usehivy/hivy/internal/sandbox"
	"github.com/usehivy/hivy/internal/storage"
	"github.com/usehivy/hivy/internal/transcription"
)

type SessionHandler struct {
	db                       *gorm.DB
	enqueuer                 enqueue.TaskEnqueuer
	runtimeEncKey            *crypto.SymmetricKey
	runtimeStreamStore       *runtimestream.Store
	orchestrator             *sandbox.Orchestrator
	compileDeps              agentruntime.CompileDeps
	preContext               precontext.Builder
	transcriptionKMS         *crypto.KeyWrapper
	transcriptionReader      storage.Reader
	transcriptionTranscriber transcription.Transcriber
	transcriptionRegistry    *registry.Registry
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

func (h *SessionHandler) WithRuntimeStreamStore(store *runtimestream.Store) *SessionHandler {
	h.runtimeStreamStore = store
	return h
}

func (h *SessionHandler) WithRuntimeDelivery(orchestrator *sandbox.Orchestrator, compileDeps agentruntime.CompileDeps) *SessionHandler {
	h.orchestrator = orchestrator
	h.compileDeps = compileDeps
	return h
}

func (h *SessionHandler) WithPreContextBuilder(builder precontext.Builder) *SessionHandler {
	h.preContext = builder
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

type createSessionRequest struct {
	ChannelID       string                         `json:"channel_id"`
	AgentID         string                         `json:"agent_id,omitempty"`
	Text            string                         `json:"text,omitempty"`
	ModelDefinition *sessionModelDefinitionRequest `json:"model_definition,omitempty"`
}

type sessionModelDefinitionRequest struct {
	ModelID         string `json:"model_id"`
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
}

type sendSessionMessageRequest struct {
	Text             string       `json:"text"`
	AttachmentIDs    []string     `json:"attachment_ids,omitempty"`
	CodeLineComments []model.JSON `json:"code_line_comments,omitempty"`
	ArtifactComments []model.JSON `json:"artifact_comments,omitempty"`
}

func sessionMessageHasContent(text string, payload model.JSON) bool {
	if strings.TrimSpace(text) != "" {
		return true
	}
	return payloadArrayLen(payload, "attachments") > 0 ||
		payloadArrayLen(payload, "attachment_ids") > 0 ||
		payloadArrayLen(payload, "code_line_comments") > 0 ||
		payloadArrayLen(payload, "artifact_comments") > 0
}

func payloadArrayLen(payload model.JSON, key string) int {
	if payload == nil {
		return 0
	}
	items, ok := payload[key].([]any)
	if !ok {
		return 0
	}
	return len(items)
}

type updateSessionRequest struct {
	Name             *string `json:"name,omitempty"`
	ChannelID        *string `json:"channel_id,omitempty"`
	AgentID          *string `json:"agent_id,omitempty"`
	ImageModel       *string `json:"image_model,omitempty"`
	VectorImageModel *string `json:"vector_image_model,omitempty"`
	Status           *string `json:"status,omitempty"`
}

type sessionMutationResponse struct {
	Session sessionResponse       `json:"session"`
	Event   *sessionEventResponse `json:"event,omitempty"`
	Queued  bool                  `json:"queued,omitempty"`
}

type sessionInputResponseRequest struct {
	RequestID         string                                `json:"request_id"`
	QuestionRequestID string                                `json:"question_request_id,omitempty"`
	Text              string                                `json:"text,omitempty"`
	OptionID          string                                `json:"option_id,omitempty"`
	Answers           map[string]sessionInputResponseAnswer `json:"answers,omitempty"`
}

type sessionInputResponseAnswer struct {
	Answers []string `json:"answers"`
	Other   *string  `json:"other,omitempty"`
}

type sessionDetailResponse struct {
	Session      sessionResponse              `json:"session"`
	Participants []sessionParticipantResponse `json:"participants"`
}

type sessionResponse struct {
	ID                 string  `json:"id"`
	ChannelID          string  `json:"channel_id"`
	AgentID            string  `json:"agent_id"`
	SandboxID          *string `json:"sandbox_id,omitempty"`
	CreatedBy          *string `json:"created_by,omitempty"`
	Model              string  `json:"model"`
	ImageModel         string  `json:"image_model"`
	VectorImageModel   string  `json:"vector_image_model"`
	ReasoningEffort    string  `json:"reasoning_effort"`
	Source             string  `json:"source"`
	SourceResourceKey  string  `json:"source_resource_key"`
	Name               string  `json:"name"`
	Status             string  `json:"status"`
	AgentTurnStatus    string  `json:"agent_turn_status"`
	AgentTurnID        string  `json:"agent_turn_id,omitempty"`
	AgentStreamID      string  `json:"agent_stream_id,omitempty"`
	AgentTurnStartedAt *string `json:"agent_turn_started_at,omitempty"`
	LastTurnOutcome    string  `json:"last_turn_outcome,omitempty"`
	ParticipantCount   int64   `json:"participant_count"`
	EventCount         int64   `json:"event_count"`
	LastActivityAt     string  `json:"last_activity_at"`
	CreatedAt          string  `json:"created_at"`
	UpdatedAt          string  `json:"updated_at"`
	EndedAt            *string `json:"ended_at,omitempty"`
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
	RuntimeSeq     *int64     `json:"runtime_seq,omitempty"`
	RuntimeEventID string     `json:"runtime_event_id,omitempty"`
	TurnID         string     `json:"turn_id,omitempty"`
	SpanID         string     `json:"span_id,omitempty"`
	Durability     string     `json:"durability,omitempty"`
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
