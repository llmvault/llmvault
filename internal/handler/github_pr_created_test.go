package handler

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/model"
)

func TestParseGitHubPRURL(t *testing.T) {
	cases := []struct {
		url        string
		wantRepo   string
		wantNumber int
		wantOK     bool
	}{
		{"https://github.com/usehivy/hivy/pull/42", "usehivy/hivy", 42, true},
		{"https://github.com/usehivy/hivy/pull/42\n", "usehivy/hivy", 42, true},
		{"https://github.example.com/acme/repo-x/pull/7", "acme/repo-x", 7, true},
		{"Opening PR...\nhttps://github.com/usehivy/hivy/pull/9", "usehivy/hivy", 9, true},
		{"https://github.com/usehivy/hivy/issues/42", "", 0, false},
		{"https://github.com/usehivy/hivy/pull/0", "", 0, false},
		{"not a url", "", 0, false},
		{"", "", 0, false},
	}
	for _, tc := range cases {
		repo, number, ok := parseGitHubPRURL(tc.url)
		if ok != tc.wantOK || repo != tc.wantRepo || number != tc.wantNumber {
			t.Errorf("parseGitHubPRURL(%q) = (%q,%d,%v), want (%q,%d,%v)",
				tc.url, repo, number, ok, tc.wantRepo, tc.wantNumber, tc.wantOK)
		}
	}
}

func newPRCreatedTestKey(t *testing.T) *crypto.SymmetricKey {
	t.Helper()
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 7)
	}
	encKey, err := crypto.NewSymmetricKey(base64.StdEncoding.EncodeToString(key))
	if err != nil {
		t.Fatalf("new symmetric key: %v", err)
	}
	return encKey
}

type prCreatedFixture struct {
	db        *gorm.DB
	router    *chi.Router
	orgID     uuid.UUID
	agentID   uuid.UUID
	sessionID uuid.UUID
	secret    string
}

func seedPRCreatedFixture(t *testing.T, db *gorm.DB, encKey *crypto.SymmetricKey, sessionStatus string) prCreatedFixture {
	t.Helper()
	org := model.Org{Name: "pr-created-" + uuid.NewString()[:8], Active: true}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	agent := model.Agent{OrgID: &org.ID, TeamID: firstTeamID(t, db, org.ID), Name: "pr-agent-" + uuid.NewString()[:8], Model: "gpt-5", Status: "active"}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	channel := model.Channel{OrgID: org.ID, Name: "repo-" + uuid.NewString()[:8], TeamID: agent.TeamID, DefaultAgentID: agent.ID, Origin: "native"}
	if err := db.Create(&channel).Error; err != nil {
		t.Fatalf("create channel: %v", err)
	}
	secret := "runtime-secret-" + uuid.NewString()
	encrypted, err := encKey.EncryptString(secret)
	if err != nil {
		t.Fatalf("encrypt secret: %v", err)
	}
	sandbox := model.Sandbox{
		OrgID: &org.ID, AgentID: &agent.ID, EncryptedRuntimeSecret: encrypted,
		Status: "running", ExternalID: "ext-" + uuid.NewString()[:8], RuntimeURL: "http://runtime.test",
	}
	if err := db.Create(&sandbox).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	session := model.Session{
		OrgID: org.ID, ChannelID: channel.ID, AgentID: agent.ID, Model: agent.Model,
		SandboxID: &sandbox.ID, Source: "external", Status: sessionStatus,
		AgentTurnStatus: model.SessionAgentTurnIdle, IntegrationScopes: model.JSON{},
	}
	if err := db.Create(&session).Error; err != nil {
		t.Fatalf("create session: %v", err)
	}
	router := chi.NewRouter()
	router.Post("/internal/github-pr-created/{agentID}", NewGitHubPRCreatedHandler(db, encKey).Handle)
	var r chi.Router = router
	return prCreatedFixture{db: db, router: &r, orgID: org.ID, agentID: agent.ID, sessionID: session.ID, secret: secret}
}

func postPRCreated(fx prCreatedFixture, agentID, bearer, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/internal/github-pr-created/"+agentID, bytes.NewReader([]byte(body)))
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	rr := httptest.NewRecorder()
	(*fx.router).ServeHTTP(rr, req)
	return rr
}

func TestGitHubPRCreatedLinksSession(t *testing.T) {
	db := connectNangoSlackTestDB(t)
	encKey := newPRCreatedTestKey(t)
	fx := seedPRCreatedFixture(t, db, encKey, "active")

	rr := postPRCreated(fx, fx.agentID.String(), fx.secret, `{"pr_url":"https://github.com/usehivy/hivy/pull/42","head_ref":"hivy/fix"}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var row model.GitHubPullRequestSession
	if err := db.Where("org_id = ? AND repo = ? AND pr_number = ?", fx.orgID, "usehivy/hivy", 42).First(&row).Error; err != nil {
		t.Fatalf("load mapping: %v", err)
	}
	if row.SessionID != fx.sessionID || row.HeadRef != "hivy/fix" {
		t.Fatalf("mapping=%+v want session=%s", row, fx.sessionID)
	}

	// Upsert: a second report for the same PR updates rather than duplicates.
	rr = postPRCreated(fx, fx.agentID.String(), fx.secret, `{"pr_url":"https://github.com/usehivy/hivy/pull/42"}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("upsert status=%d body=%s", rr.Code, rr.Body.String())
	}
	var count int64
	if err := db.Model(&model.GitHubPullRequestSession{}).Where("org_id = ? AND repo = ? AND pr_number = ?", fx.orgID, "usehivy/hivy", 42).Count(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("mapping rows=%d, want 1", count)
	}
}

func TestGitHubPRCreatedRejectsBadSecret(t *testing.T) {
	db := connectNangoSlackTestDB(t)
	encKey := newPRCreatedTestKey(t)
	fx := seedPRCreatedFixture(t, db, encKey, "active")

	rr := postPRCreated(fx, fx.agentID.String(), "wrong-secret", `{"pr_url":"https://github.com/usehivy/hivy/pull/42"}`)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var count int64
	db.Model(&model.GitHubPullRequestSession{}).Where("org_id = ?", fx.orgID).Count(&count)
	if count != 0 {
		t.Fatalf("mapping rows=%d, want 0", count)
	}
}

func TestGitHubPRCreatedRejectsBadURL(t *testing.T) {
	db := connectNangoSlackTestDB(t)
	encKey := newPRCreatedTestKey(t)
	fx := seedPRCreatedFixture(t, db, encKey, "active")

	rr := postPRCreated(fx, fx.agentID.String(), fx.secret, `{"pr_url":"https://github.com/usehivy/hivy/issues/42"}`)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestGitHubPRCreatedNoActiveSession(t *testing.T) {
	db := connectNangoSlackTestDB(t)
	encKey := newPRCreatedTestKey(t)
	fx := seedPRCreatedFixture(t, db, encKey, "closed") // sandbox has no active session

	rr := postPRCreated(fx, fx.agentID.String(), fx.secret, `{"pr_url":"https://github.com/usehivy/hivy/pull/42"}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var body map[string]string
	json.Unmarshal(rr.Body.Bytes(), &body)
	if body["status"] != "no_active_session" {
		t.Fatalf("status=%q, want no_active_session", body["status"])
	}
	var count int64
	db.Model(&model.GitHubPullRequestSession{}).Where("org_id = ?", fx.orgID).Count(&count)
	if count != 0 {
		t.Fatalf("mapping rows=%d, want 0", count)
	}
}
