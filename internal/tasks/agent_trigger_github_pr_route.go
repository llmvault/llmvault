package tasks

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
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
	ReviewID    string // review.id (review) / comment.pull_request_review_id (comment)

	Author      string // comment author (issue_comment / review_comment)
	Body        string
	CommentID   string // bare comment.id, used to target the eyes reaction
	Path        string // inline review comment location
	Line        string
	DiffHunk    string
	IsInline    bool
	InReplyToID string // review comment reply target

	CheckSummary   *checkSuitesSummary   // aggregate CI state, check_suite only
	ReviewComments []githubReviewComment // inline comments attached to a review

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
	// connection; its integration drives the self-guard, the write-access filter,
	// and every GitHub API call below.
	conn, connOK := h.loadTriggerConnection(ctx, payload)
	primaryHandle := ""
	if connOK {
		primaryHandle = conn.Integration.BotHandle
	}

	// check_suite has no author; comments and reviews must not loop back on
	// content the primary bot or the PR author itself produced.
	if event.guardAuthor && prRouteSelfAuthored(event.Author, event.PRAuthor, primaryHandle) {
		log.InfoContext(ctx, "github pr event self-authored, skipping",
			"repo", event.Repo, "event_key", key, "author", event.Author)
		return nil, nil
	}

	// A review comment that belongs to a submitted review is already covered by
	// the aggregated review turn; only thread replies route on their own.
	if key == "pull_request_review_comment.created" && suppressReviewBoundComment(event) {
		log.InfoContext(ctx, "github review comment covered by review turn, skipping",
			"repo", event.Repo, "review_id", event.ReviewID, "comment_id", event.CommentID)
		return nil, nil
	}

	// Mention-exclusive addressing: a comment/review that @mentions only the
	// OTHER Hivy app (e.g. the code-reviews bot on a primary-owned PR) is not for
	// the build session — the other app's mention trigger handles it. Skip before
	// the write-access API call so an addressed-away event costs nothing.
	if claimed, claimant := h.prRouteAddressedToOtherApp(ctx, payload, event, primaryHandle); claimed {
		log.InfoContext(ctx, "github pr event addressed to other hivy app, skipping",
			"repo", event.Repo, "event_key", key, "claimed_by", claimant)
		return nil, nil
	}

	if skip, err := h.prRouteAuthorBlocked(ctx, conn, connOK, event); err != nil {
		return nil, err
	} else if skip {
		return nil, nil
	}

	eventID := prRouteEventID(payload, key, event.StableID)
	if key == "check_suite.completed" {
		proceed, err := h.settleCheckSuite(ctx, conn, connOK, &event)
		if err != nil {
			return nil, err
		}
		if !proceed {
			log.InfoContext(ctx, "github check suite not settled, waiting",
				"repo", event.Repo, "head_sha", event.HeadSHA)
			return nil, nil
		}
		if event.HeadSHA != "" {
			eventID = "github:check_suite.head_sha_settled:" + event.HeadSHA
		}
	}

	if key == "pull_request_review.submitted" && connOK && h.nangoClient != nil && len(prNumbers) > 0 {
		event.ReviewComments = h.fetchReviewComments(ctx, conn, event.Repo, prNumbers[0], event.ReviewID)
	}

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

// prRouteAuthorBlocked reports whether a comment/review author lacks the write
// standing required to spend an agent turn. It applies only to guardAuthor
// events (check_suite is unaffected) and only when the GitHub API is reachable;
// Hivy's own apps (the code-reviews bot) are trusted and bypass the check so
// their feedback still routes into the build session. Any non-2xx (bots and
// non-collaborators 404) fails closed.
func (h *AgentTriggerDispatchHandler) prRouteAuthorBlocked(ctx context.Context, conn *model.Connection, connOK bool, event prRouteEvent) (bool, error) {
	if !event.guardAuthor || h.nangoClient == nil {
		return false, nil
	}
	if isGitHubHivyBotLogin(event.Author) {
		return false, nil
	}
	log := logging.FromContext(ctx)
	if !connOK {
		log.WarnContext(ctx, "github pr event author unverifiable (no connection), skipping",
			"repo", event.Repo, "author", event.Author)
		return true, nil
	}
	allowed, err := h.githubAuthorMayRoute(ctx, conn, event.Repo, event.Author)
	if err != nil {
		return false, err
	}
	if !allowed {
		log.InfoContext(ctx, "github pr event author lacks write access, skipping",
			"repo", event.Repo, "author", event.Author, "event_key", event.EventKey)
		return true, nil
	}
	return false, nil
}

// settleCheckSuite gates a check_suite.completed delivery on every check suite
// for the head SHA having finished, and enriches the event with the aggregate
// pass/fail summary. When the API is unreachable it degrades to routing on the
// single suite's own payload; the head-SHA event id still collapses the burst
// of per-workflow completions into one delivery.
func (h *AgentTriggerDispatchHandler) settleCheckSuite(ctx context.Context, conn *model.Connection, connOK bool, event *prRouteEvent) (bool, error) {
	if h.nangoClient == nil || !connOK || event.HeadSHA == "" {
		return true, nil
	}
	summary, settled, err := h.checkSuiteSettleState(ctx, conn, event.Repo, event.HeadSHA)
	if err != nil {
		return false, err
	}
	if !settled {
		return false, nil
	}
	event.CheckSummary = summary
	return true, nil
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
