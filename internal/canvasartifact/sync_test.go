package canvasartifact

import (
	"context"
	"encoding/json"
	"io"
	"sync"
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/runtimestream"
	"github.com/usehivy/hivy/internal/storage"
	"github.com/usehivy/hivy/internal/testdb"
)

type fakeFileStore struct {
	objects map[string][]byte
}

func (s *fakeFileStore) Stream(ctx context.Context, key, contentType string, body io.Reader) (*storage.StoredAsset, error) {
	data, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}
	s.objects[key] = data
	return &storage.StoredAsset{Key: key, Bytes: int64(len(data))}, nil
}

func (s *fakeFileStore) Delete(ctx context.Context, key string) error {
	delete(s.objects, key)
	return nil
}

func (s *fakeFileStore) PresignGet(ctx context.Context, key string) (string, error) {
	return "https://storage.test/" + key, nil
}

type recordingPublisher struct {
	mu      sync.Mutex
	notices []ArtifactSyncedNotice
}

func (p *recordingPublisher) PublishArtifactSynced(ctx context.Context, notice ArtifactSyncedNotice) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.notices = append(p.notices, notice)
}

func (p *recordingPublisher) last() ArtifactSyncedNotice {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.notices[len(p.notices)-1]
}

func (p *recordingPublisher) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.notices)
}

type canvasFixture struct {
	db      *gorm.DB
	svc     *Service
	org     model.Org
	agentID uuid.UUID
	session model.Session
}

func connectCanvasTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(postgres.Open(testdb.DatabaseURL()), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect test db: %v", err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(3)
	sqlDB.SetMaxIdleConns(1)
	testdb.ApplyMigrations(t, db)
	t.Cleanup(func() { sqlDB.Close() })
	return db
}

func seedCanvasFixture(t *testing.T, db *gorm.DB) *canvasFixture {
	t.Helper()
	f := &canvasFixture{db: db, svc: NewService(db, &fakeFileStore{objects: map[string][]byte{}})}
	f.org = model.Org{ID: uuid.New(), Name: "canvas-" + uuid.NewString(), Active: true, RateLimit: 1000}
	team := model.Team{ID: uuid.New(), OrgID: f.org.ID, Name: "canvas-team-" + uuid.NewString()[:8]}
	agent := model.Agent{ID: uuid.New(), OrgID: &f.org.ID, TeamID: team.ID, Name: "Canvas Agent " + uuid.NewString(), Model: "test", Status: "active"}
	channel := model.Channel{ID: uuid.New(), OrgID: f.org.ID, TeamID: team.ID, Name: "canvas-ch-" + uuid.NewString(), DefaultAgentID: agent.ID}
	session := model.Session{ID: uuid.New(), OrgID: f.org.ID, ChannelID: channel.ID, AgentID: agent.ID, Status: "active"}
	for _, seed := range []any{&f.org, &team, &agent, &channel, &session} {
		if err := db.Create(seed).Error; err != nil {
			t.Fatalf("seed fixture record: %v", err)
		}
	}
	t.Cleanup(func() {
		db.Delete(&model.Org{}, "id = ?", f.org.ID)
	})
	f.agentID = agent.ID
	f.session = session
	return f
}

func syncRequest(sessionID *uuid.UUID) SyncRequest {
	return SyncRequest{
		SessionID: sessionID,
		Project:   SyncProjectInput{Slug: "marketing", Name: "Marketing"},
		Artifact: SyncArtifactInput{
			Slug:     "landing-page",
			Type:     "web_page",
			Name:     "Landing page",
			Manifest: model.RawJSON(`{}`),
		},
		Files: []SyncFileInput{
			{Path: "index.html", Role: "entrypoint", Content: "<!doctype html><html></html>"},
		},
	}
}

func TestSyncArtifactForAgent_PublishesOnSessionfulSync(t *testing.T) {
	db := connectCanvasTestDB(t)
	f := seedCanvasFixture(t, db)
	pub := &recordingPublisher{}
	f.svc.WithPublisher(pub)
	ctx := context.Background()

	resp, err := f.svc.SyncArtifactForAgent(ctx, f.agentID, syncRequest(&f.session.ID))
	if err != nil {
		t.Fatalf("sync (create): %v", err)
	}
	if pub.count() != 1 {
		t.Fatalf("expected 1 notice after create, got %d", pub.count())
	}
	notice := pub.last()
	if notice.OrgID != f.org.ID {
		t.Fatalf("notice org=%s want %s", notice.OrgID, f.org.ID)
	}
	if notice.SessionID != f.session.ID {
		t.Fatalf("notice session=%s want %s", notice.SessionID, f.session.ID)
	}
	p := notice.Payload
	if p.ArtifactID != resp.Artifact.ID.String() {
		t.Fatalf("payload artifact_id=%s want %s", p.ArtifactID, resp.Artifact.ID)
	}
	if p.ProjectID != resp.Artifact.ProjectID.String() {
		t.Fatalf("payload project_id=%s want %s", p.ProjectID, resp.Artifact.ProjectID)
	}
	if p.Slug != "landing-page" || p.Name != "Landing page" || p.ArtifactType != "web_page" {
		t.Fatalf("payload fields wrong: %+v", p)
	}
	if !p.Created {
		t.Fatalf("expected created=true on first sync")
	}
	if _, err := json.Marshal(p); err != nil {
		t.Fatalf("payload not marshalable: %v", err)
	}

	if _, err := f.svc.SyncArtifactForAgent(ctx, f.agentID, syncRequest(&f.session.ID)); err != nil {
		t.Fatalf("sync (update): %v", err)
	}
	if pub.count() != 2 {
		t.Fatalf("expected 2 notices after update, got %d", pub.count())
	}
	updated := pub.last()
	if updated.Payload.Created {
		t.Fatalf("expected created=false on second sync")
	}
	if updated.Payload.ArtifactID != p.ArtifactID {
		t.Fatalf("update targeted a different artifact: %s vs %s", updated.Payload.ArtifactID, p.ArtifactID)
	}
}

func TestSyncArtifactForAgent_NoPublishWhenSessionNil(t *testing.T) {
	db := connectCanvasTestDB(t)
	f := seedCanvasFixture(t, db)
	pub := &recordingPublisher{}
	f.svc.WithPublisher(pub)

	if _, err := f.svc.SyncArtifactForAgent(context.Background(), f.agentID, syncRequest(nil)); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if pub.count() != 0 {
		t.Fatalf("expected no notice when session is nil, got %d", pub.count())
	}
}

func TestSyncArtifactForAgent_SucceedsWhenPublisherErrors(t *testing.T) {
	db := connectCanvasTestDB(t)
	f := seedCanvasFixture(t, db)
	// A runtime publisher over an unconfigured store: PublishNotice returns an
	// error the publisher swallows; the sync must still succeed.
	f.svc.WithPublisher(NewRuntimeNoticePublisher(runtimestream.NewStore(nil, 0)))

	resp, err := f.svc.SyncArtifactForAgent(context.Background(), f.agentID, syncRequest(&f.session.ID))
	if err != nil {
		t.Fatalf("sync must succeed despite publish failure: %v", err)
	}
	if resp.Artifact.ID == uuid.Nil {
		t.Fatalf("expected a persisted artifact")
	}
}
