package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/nango"
)

const (
	githubAPIPerPage           = 100
	githubCheckSuiteMaxPages   = 5
	githubReviewCommentMaxPage = 5
	githubReviewCommentMax     = 50
)

// loadTriggerConnection loads the delivering connection with its integration,
// exactly as fetchGitHubMentionComments does, so every GitHub API call in the
// PR-route path is made under the same GitHub App identity as the delivery. It
// is best-effort: a nil db, a missing/revoked connection, or a query error all
// return ok=false.
func (h *AgentTriggerDispatchHandler) loadTriggerConnection(ctx context.Context, payload AgentTriggerDispatchPayload) (*model.Connection, bool) {
	if h == nil || h.db == nil {
		return nil, false
	}
	var conn model.Connection
	if err := h.db.WithContext(ctx).
		Preload("Integration").
		Where("id = ? AND org_id = ? AND revoked_at IS NULL", payload.ConnectionID, payload.OrgID).
		First(&conn).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			logging.CaptureWithFields(ctx, fmt.Errorf("github pr route: load connection: %w", err), map[string]any{
				"org_id": payload.OrgID.String(),
			})
		}
		return nil, false
	}
	return &conn, true
}

func (h *AgentTriggerDispatchHandler) githubGet(ctx context.Context, conn *model.Connection, path, rawQuery string) (*nango.RawProxyResponse, error) {
	return h.nangoClient.RawProxyRequest(ctx, http.MethodGet,
		conn.Integration.UniqueKey, conn.NangoConnectionID, path, rawQuery, nil, "")
}

// --- collaborator permission (write-access author filter) ---

// githubAuthorMayRoute reports whether a GitHub user has write (or higher)
// standing on the repository, via the collaborator permission endpoint
// (docs.github.com/en/rest/collaborators/collaborators#get-repository-permissions-for-a-user).
// Bots and non-collaborators are not collaborators and the endpoint answers 404
// for them; every non-2xx fails closed. A transport error is returned so the
// dispatch retries rather than silently dropping the event.
func (h *AgentTriggerDispatchHandler) githubAuthorMayRoute(ctx context.Context, conn *model.Connection, repo, author string) (bool, error) {
	owner, name, ok := strings.Cut(repo, "/")
	if !ok {
		return false, nil
	}
	login := strings.TrimSpace(author)
	if login == "" {
		return false, nil
	}
	path := fmt.Sprintf("/repos/%s/%s/collaborators/%s/permission",
		url.PathEscape(owner), url.PathEscape(name), url.PathEscape(login))
	resp, err := h.githubGet(ctx, conn, path, "")
	if err != nil {
		return false, fmt.Errorf("github permission lookup: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if resp.StatusCode != http.StatusNotFound {
			logging.FromContext(ctx).WarnContext(ctx, "github permission lookup non-2xx, failing closed",
				"repo", repo, "author", author, "status", resp.StatusCode)
		}
		return false, nil
	}
	var parsed struct {
		Permission string `json:"permission"`
		RoleName   string `json:"role_name"`
	}
	if err := json.Unmarshal(resp.Body, &parsed); err != nil {
		return false, nil
	}
	return githubPermissionAllowsWrite(parsed.Permission, parsed.RoleName), nil
}

// githubPermissionAllowsWrite accepts admin/maintain/write. role_name carries
// the precise tier (including maintain); the legacy permission field collapses
// maintain→write and triage→read, so write/admin there also qualify.
func githubPermissionAllowsWrite(permission, roleName string) bool {
	switch strings.ToLower(strings.TrimSpace(roleName)) {
	case "admin", "maintain", "write":
		return true
	}
	switch strings.ToLower(strings.TrimSpace(permission)) {
	case "admin", "write":
		return true
	}
	return false
}

// --- check suites for a ref (settling + aggregate summary) ---

type githubCheckSuite struct {
	ID                   int64  `json:"id"`
	Status               string `json:"status"`
	Conclusion           string `json:"conclusion"`
	HeadSHA              string `json:"head_sha"`
	LatestCheckRunsCount int    `json:"latest_check_runs_count"`
}

type githubCheckSuitesResponse struct {
	TotalCount  int                `json:"total_count"`
	CheckSuites []githubCheckSuite `json:"check_suites"`
}

// checkSuitesSummary aggregates every check suite (with runs) for a head SHA so
// a green last-arriving suite cannot mask an earlier red one.
type checkSuitesSummary struct {
	Total   int
	Success int
	Failure int
	Neutral int
	Overall string
}

// checkSuiteSettleState fetches all check suites for the ref and reports whether
// they have all finished, with the aggregate summary when they have. A suite
// with no check runs is ignored: GitHub creates placeholder suites for apps that
// never run, and those sit in a non-completed status indefinitely.
func (h *AgentTriggerDispatchHandler) checkSuiteSettleState(ctx context.Context, conn *model.Connection, repo, sha string) (*checkSuitesSummary, bool, error) {
	owner, name, ok := strings.Cut(repo, "/")
	if !ok {
		return nil, true, nil
	}
	path := fmt.Sprintf("/repos/%s/%s/commits/%s/check-suites",
		url.PathEscape(owner), url.PathEscape(name), url.PathEscape(sha))
	var suites []githubCheckSuite
	for page := 1; page <= githubCheckSuiteMaxPages; page++ {
		resp, err := h.githubGet(ctx, conn, path, fmt.Sprintf("per_page=%d&page=%d", githubAPIPerPage, page))
		if err != nil {
			return nil, false, fmt.Errorf("github check suites lookup: %w", err)
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, false, fmt.Errorf("github check suites lookup status %d", resp.StatusCode)
		}
		var parsed githubCheckSuitesResponse
		if err := json.Unmarshal(resp.Body, &parsed); err != nil {
			return nil, false, fmt.Errorf("decode github check suites: %w", err)
		}
		suites = append(suites, parsed.CheckSuites...)
		if len(parsed.CheckSuites) < githubAPIPerPage {
			break
		}
	}
	if !checkSuitesSettled(suites) {
		return nil, false, nil
	}
	return summarizeCheckSuites(suites), true, nil
}

func checkSuitesSettled(suites []githubCheckSuite) bool {
	for _, s := range suites {
		if s.LatestCheckRunsCount == 0 {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(s.Status), "completed") {
			return false
		}
	}
	return true
}

func summarizeCheckSuites(suites []githubCheckSuite) *checkSuitesSummary {
	summary := &checkSuitesSummary{}
	for _, s := range suites {
		if s.LatestCheckRunsCount == 0 {
			continue
		}
		summary.Total++
		switch strings.ToLower(strings.TrimSpace(s.Conclusion)) {
		case "success":
			summary.Success++
		case "failure", "timed_out", "startup_failure", "action_required":
			summary.Failure++
		default:
			summary.Neutral++
		}
	}
	if summary.Failure > 0 {
		summary.Overall = "failure"
	} else {
		summary.Overall = "success"
	}
	return summary
}

// --- review comments for a submitted review ---

type githubReviewComment struct {
	Path     string
	Line     int
	Body     string
	DiffHunk string
}

type githubReviewCommentAPI struct {
	Path         string `json:"path"`
	Line         int    `json:"line"`
	OriginalLine int    `json:"original_line"`
	Body         string `json:"body"`
	DiffHunk     string `json:"diff_hunk"`
}

// fetchReviewComments loads the inline comments attached to a submitted review
// (docs.github.com/en/rest/pulls/reviews#list-comments-for-a-pull-request-review),
// paginating. It is best-effort: any error returns the comments gathered so far
// so the review turn still renders whatever context is available.
func (h *AgentTriggerDispatchHandler) fetchReviewComments(ctx context.Context, conn *model.Connection, repo, prNumber, reviewID string) []githubReviewComment {
	if strings.TrimSpace(reviewID) == "" {
		return nil
	}
	owner, name, ok := strings.Cut(repo, "/")
	if !ok {
		return nil
	}
	path := fmt.Sprintf("/repos/%s/%s/pulls/%s/reviews/%s/comments",
		url.PathEscape(owner), url.PathEscape(name), url.PathEscape(prNumber), url.PathEscape(reviewID))
	var out []githubReviewComment
	for page := 1; page <= githubReviewCommentMaxPage; page++ {
		resp, err := h.githubGet(ctx, conn, path, fmt.Sprintf("per_page=%d&page=%d", githubAPIPerPage, page))
		if err != nil || resp.StatusCode < 200 || resp.StatusCode >= 300 {
			logging.FromContext(ctx).WarnContext(ctx, "github review comments fetch failed",
				"repo", repo, "review_id", reviewID, "page", page)
			return out
		}
		var items []githubReviewCommentAPI
		if err := json.Unmarshal(resp.Body, &items); err != nil {
			return out
		}
		for _, it := range items {
			out = append(out, githubReviewComment{
				Path:     it.Path,
				Line:     reviewCommentLine(it),
				Body:     it.Body,
				DiffHunk: it.DiffHunk,
			})
			if len(out) >= githubReviewCommentMax {
				return out
			}
		}
		if len(items) < githubAPIPerPage {
			break
		}
	}
	return out
}

func reviewCommentLine(c githubReviewCommentAPI) int {
	if c.Line > 0 {
		return c.Line
	}
	return c.OriginalLine
}
