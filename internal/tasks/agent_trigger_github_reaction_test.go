package tasks

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/nango"
)

// --- pure target-derivation / path unit tests (no DB) ---

func TestMentionReactionTarget(t *testing.T) {
	comment := githubMentionEvent{Repo: "acme/repo", CommentID: "492700400", IsComment: true, Number: "42"}
	if got := mentionReactionTarget(comment); got.Kind != reactionKindIssueComment || got.ID != "492700400" {
		t.Fatalf("comment target=%+v", got)
	}
	body := githubMentionEvent{Repo: "acme/repo", Number: "7"}
	if got := mentionReactionTarget(body); got.Kind != reactionKindIssue || got.ID != "7" {
		t.Fatalf("body target=%+v", got)
	}
}

func TestReactionPath(t *testing.T) {
	cases := []struct {
		target githubReactionTarget
		want   string
	}{
		{githubReactionTarget{Kind: reactionKindIssueComment, Repo: "acme/repo", ID: "5"}, "/repos/acme/repo/issues/comments/5/reactions"},
		{githubReactionTarget{Kind: reactionKindIssue, Repo: "acme/repo", ID: "9"}, "/repos/acme/repo/issues/9/reactions"},
		{githubReactionTarget{Kind: reactionKindReviewComment, Repo: "acme/repo", ID: "3"}, "/repos/acme/repo/pulls/comments/3/reactions"},
		{githubReactionTarget{Kind: "unknown", Repo: "acme/repo", ID: "1"}, ""},
	}
	for _, tc := range cases {
		if got := tc.target.reactionPath("acme", "repo"); got != tc.want {
			t.Fatalf("path(%+v)=%q want %q", tc.target, got, tc.want)
		}
	}
}

func TestReactionTargetComplete(t *testing.T) {
	if (githubReactionTarget{Kind: reactionKindIssue, Repo: "acme/repo"}).complete() {
		t.Fatal("target with empty id should be incomplete")
	}
	if (githubReactionTarget{Repo: "acme/repo", ID: "1"}).complete() {
		t.Fatal("target with empty kind should be incomplete")
	}
	if !(githubReactionTarget{Kind: reactionKindIssue, Repo: "acme/repo", ID: "1"}).complete() {
		t.Fatal("fully-populated target should be complete")
	}
}

// --- httptest-backed reaction capture ---

type reactionCapture struct {
	mu    sync.Mutex
	calls []reactionCall
}

type reactionCall struct {
	method            string
	path              string // path after the /proxy prefix
	body              string
	providerConfigKey string // Provider-Config-Key header = integration UniqueKey
}

func (c *reactionCapture) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.calls)
}

// newReactionCapture spins up an httptest server standing in for the Nango
// proxy and returns a client pointed at it plus the recorded reaction calls.
func newReactionCapture(t *testing.T) (*nango.Client, *reactionCapture) {
	t.Helper()
	rc := &reactionCapture{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/proxy")
		// GETs are the PR-route enrichment calls (write-access filter, review
		// comments, check-suite settling); answer them permissively so routing
		// proceeds, and record only the reaction POSTs.
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusOK)
			switch {
			case strings.HasSuffix(path, "/permission"):
				_, _ = w.Write([]byte(`{"permission":"write","role_name":"write"}`))
			case strings.HasSuffix(path, "/check-suites"):
				_, _ = w.Write([]byte(`{"total_count":0,"check_suites":[]}`))
			default:
				_, _ = w.Write([]byte(`[]`))
			}
			return
		}
		body, _ := io.ReadAll(r.Body)
		rc.mu.Lock()
		rc.calls = append(rc.calls, reactionCall{
			method:            r.Method,
			path:              path,
			body:              strings.TrimSpace(string(body)),
			providerConfigKey: r.Header.Get("Provider-Config-Key"),
		})
		rc.mu.Unlock()
		w.WriteHeader(http.StatusCreated) // 201 = created
		_, _ = w.Write([]byte(`{"id":1,"content":"eyes"}`))
	}))
	t.Cleanup(srv.Close)
	return nango.NewClient(srv.URL, "secret"), rc
}

// seedGitHubConnection seeds the PRIMARY GitHub App connection (bot handle
// "usehivy"). Use seedGitHubAppConnection for a specific provider/handle.
func seedGitHubConnection(t *testing.T, db *gorm.DB, orgID uuid.UUID) uuid.UUID {
	t.Helper()
	return seedGitHubAppConnection(t, db, orgID, "github-app", "usehivy")
}

// seedGitHubAppConnection seeds a GitHub App connection for the given provider
// with its bot handle populated on the integration (the column the exact
// per-app mention matcher reads).
func seedGitHubAppConnection(t *testing.T, db *gorm.DB, orgID uuid.UUID, provider, botHandle string) uuid.UUID {
	t.Helper()
	user := model.User{Email: "gh-react-" + uuid.NewString() + "@example.com"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	integration := model.Integration{
		UniqueKey:   provider + "-" + uuid.NewString(),
		Provider:    provider,
		DisplayName: "GitHub",
		BotHandle:   botHandle,
	}
	if err := db.Create(&integration).Error; err != nil {
		t.Fatalf("create integration: %v", err)
	}
	conn := model.Connection{
		OrgID: orgID, UserID: user.ID, IntegrationID: integration.ID,
		NangoConnectionID: "gh-conn-" + uuid.NewString(), Meta: model.JSON{},
	}
	if err := db.Create(&conn).Error; err != nil {
		t.Fatalf("create connection: %v", err)
	}
	return conn.ID
}

func assertEyesBody(t *testing.T, body string) {
	t.Helper()
	var decoded map[string]string
	if err := json.Unmarshal([]byte(body), &decoded); err != nil {
		t.Fatalf("decode reaction body %q: %v", body, err)
	}
	if decoded["content"] != "eyes" {
		t.Fatalf("reaction body=%q, want content=eyes", body)
	}
}

// --- Hook A: mention path posts to the right reactions endpoint ---

func TestAddGitHubEyesReactionMentionComment(t *testing.T) {
	db := connectTestDB(t)
	org, _, _ := seedTriggerSessionFixture(t, db)
	connID := seedGitHubConnection(t, db, org.ID)
	client, rc := newReactionCapture(t)
	h := &AgentTriggerDispatchHandler{db: db, nangoClient: client}

	payload := AgentTriggerDispatchPayload{
		Provider: "github-app", EventType: "issue_comment", EventAction: "created",
		OrgID: org.ID, ConnectionID: connID,
	}
	event := githubMentionEvent{Repo: "acme/repo", CommentID: "492700400", IsComment: true, Number: "42"}
	h.addGitHubEyesReaction(context.Background(), payload, mentionReactionTarget(event))

	if rc.count() != 1 {
		t.Fatalf("reaction calls=%d, want 1", rc.count())
	}
	call := rc.calls[0]
	if call.method != http.MethodPost || call.path != "/repos/acme/repo/issues/comments/492700400/reactions" {
		t.Fatalf("call=%+v", call)
	}
	assertEyesBody(t, call.body)
}

func TestAddGitHubEyesReactionIssueBody(t *testing.T) {
	db := connectTestDB(t)
	org, _, _ := seedTriggerSessionFixture(t, db)
	connID := seedGitHubConnection(t, db, org.ID)
	client, rc := newReactionCapture(t)
	h := &AgentTriggerDispatchHandler{db: db, nangoClient: client}

	payload := AgentTriggerDispatchPayload{
		Provider: "github-app", EventType: "issues", EventAction: "opened",
		OrgID: org.ID, ConnectionID: connID,
	}
	event := githubMentionEvent{Repo: "acme/repo", Number: "7"}
	h.addGitHubEyesReaction(context.Background(), payload, mentionReactionTarget(event))

	if rc.count() != 1 {
		t.Fatalf("reaction calls=%d, want 1", rc.count())
	}
	if got := rc.calls[0].path; got != "/repos/acme/repo/issues/7/reactions" {
		t.Fatalf("path=%q", got)
	}
}

// A nil nango client or an unknown connection is a silent no-op, never a panic
// or an error, so the ack can never fail or delay delivery.
func TestAddGitHubEyesReactionBestEffort(t *testing.T) {
	(&AgentTriggerDispatchHandler{}).addGitHubEyesReaction(context.Background(),
		AgentTriggerDispatchPayload{}, githubReactionTarget{Kind: reactionKindIssue, Repo: "a/b", ID: "1"})

	db := connectTestDB(t)
	org, _, _ := seedTriggerSessionFixture(t, db)
	client, rc := newReactionCapture(t)
	h := &AgentTriggerDispatchHandler{db: db, nangoClient: client}
	h.addGitHubEyesReaction(context.Background(),
		AgentTriggerDispatchPayload{OrgID: org.ID, ConnectionID: uuid.New()},
		githubReactionTarget{Kind: reactionKindIssue, Repo: "a/b", ID: "1"})
	if rc.count() != 0 {
		t.Fatalf("unknown connection produced %d calls, want 0", rc.count())
	}
}
