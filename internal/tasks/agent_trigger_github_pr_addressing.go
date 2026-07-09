package tasks

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

// prRouteAddressingText returns the human-authored text whose @mentions decide
// which Hivy app a PR-scoped event is addressed to. Only comments and review
// bodies carry such text; check_suite has none and is never addressed away.
func prRouteAddressingText(event prRouteEvent) string {
	switch event.EventKey {
	case "pull_request_review.submitted":
		return event.ReviewBody
	case "issue_comment.created", "pull_request_review_comment.created":
		return event.Body
	}
	return ""
}

// prRouteAddressedToOtherApp reports whether a PR comment/review is addressed
// exclusively to the org's OTHER Hivy app by @mention, and returns that app's
// handle when so. Mentions are the addressing mechanism: a comment naming only
// the code-reviews bot (e.g. "@usehivy-reviews") on a primary-owned PR must not
// spend a build turn — the other app's own mention trigger answers it. Naming
// the PR-owning app's own handle (alone or alongside the other) keeps delivery,
// and text with no Hivy handle keeps delivery. The decision is made only when
// the PR-owning app's own handle is known, so a misconfigured connection fails
// open (route) rather than silently dropping an event.
func (h *AgentTriggerDispatchHandler) prRouteAddressedToOtherApp(ctx context.Context, payload AgentTriggerDispatchPayload, event prRouteEvent, ownHandle string) (bool, string) {
	if strings.TrimSpace(ownHandle) == "" {
		return false, ""
	}
	text := prRouteAddressingText(event)
	if strings.TrimSpace(text) == "" {
		return false, ""
	}
	otherHandle := h.githubOtherAppBotHandle(ctx, payload.OrgID, ownHandle)
	if otherHandle == "" {
		return false, ""
	}
	if !githubTextMentionsHandle(text, otherHandle) {
		return false, ""
	}
	if githubTextMentionsHandle(text, ownHandle) {
		return false, ""
	}
	return true, otherHandle
}

// githubOtherAppBotHandle resolves the bot handle of the org's OTHER Hivy GitHub
// App: the code-reviews app when ownHandle is the primary's, and vice versa. It
// reads the handle from the other app's live connection so nothing is
// hardcoded. Returns "" when the org has no second Hivy GitHub App connection or
// its handle is unset. Best-effort: a nil db or query error returns "".
func (h *AgentTriggerDispatchHandler) githubOtherAppBotHandle(ctx context.Context, orgID uuid.UUID, ownHandle string) string {
	if h == nil || h.db == nil {
		return ""
	}
	var conns []model.Connection
	if err := h.db.WithContext(ctx).
		Joins("JOIN integrations ON integrations.id = connections.integration_id").
		Where("connections.org_id = ? AND connections.revoked_at IS NULL AND integrations.provider IN ?",
			orgID, []string{githubPrimaryProvider, githubCodeReviewsProvider}).
		Preload("Integration").
		Find(&conns).Error; err != nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("github: load other app bot handle: %w", err), map[string]any{
			"org_id": orgID.String(),
		})
		return ""
	}
	own := normalizeGitHubLogin(ownHandle)
	for _, conn := range conns {
		handle := conn.Integration.BotHandle
		if handle == "" || normalizeGitHubLogin(handle) == own {
			continue
		}
		return handle
	}
	return ""
}
