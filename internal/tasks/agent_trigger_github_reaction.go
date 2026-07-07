package tasks

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

// githubReactionKind selects which Reactions API endpoint an eyes reaction is
// posted to (docs.github.com/en/rest/reactions/reactions).
type githubReactionKind string

const (
	// reactionKindIssueComment covers issue and PR comments
	// (issue_comment.*): /issues/comments/{id}/reactions.
	reactionKindIssueComment githubReactionKind = "issueComment"
	// reactionKindIssue covers an issue or PR body (issues.opened,
	// pull_request.opened) — PRs are issues for this endpoint:
	// /issues/{number}/reactions.
	reactionKindIssue githubReactionKind = "issue"
	// reactionKindReviewComment covers inline PR review comments
	// (pull_request_review_comment.created): /pulls/comments/{id}/reactions.
	reactionKindReviewComment githubReactionKind = "reviewComment"
)

// githubReactionTarget identifies the GitHub item to add an eyes reaction to.
// ID is the comment id for the comment kinds and the issue/PR number for the
// issue-body kind.
type githubReactionTarget struct {
	Kind githubReactionKind
	Repo string // "owner/name"
	ID   string
}

// mentionReactionTarget derives the reaction target for a mention event: a
// comment mention reacts on the comment, otherwise (issues.opened /
// pull_request.opened) on the issue/PR body.
func mentionReactionTarget(event githubMentionEvent) githubReactionTarget {
	if event.IsComment {
		return githubReactionTarget{Kind: reactionKindIssueComment, Repo: event.Repo, ID: event.CommentID}
	}
	return githubReactionTarget{Kind: reactionKindIssue, Repo: event.Repo, ID: event.Number}
}

func (t githubReactionTarget) complete() bool {
	return t.Kind != "" && strings.TrimSpace(t.Repo) != "" && strings.TrimSpace(t.ID) != ""
}

// reactionPath returns the Reactions API path for the target, or "" when the
// kind is unknown.
func (t githubReactionTarget) reactionPath(owner, name string) string {
	o, n, id := url.PathEscape(owner), url.PathEscape(name), url.PathEscape(t.ID)
	switch t.Kind {
	case reactionKindIssueComment:
		return fmt.Sprintf("/repos/%s/%s/issues/comments/%s/reactions", o, n, id)
	case reactionKindIssue:
		return fmt.Sprintf("/repos/%s/%s/issues/%s/reactions", o, n, id)
	case reactionKindReviewComment:
		return fmt.Sprintf("/repos/%s/%s/pulls/comments/%s/reactions", o, n, id)
	default:
		return ""
	}
}

// addGitHubEyesReaction adds an "eyes" reaction to the GitHub item so the human
// sees the event was noticed before the agent finishes its turn. It is strictly
// best-effort: every failure is logged as a warning and none is returned to the
// caller, so acknowledging never fails or delays delivery. GitHub reactions are
// idempotent per identity+content, so a duplicate call is a harmless 200.
// reactToRoutedComment acknowledges an auto-routed PR event with an eyes
// reaction when it is a comment that @mentions the PRIMARY handle. Auto-route is
// primary-only, so primaryHandle is the delivering (primary) connection's bot
// handle; the code-reviews app acks its own mentions via its own trigger path
// under its own identity, so this path acks only primary-handle mentions and
// there is no cross-identity double-eyes. Only the two comment shapes have a
// reactions endpoint and only mentioning comments warrant an ack:
// pull_request_review.submitted has no reactions endpoint (skipped) and
// check_suite carries no mention (skipped).
func (h *AgentTriggerDispatchHandler) reactToRoutedComment(ctx context.Context, payload AgentTriggerDispatchPayload, event prRouteEvent, primaryHandle string) {
	if !githubTextMentionsHandle(event.Body, primaryHandle) {
		return
	}
	var kind githubReactionKind
	switch event.EventKey {
	case "issue_comment.created":
		kind = reactionKindIssueComment
	case "pull_request_review_comment.created":
		kind = reactionKindReviewComment
	default:
		return
	}
	h.addGitHubEyesReaction(ctx, payload, githubReactionTarget{Kind: kind, Repo: event.Repo, ID: event.CommentID})
}

func (h *AgentTriggerDispatchHandler) addGitHubEyesReaction(ctx context.Context, payload AgentTriggerDispatchPayload, target githubReactionTarget) {
	if h.nangoClient == nil || !target.complete() {
		return
	}
	log := logging.FromContext(ctx)

	// Load the connection exactly like fetchGitHubMentionComments does so the
	// reaction is posted under the same GitHub App identity as the delivery.
	var conn model.Connection
	if err := h.db.WithContext(ctx).
		Preload("Integration").
		Where("id = ? AND org_id = ? AND revoked_at IS NULL", payload.ConnectionID, payload.OrgID).
		First(&conn).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			logging.CaptureWithFields(ctx, fmt.Errorf("github reaction: load connection: %w", err), map[string]any{
				"org_id": payload.OrgID.String(),
			})
		}
		log.WarnContext(ctx, "github reaction: connection unavailable, skipping ack",
			"repo", target.Repo, "target", string(target.Kind))
		return
	}

	owner, name, ok := strings.Cut(target.Repo, "/")
	if !ok {
		log.WarnContext(ctx, "github reaction: malformed repository, skipping ack",
			"repo", target.Repo, "target", string(target.Kind))
		return
	}
	path := target.reactionPath(owner, name)
	if path == "" {
		return
	}

	// 200 (already existed) and 201 (created) both count as success; ProxyRequest
	// only returns an error for >=400, so any error here is a real failure.
	if _, err := h.nangoClient.ProxyRequest(ctx, http.MethodPost, conn.Integration.UniqueKey, conn.NangoConnectionID, path,
		nil, map[string]string{"content": "eyes"}); err != nil {
		log.WarnContext(ctx, "github reaction: add eyes failed",
			"repo", target.Repo, "target", string(target.Kind),
			"event_key", eventKey(payload.EventType, payload.EventAction), "error", err)
	}
}
