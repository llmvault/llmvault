package handler

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

// GitHubPRCreatedHandler records which session opened a pull request. The
// gh-wrapper in the sandbox reports the created PR URL here after a successful
// `gh pr create`, authenticated with the sandbox runtime secret. Because a
// sandbox belongs to exactly one active session, the reporting sandbox pins the
// session that opened the PR.
type GitHubPRCreatedHandler struct {
	db     *gorm.DB
	encKey *crypto.SymmetricKey
}

func NewGitHubPRCreatedHandler(db *gorm.DB, encKey *crypto.SymmetricKey) *GitHubPRCreatedHandler {
	return &GitHubPRCreatedHandler{db: db, encKey: encKey}
}

type githubPRCreatedRequest struct {
	PRURL   string `json:"pr_url"`
	HeadRef string `json:"head_ref"`
}

// githubPRURLPattern matches https://github.com/<owner>/<repo>/pull/<number>,
// tolerating an enterprise host and a trailing path/query.
var githubPRURLPattern = regexp.MustCompile(`https?://[^/]+/([\w.-]+)/([\w.-]+)/pull/(\d+)`)

// parseGitHubPRURL extracts "owner/repo" and the PR number from a PR URL.
func parseGitHubPRURL(raw string) (repo string, number int, ok bool) {
	m := githubPRURLPattern.FindStringSubmatch(strings.TrimSpace(raw))
	if m == nil {
		return "", 0, false
	}
	n, err := strconv.Atoi(m[3])
	if err != nil || n <= 0 {
		return "", 0, false
	}
	return m[1] + "/" + m[2], n, true
}

// Handle processes POST /internal/github-pr-created/{agentID}.
func (h *GitHubPRCreatedHandler) Handle(w http.ResponseWriter, r *http.Request) {
	agentID, err := uuid.Parse(chi.URLParam(r, "agentID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agent_id"})
		return
	}
	sandbox, ok := resolveSandboxBySecret(h.db, h.encKey, agentID, extractBearerToken(r))
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
		return
	}

	var req githubPRCreatedRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	repo, number, ok := parseGitHubPRURL(req.PRURL)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid pr_url"})
		return
	}

	// The sandbox serves exactly one active session; that is the session that
	// opened the PR. Best-effort: if no active session is found, ack without
	// recording rather than failing the agent's command.
	var session model.Session
	if err := h.db.WithContext(r.Context()).
		Where("sandbox_id = ? AND status = ?", sandbox.ID, "active").
		Order("updated_at DESC").
		First(&session).Error; err != nil {
		logging.FromContext(r.Context()).InfoContext(r.Context(), "github-pr-created: no active session for sandbox",
			"agent_id", agentID, "sandbox_id", sandbox.ID, "repo", repo, "pr_number", number)
		writeJSON(w, http.StatusOK, map[string]string{"status": "no_active_session"})
		return
	}

	// Store the repo lowercased so the routing lookup (which also lowercases the
	// webhook's repository.full_name) matches regardless of GitHub's casing.
	repo = strings.ToLower(repo)
	now := time.Now().UTC()
	row := model.GitHubPullRequestSession{
		OrgID:     session.OrgID,
		AgentID:   &session.AgentID,
		Repo:      repo,
		PRNumber:  number,
		SessionID: session.ID,
		HeadRef:   strings.TrimSpace(req.HeadRef),
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := h.db.WithContext(r.Context()).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "org_id"}, {Name: "repo"}, {Name: "pr_number"}},
			DoUpdates: clause.Assignments(map[string]any{
				"session_id": session.ID,
				"agent_id":   session.AgentID,
				"head_ref":   row.HeadRef,
				"updated_at": now,
			}),
		}).
		Create(&row).Error; err != nil {
		logging.CaptureWithFields(r.Context(), err, map[string]any{
			"agent_id": agentID.String(), "repo": repo, "pr_number": number,
		})
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to record pull request"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "linked", "session_id": session.ID.String()})
}
