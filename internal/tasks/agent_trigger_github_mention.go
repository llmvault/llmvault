package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

const (
	githubMentionBodyLimit       = 6000
	githubMentionCommentLimit    = 1500
	githubMentionMaxComments     = 10
	githubMentionCommentsPerPage = 100 // endpoint maximum (docs.github.com/en/rest/issues/comments)
)

var githubHandlePattern = regexp.MustCompile(`@([A-Za-z0-9](?:[A-Za-z0-9-]*[A-Za-z0-9])?)`)

func isGitHubMentionTrigger(provider string, trigger model.AgentTrigger) bool {
	return trigger.TriggerKey == model.TriggerKeyGitHubMention && strings.HasPrefix(provider, "github")
}

// githubMentionEvent is the normalized view of the three webhook shapes a
// mention trigger subscribes to (issue_comment.created, issues.opened,
// pull_request.opened).
type githubMentionEvent struct {
	Repo         string // repository.full_name
	Number       string
	Title        string
	URL          string
	OpenedBy     string // author of the issue / pull request
	MentionedBy  string // author of the mentioning comment or body
	AuthorType   string // user.type of the author ("User", "Bot", ...)
	Body         string // the text the mention appears in
	IssueBody    string // issue/PR description when the mention is a comment
	CommentID    string // comment.id for comment mentions
	CommentCount int    // issue.comments at event time, drives context paging
	IsPR         bool
	IsComment    bool
}

func (h *AgentTriggerDispatchHandler) deliverGitHubMention(ctx context.Context, payload AgentTriggerDispatchPayload, trigger model.AgentTrigger, webhookPayload map[string]any) error {
	event, ok := parseGitHubMentionEvent(payload, webhookPayload)
	skipReason := ""
	switch {
	case !ok:
		skipReason = "event shape not supported"
	case !strings.EqualFold(event.Repo, trigger.TriggerValue):
		skipReason = "event repository does not match trigger"
	case isHivyIdentityValue(event.MentionedBy):
		// Loop protection: never respond to content Hivy itself authored.
		skipReason = "event author is Hivy"
	case isGitHubBotAuthor(event.MentionedBy, event.AuthorType):
		// Loop protection: two bots can ping-pong through mentions, so no
		// bot-authored content triggers a run. App bots always carry the
		// "<slug>[bot]" login. We deliberately do NOT use
		// performed_via_github_app here: comments humans post via GitHub
		// Mobile set that field too.
		skipReason = "event author is a bot"
	case !githubTextMentionsHivy(event.Body):
		skipReason = "no Hivy mention in body"
	}
	if skipReason != "" {
		logging.FromContext(ctx).InfoContext(ctx, "github mention trigger skipped event",
			"trigger_id", trigger.ID,
			"agent_id", trigger.AgentID,
			"delivery_id", payload.DeliveryID,
			"event_key", eventKey(payload.EventType, payload.EventAction),
			"reason", skipReason)
		return nil
	}

	agent, err := h.loadTriggerAgent(ctx, trigger)
	if err != nil {
		return err
	}
	compiled := h.compileGitHubMentionMessage(ctx, payload, trigger, event)
	return h.deliverCompiled(ctx, payload, trigger, agent, compiled)
}

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

// isGitHubBotAuthor reports whether a webhook author is a bot account.
// GitHub App installations always act as "<app-slug>[bot]" users with
// user.type "Bot" (docs.github.com webhook payload reference).
func isGitHubBotAuthor(login, userType string) bool {
	return strings.HasSuffix(strings.ToLower(strings.TrimSpace(login)), "[bot]") ||
		strings.EqualFold(strings.TrimSpace(userType), "Bot")
}

// githubTextMentionsHivy reports whether any @handle in the text belongs to
// the Hivy GitHub App identity.
func githubTextMentionsHivy(text string) bool {
	for _, match := range githubHandlePattern.FindAllStringSubmatch(text, -1) {
		if isHivyIdentityValue(match[1]) {
			return true
		}
	}
	return false
}

// githubMentionResourceKey follows the canonical resource templates from
// github-app.resources.json so all events on the same issue or PR share one
// session.
func githubMentionResourceKey(event githubMentionEvent) string {
	kind := "issue"
	if event.IsPR {
		kind = "pull"
	}
	return "github/" + event.Repo + "/" + kind + "/" + event.Number
}

func (h *AgentTriggerDispatchHandler) compileGitHubMentionMessage(ctx context.Context, payload AgentTriggerDispatchPayload, trigger model.AgentTrigger, event githubMentionEvent) compiledTriggerMessage {
	resourceKey := githubMentionResourceKey(event)
	subject := "issue"
	if event.IsPR {
		subject = "pull_request"
	}

	var b strings.Builder
	b.WriteString("You were mentioned on GitHub.\n\n")
	if strings.TrimSpace(trigger.Instructions) != "" {
		b.WriteString("Instructions:\n")
		b.WriteString(strings.TrimSpace(trigger.Instructions))
		b.WriteString("\n\n")
	}
	b.WriteString("Event:\n")
	writeKV(&b, "provider", payload.Provider)
	writeKV(&b, "event_key", eventKey(payload.EventType, payload.EventAction))
	writeKV(&b, "repository", event.Repo)
	writeKV(&b, subject, "#"+event.Number+" — "+event.Title)
	writeKV(&b, "url", event.URL)
	writeKV(&b, "opened_by", event.OpenedBy)
	writeKV(&b, "mentioned_by", event.MentionedBy)
	writeKV(&b, "resource_key", resourceKey)

	if event.IsComment && strings.TrimSpace(event.IssueBody) != "" {
		b.WriteString("\nDescription:\n")
		b.WriteString(truncateForPrompt(event.IssueBody, githubMentionBodyLimit))
		b.WriteByte('\n')
	}
	if comments := h.fetchGitHubMentionComments(ctx, payload, event); len(comments) > 0 {
		b.WriteString("\nRecent comments (oldest first):\n")
		for _, comment := range comments {
			b.WriteString("[")
			b.WriteString(comment.author)
			b.WriteString("] ")
			b.WriteString(truncateForPrompt(comment.body, githubMentionCommentLimit))
			b.WriteByte('\n')
		}
	}
	b.WriteString("\nMention:\n[")
	b.WriteString(event.MentionedBy)
	b.WriteString("] ")
	b.WriteString(truncateForPrompt(event.Body, githubMentionBodyLimit))
	b.WriteByte('\n')

	return compiledTriggerMessage{
		Text:        b.String(),
		ResourceKey: resourceKey,
		Raw: map[string]any{
			"source":       "trigger",
			"trigger_id":   trigger.ID.String(),
			"trigger_type": trigger.TriggerType,
			"provider":     payload.Provider,
			"event_key":    eventKey(payload.EventType, payload.EventAction),
			"delivery_id":  payload.DeliveryID,
			"resource_key": resourceKey,
			"received_at":  time.Now().UTC().Format(time.RFC3339),
		},
	}
}

type githubMentionComment struct {
	author string
	body   string
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
