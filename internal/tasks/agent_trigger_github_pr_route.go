package tasks

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/logging"
)

// prRouteEvent is the normalized view of a PR-scoped GitHub event that carries
// enough context to compile a message and dedup the delivery, independent of
// which of the four webhook shapes it came from.
type prRouteEvent struct {
	Repo     string
	EventKey string
	StableID string // connection-free payload id used for the transcript event id

	HeadBranch string // check_suite
	HeadSHA    string
	Conclusion string
	Status     string

	Reviewer    string // pull_request_review
	ReviewState string
	ReviewBody  string

	Author    string // comment author (issue_comment / review_comment)
	Body      string
	CommentID string // bare comment.id, used to target the eyes reaction
	Path      string // inline review comment location
	Line      string
	DiffHunk  string
	IsInline  bool

	guardAuthor bool // comment/review events carry an author to self-guard on
	PRAuthor    string
}

// isPRRouteEventKey reports whether an event is one of the PR-scoped GitHub
// events that auto-route back into the session that opened the PR, no installed
// trigger required. Auto-route is PRIMARY-app only: the code-reviews app
// receives its own copy of every PR event, and that copy must never be
// auto-routed into a build session (review feedback reaches the build agent
// through the primary app's copy).
func isPRRouteEventKey(provider, key string) bool {
	if !isGitHubPrimary(provider) {
		return false
	}
	switch key {
	case "check_suite.completed",
		"pull_request_review.submitted",
		"pull_request_review_comment.created",
		"issue_comment.created":
		return true
	}
	return false
}

// maybeRoutePREvent delivers a PR-scoped GitHub event into the session that
// opened the PR, when we recorded that mapping. It returns the sessions it
// routed into (session id → transcript event id) so the mention trigger loop
// can suppress a duplicate delivery for the same event. A nil/empty result
// means nothing was routed; the trigger loop still runs afterwards.
func (h *AgentTriggerDispatchHandler) maybeRoutePREvent(ctx context.Context, payload AgentTriggerDispatchPayload, webhookPayload map[string]any) (map[uuid.UUID]string, error) {
	if h.enqueuer == nil {
		return nil, nil
	}
	key := eventKey(payload.EventType, payload.EventAction)
	event, prNumbers, ok := buildPRRouteEvent(key, webhookPayload)
	if !ok {
		return nil, nil
	}
	log := logging.FromContext(ctx)
	// Auto-route is primary-only, so payload.ConnectionID IS the primary
	// connection; load its bot handle once to drive the self-guard and the
	// eyes-reaction gate. A comment authored by the primary bot ("usehivy") or
	// the PR author itself must not loop back; a comment authored by the
	// code-reviews bot ("usehivy-reviews") is NOT self and MUST route (review
	// feedback to the build agent is the whole point).
	primaryHandle := h.githubConnectionBotHandle(ctx, payload)
	// check_suite has no author; comments and reviews must not loop back on
	// content the primary bot or the PR author itself produced.
	if event.guardAuthor && prRouteSelfAuthored(event.Author, event.PRAuthor, primaryHandle) {
		log.InfoContext(ctx, "github pr event self-authored, skipping",
			"repo", event.Repo, "event_key", key, "author", event.Author)
		return nil, nil
	}
	eventID := prRouteEventID(payload, key, event.StableID)
	routed := map[uuid.UUID]string{}
	for _, prNumber := range prNumbers {
		session, ok := h.lookupPRSession(ctx, payload.OrgID, event.Repo, prNumber)
		if !ok {
			log.InfoContext(ctx, "github pr event has no routable session",
				"repo", event.Repo, "pr_number", prNumber, "event_key", key)
			continue
		}
		compiled := compilePRRouteMessage(payload, event, prNumber)
		created, err := h.enqueueTriggerSessionMessage(ctx, session, compiled, eventID, triggerConversationSource)
		if err != nil {
			return routed, err
		}
		if err := EnqueueSessionMessageDeliver(ctx, h.enqueuer, session.ID); err != nil {
			return routed, err
		}
		// Acknowledge with an eyes reaction only when this dispatch task actually
		// enqueued the message. The same event arrives via both installed GitHub
		// App connections; reacting only on the newly-created row avoids a second
		// ack under the other bot identity. Comment events that @mention Hivy
		// react on the comment; check_suite never reacts, and
		// pull_request_review.submitted has no reactions endpoint, so both are
		// skipped here.
		if created {
			h.reactToRoutedComment(ctx, payload, event, primaryHandle)
		}
		routed[session.ID] = eventID
		log.InfoContext(ctx, "github pr event routed to originating session",
			"repo", event.Repo, "pr_number", prNumber, "event_key", key, "session_id", session.ID)
	}
	if len(routed) == 0 {
		return nil, nil
	}
	return routed, nil
}

// prRouteEventID derives the connection-free transcript event id for a routed
// PR event. The dispatch delivery id already carries the connection-prefixed
// stable key, so triggerSessionEventID's stripped suffix matches what the
// trigger path uses for the same event. Only when the delivery id lacks a
// github suffix do we rebuild it from the payload's stable id.
func prRouteEventID(payload AgentTriggerDispatchPayload, key, stableID string) string {
	if id := triggerSessionEventID(payload); strings.HasPrefix(id, "github:") {
		return id
	}
	if stableID != "" {
		return "github:" + key + ":" + stableID
	}
	return triggerSessionEventID(payload)
}

// prRouteSelfAuthored reports whether an event was authored by the primary bot
// itself or by the PR author (the bot that opened the PR), which must not
// trigger a run. It matches the primary bot handle exactly (not a substring),
// so the code-reviews bot ("usehivy-reviews") never counts as self and its
// review feedback still routes into the build session.
func prRouteSelfAuthored(author, prAuthor, primaryHandle string) bool {
	a := normalizeGitHubLogin(author)
	if a == "" {
		return false
	}
	if githubLoginMatchesHandle(author, primaryHandle) {
		return true
	}
	pr := normalizeGitHubLogin(prAuthor)
	return pr != "" && a == pr
}

func normalizeGitHubLogin(login string) string {
	n := strings.ToLower(strings.TrimSpace(login))
	n = strings.TrimSuffix(n, "[bot]")
	return strings.TrimSpace(n)
}

// buildPRRouteEvent extracts the repo, PR number(s), and message context per
// webhook shape. It returns ok=false when the event is not routable (missing
// repo, no PR context, empty check_suite.pull_requests, or a plain issue).
func buildPRRouteEvent(key string, wp map[string]any) (prRouteEvent, []string, bool) {
	repo := payloadText(wp, "repository.full_name")
	if repo == "" {
		return prRouteEvent{}, nil, false
	}
	event := prRouteEvent{Repo: repo, EventKey: key}
	switch key {
	case "check_suite.completed":
		numbers := checkSuitePRNumbers(wp)
		if len(numbers) == 0 {
			return prRouteEvent{}, nil, false
		}
		event.HeadBranch = payloadText(wp, "check_suite.head_branch")
		event.HeadSHA = payloadText(wp, "check_suite.head_sha")
		event.Conclusion = payloadText(wp, "check_suite.conclusion")
		event.Status = payloadText(wp, "check_suite.status")
		event.StableID = payloadNumber(wp, "check_suite.id")
		if event.StableID != "" && event.Conclusion != "" {
			event.StableID += ":" + event.Conclusion
		}
		return event, numbers, true
	case "pull_request_review.submitted":
		number := payloadNumber(wp, "pull_request.number")
		if number == "" {
			return prRouteEvent{}, nil, false
		}
		event.guardAuthor = true
		event.Reviewer = payloadText(wp, "review.user.login")
		event.Author = event.Reviewer
		event.ReviewState = payloadText(wp, "review.state")
		event.ReviewBody = payloadText(wp, "review.body")
		event.PRAuthor = payloadText(wp, "pull_request.user.login")
		event.StableID = payloadNumber(wp, "review.id")
		return event, []string{number}, true
	case "pull_request_review_comment.created":
		number := payloadNumber(wp, "pull_request.number")
		if number == "" {
			return prRouteEvent{}, nil, false
		}
		event.guardAuthor = true
		event.Author = payloadText(wp, "comment.user.login")
		event.Body = payloadText(wp, "comment.body")
		event.Path = payloadText(wp, "comment.path")
		event.Line = commentLine(wp)
		event.DiffHunk = payloadText(wp, "comment.diff_hunk")
		event.IsInline = true
		event.PRAuthor = payloadText(wp, "pull_request.user.login")
		event.CommentID = payloadNumber(wp, "comment.id")
		event.StableID = event.CommentID
		return event, []string{number}, true
	case "issue_comment.created":
		if _, ok := lookupTriggerPayloadPath(wp, "issue.pull_request"); !ok {
			return prRouteEvent{}, nil, false
		}
		number := payloadNumber(wp, "issue.number")
		if number == "" {
			return prRouteEvent{}, nil, false
		}
		event.guardAuthor = true
		event.Author = payloadText(wp, "comment.user.login")
		event.Body = payloadText(wp, "comment.body")
		event.PRAuthor = payloadText(wp, "issue.user.login")
		event.CommentID = payloadNumber(wp, "comment.id")
		event.StableID = event.CommentID
		return event, []string{number}, true
	}
	return prRouteEvent{}, nil, false
}

func checkSuitePRNumbers(wp map[string]any) []string {
	raw, ok := lookupTriggerPayloadPath(wp, "check_suite.pull_requests")
	if !ok {
		return nil
	}
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	var numbers []string
	for _, item := range items {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if number := payloadNumberValue(entry["number"]); number != "" {
			numbers = append(numbers, number)
		}
	}
	return numbers
}

// commentLine prefers the current line, falling back to the original line for
// comments on outdated diffs (either may be null in the payload).
func commentLine(wp map[string]any) string {
	if line := payloadNumber(wp, "comment.line"); line != "" && line != "0" {
		return line
	}
	return payloadNumber(wp, "comment.original_line")
}
