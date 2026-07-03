package tasks

import (
	"context"
	"regexp"
	"strings"
	"time"

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
