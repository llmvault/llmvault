package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

func parseGitHubMentionEvent(payload AgentTriggerDispatchPayload, webhookPayload map[string]any) (githubMentionEvent, bool) {
	repo := payloadPathString(webhookPayload, "repository.full_name")
	if repo == "" {
		return githubMentionEvent{}, false
	}
	event := githubMentionEvent{Repo: repo}
	switch eventKey(payload.EventType, payload.EventAction) {
	case "issue_comment.created":
		event.IsComment = true
		event.Number = payloadPathString(webhookPayload, "issue.number")
		event.Title = payloadPathString(webhookPayload, "issue.title")
		event.URL = payloadPathString(webhookPayload, "comment.html_url")
		event.OpenedBy = payloadPathString(webhookPayload, "issue.user.login")
		event.MentionedBy = payloadPathString(webhookPayload, "comment.user.login")
		event.AuthorType = payloadPathString(webhookPayload, "comment.user.type")
		event.Body = payloadPathString(webhookPayload, "comment.body")
		event.IssueBody = payloadPathString(webhookPayload, "issue.body")
		event.CommentID = payloadPathString(webhookPayload, "comment.id")
		event.CommentCount = payloadPathInt(webhookPayload, "issue.comments")
		_, event.IsPR = lookupTriggerPayloadPath(webhookPayload, "issue.pull_request")
	case "issues.opened":
		event.Number = payloadPathString(webhookPayload, "issue.number")
		event.Title = payloadPathString(webhookPayload, "issue.title")
		event.URL = payloadPathString(webhookPayload, "issue.html_url")
		event.OpenedBy = payloadPathString(webhookPayload, "issue.user.login")
		event.MentionedBy = event.OpenedBy
		event.AuthorType = payloadPathString(webhookPayload, "issue.user.type")
		event.Body = payloadPathString(webhookPayload, "issue.body")
	case "pull_request.opened":
		event.IsPR = true
		event.Number = payloadPathString(webhookPayload, "pull_request.number")
		event.Title = payloadPathString(webhookPayload, "pull_request.title")
		event.URL = payloadPathString(webhookPayload, "pull_request.html_url")
		event.OpenedBy = payloadPathString(webhookPayload, "pull_request.user.login")
		event.MentionedBy = event.OpenedBy
		event.AuthorType = payloadPathString(webhookPayload, "pull_request.user.type")
		event.Body = payloadPathString(webhookPayload, "pull_request.body")
	default:
		return githubMentionEvent{}, false
	}
	if event.Number == "" || event.MentionedBy == "" {
		return githubMentionEvent{}, false
	}
	return event, true
}

// fetchGitHubMentionComments loads recent discussion context for comment
// mentions, best-effort: prompt compilation proceeds without it on any error.
//
// The per-issue comments endpoint returns comments ascending by id with no
// direction parameter (docs.github.com/en/rest/issues/comments), so the most
// recent comments live on the LAST page. issue.comments from the webhook
// payload tells us where that is.
func (h *AgentTriggerDispatchHandler) fetchGitHubMentionComments(ctx context.Context, payload AgentTriggerDispatchPayload, event githubMentionEvent) []githubMentionComment {
	if !event.IsComment || h.nangoClient == nil {
		return nil
	}
	var conn model.Connection
	if err := h.db.WithContext(ctx).
		Preload("Integration").
		Where("id = ? AND org_id = ? AND revoked_at IS NULL", payload.ConnectionID, payload.OrgID).
		First(&conn).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			logging.CaptureWithFields(ctx, fmt.Errorf("github mention: load connection: %w", err), map[string]any{
				"org_id": payload.OrgID.String(),
			})
		}
		return nil
	}
	owner, repoName, ok := strings.Cut(event.Repo, "/")
	if !ok {
		return nil
	}
	path := fmt.Sprintf("/repos/%s/%s/issues/%s/comments",
		url.PathEscape(owner), url.PathEscape(repoName), url.PathEscape(event.Number))

	var comments []githubMentionComment
	for _, page := range githubMentionCommentPages(event.CommentCount) {
		resp, err := h.nangoClient.ProxyRequest(ctx, http.MethodGet, conn.Integration.UniqueKey, conn.NangoConnectionID, path,
			map[string]string{"per_page": strconv.Itoa(githubMentionCommentsPerPage), "page": strconv.Itoa(page)}, nil)
		if err != nil {
			logging.FromContext(ctx).WarnContext(ctx, "github mention: fetch comments failed",
				"repository", event.Repo, "number", event.Number, "page", page, "error", err)
			return nil
		}
		pageComments := make([]githubMentionComment, 0, githubMentionMaxComments)
		for _, item := range proxyResponseArray(resp) {
			obj, ok := item.(map[string]any)
			if !ok {
				continue
			}
			body := payloadPathString(obj, "body")
			author := payloadPathString(obj, "user.login")
			if body == "" || author == "" {
				continue
			}
			// The triggering comment is appended separately as the mention.
			if event.CommentID != "" && payloadPathString(obj, "id") == event.CommentID {
				continue
			}
			pageComments = append(pageComments, githubMentionComment{author: author, body: body})
		}
		comments = append(pageComments, comments...)
		if len(comments) >= githubMentionMaxComments {
			break
		}
	}
	if len(comments) > githubMentionMaxComments {
		comments = comments[len(comments)-githubMentionMaxComments:]
	}
	return comments
}

// githubMentionCommentPages returns the page numbers to fetch, last page
// first, so the tail of the discussion is loaded before older pages.
func githubMentionCommentPages(commentCount int) []int {
	lastPage := max((commentCount+githubMentionCommentsPerPage-1)/githubMentionCommentsPerPage, 1)
	pages := []int{lastPage}
	if lastPage > 1 {
		pages = append(pages, lastPage-1)
	}
	return pages
}

// proxyResponseArray unwraps a JSON-array proxy response: the Nango client
// stores non-object bodies as a raw JSON string under "_raw".
func proxyResponseArray(resp map[string]any) []any {
	if resp == nil {
		return nil
	}
	switch raw := resp["_raw"].(type) {
	case []any:
		return raw
	case string:
		var items []any
		if err := json.Unmarshal([]byte(raw), &items); err == nil {
			return items
		}
	}
	return nil
}

func payloadPathString(payload map[string]any, path string) string {
	value, ok := lookupTriggerPayloadPath(payload, path)
	if !ok {
		return ""
	}
	// JSON numbers decode as float64; format issue/PR numbers without the
	// exponent notation fmt.Sprint would use for large values.
	if f, isFloat := value.(float64); isFloat {
		return strconv.FormatFloat(f, 'f', -1, 64)
	}
	return scalarString(value)
}

func payloadPathInt(payload map[string]any, path string) int {
	value, ok := lookupTriggerPayloadPath(payload, path)
	if !ok {
		return 0
	}
	if f, isFloat := value.(float64); isFloat {
		return int(f)
	}
	return 0
}

func truncateForPrompt(text string, limit int) string {
	text = strings.TrimSpace(text)
	if len(text) <= limit {
		return text
	}
	return text[:limit] + "\n… (truncated)"
}
