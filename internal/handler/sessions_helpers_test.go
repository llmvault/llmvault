package handler_test

import (
	"encoding/base64"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

type sessionHarness struct {
	db       *gorm.DB
	router   *chi.Mux
	enqueuer *enqueue.MockClient
}

type sessionFixture struct {
	org     model.Org
	owner   model.User
	member  model.User
	viewer  model.User
	agent   model.Agent
	channel model.Channel
}

type sessionMutationOut struct {
	Session sessionOut       `json:"session"`
	Event   *sessionEventOut `json:"event,omitempty"`
	Queued  bool             `json:"queued"`
}

type sessionOut struct {
	ID               string  `json:"id"`
	ChannelID        string  `json:"channel_id"`
	AgentID          string  `json:"agent_id"`
	SandboxID        *string `json:"sandbox_id,omitempty"`
	Status           string  `json:"status"`
	ParticipantCount int64   `json:"participant_count"`
	EventCount       int64   `json:"event_count"`
}

type sessionEventOut struct {
	ID             string         `json:"id"`
	EventType      string         `json:"event_type"`
	SequenceNumber int64          `json:"sequence_number"`
	Payload        map[string]any `json:"payload"`
}

func newSessionHarness(t *testing.T) *sessionHarness {
	return newSessionHarnessWith(t, nil)
}

func newSessionHarnessWith(t *testing.T, configure func(*handler.SessionHandler)) *sessionHarness {
	t.Helper()
	db := connectTestDB(t)
	enq := &enqueue.MockClient{}
	h := handler.NewSessionHandler(db, enq).WithRuntimeStreamKey(sessionTestEncKey(t))
	if configure != nil {
		configure(h)
	}
	r := chi.NewRouter()
	r.Route("/v1", func(r chi.Router) {
		r.Use(middleware.ResolveOrgFromHeader(db))
		r.Use(middleware.RequireAPIKeyScopeOrJWT("sessions"))
		r.Get("/channels/{id}/sessions", h.ListChannelSessions)
		r.Get("/sessions", h.List)
		r.Post("/sessions", h.Create)
		r.Get("/sessions/{id}", h.Get)
		r.Patch("/sessions/{id}", h.Update)
		r.Post("/sessions/{id}/messages", h.SendMessage)
		r.Post("/sessions/{id}/transcriptions", h.TranscribeAudio)
		r.Post("/sessions/{id}/interrupt", h.Interrupt)
		r.Get("/sessions/{id}/events", h.ListEvents)
		r.Post("/sessions/{id}/sandbox/wake", h.WakeSandbox)
		r.Post("/sessions/{id}/sandbox-access", h.SandboxAccess)
		r.Put("/sessions/{id}/participants/{userID}", h.PutParticipant)
		r.Delete("/sessions/{id}/participants/{userID}", h.DeleteParticipant)
	})
	return &sessionHarness{db: db, router: r, enqueuer: enq}
}

func (h *sessionHarness) seed(t *testing.T) sessionFixture {
	t.Helper()
	org := model.Org{Name: "session-org-" + uuid.NewString()[:8], Active: true}
	if err := h.db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	seedSessionModelCredential(t, h.db, org.ID)
	owner := seedSessionUser(t, h.db, org.ID, "owner")
	member := seedSessionUser(t, h.db, org.ID, "member")
	viewer := seedSessionUser(t, h.db, org.ID, "viewer")
	agent := seedSessionAgent(t, h.db, org.ID)
	channel := model.Channel{
		OrgID:          org.ID,
		Name:           "engineering-" + uuid.NewString()[:8],
		Kind:           "standard",
		Visibility:     "public",
		DefaultAgentID: agent.ID,
		Origin:         "native",
		CreatedBy:      &owner.ID,
	}
	if err := h.db.Create(&channel).Error; err != nil {
		t.Fatalf("create channel: %v", err)
	}
	if err := h.db.Create(&model.ChannelMember{ChannelID: channel.ID, UserID: owner.ID, Role: "owner"}).Error; err != nil {
		t.Fatalf("create channel owner: %v", err)
	}
	t.Cleanup(func() { cleanupSessionFixture(h.db, org.ID, owner.ID, member.ID, viewer.ID) })
	return sessionFixture{org: org, owner: owner, member: member, viewer: viewer, agent: agent, channel: channel}
}

func seedSessionUser(t *testing.T, db *gorm.DB, orgID uuid.UUID, role string) model.User {
	t.Helper()
	user := model.User{Email: "session-" + role + "-" + uuid.NewString()[:8] + "@test.com", Name: role}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := db.Create(&model.OrgMembership{UserID: user.ID, OrgID: orgID, Role: role}).Error; err != nil {
		t.Fatalf("create org member: %v", err)
	}
	return user
}

func seedSessionAgent(t *testing.T, db *gorm.DB, orgID uuid.UUID) model.Agent {
	t.Helper()
	agent := model.Agent{
		OrgID:           &orgID,
		Name:            "Agent-" + uuid.NewString()[:8],
		Description:     ptrString("session test agent"),
		SandboxStrategy: "always_on",
		Model:           "deepseek-v4-flash",
		Tools:           model.JSON{},
		McpServers:      model.RawJSON("[]"),
		Skills:          model.JSON{},
		RuntimeConfig:   model.JSON{},
		Permissions:     model.JSON{},
		Resources:       model.JSON{},
		Status:          "active",
	}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	return agent
}

func seedSessionModelCredential(t *testing.T, db *gorm.DB, orgID uuid.UUID) {
	t.Helper()
	cred := model.Credential{
		OrgID:        &orgID,
		Label:        "session-model-openrouter-" + uuid.NewString()[:8],
		BaseURL:      "https://openrouter.example.test/api/v1",
		AuthScheme:   "bearer",
		EncryptedKey: []byte("enc"),
		WrappedDEK:   []byte("dek"),
		ProviderID:   "openrouter",
	}
	if err := db.Create(&cred).Error; err != nil {
		t.Fatalf("create session model credential: %v", err)
	}
}

func sessionTestEncKey(t *testing.T) *crypto.SymmetricKey {
	t.Helper()
	key, err := crypto.NewSymmetricKey(base64.StdEncoding.EncodeToString(make([]byte, 32)))
	if err != nil {
		t.Fatalf("create session test enc key: %v", err)
	}
	return key
}
