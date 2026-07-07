package tasks

import (
	"context"
	"strings"
	"time"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

// deliverGitHubPROpened handles the curated auto-review trigger: whenever a
// pull request is opened in the trigger's repository, kick off a code-review
// run — no @mention required. It only ever runs for the code-reviews app (see
// isGitHubPROpenedTrigger), so every side effect (eyes reaction, delivery) is
// authenticated as the delivering usehivy-reviews connection.
//
// Guard ladder (cheap DB-free checks first, one DB read last):
//   - the event must parse as a pull_request.opened PR event,
//   - the repository must equal the trigger's repo (trigger.TriggerValue),
//   - the PR author must NOT be the code-reviews bot itself — that is the only
//     self-loop we defend against. We deliberately do NOT skip other bots:
//     PRs opened by the primary Hivy app (usehivy[bot], i.e. build-agent PRs)
//     and by e.g. dependabot[bot] SHOULD kick off a review. Reviewing those is
//     the whole point of this feature, so the guard matches ONLY the delivering
//     connection's own bot handle via githubLoginMatchesHandle.
//
// The review always runs in the trigger's own resource-keyed session
// (deliverCompiled → github/<repo>/pull/<n>); it never hijacks the build
// session that opened the PR (that override is primary-only, mention-path only).
func (h *AgentTriggerDispatchHandler) deliverGitHubPROpened(ctx context.Context, payload AgentTriggerDispatchPayload, trigger model.AgentTrigger, webhookPayload map[string]any) error {
	event, ok := parseGitHubMentionEvent(payload, webhookPayload)
	skipReason := ""
	switch {
	case !ok || !event.IsPR:
		skipReason = "event is not a pull_request.opened event"
	case !strings.EqualFold(event.Repo, trigger.TriggerValue):
		skipReason = "event repository does not match trigger"
	}
	if skipReason != "" {
		logging.FromContext(ctx).InfoContext(ctx, "github pr-opened trigger skipped event",
			"trigger_id", trigger.ID,
			"agent_id", trigger.AgentID,
			"delivery_id", payload.DeliveryID,
			"event_key", eventKey(payload.EventType, payload.EventAction),
			"reason", skipReason)
		return nil
	}

	// Self-loop guard: skip only when the code-reviews bot itself opened the PR.
	// botHandle is the delivering connection's handle ("usehivy-reviews"); an
	// empty handle (misconfigured integration) cannot be self, so we proceed —
	// unlike the mention path there is no @handle requirement to fail closed on.
	botHandle := h.githubConnectionBotHandle(ctx, payload)
	if botHandle != "" && githubLoginMatchesHandle(event.OpenedBy, botHandle) {
		logging.FromContext(ctx).InfoContext(ctx, "github pr-opened trigger skipped event",
			"trigger_id", trigger.ID,
			"agent_id", trigger.AgentID,
			"delivery_id", payload.DeliveryID,
			"event_key", eventKey(payload.EventType, payload.EventAction),
			"reason", "pull request opened by the code-reviews bot itself")
		return nil
	}

	// Acknowledge the PR with an eyes reaction on its body before any delivery
	// work — same placement as the mention path. The reaction posts under the
	// delivering (code-reviews) connection's identity, so the human sees
	// @usehivy-reviews noticed the PR instantly.
	h.addGitHubEyesReaction(ctx, payload, mentionReactionTarget(event))

	agent, err := h.loadTriggerAgent(ctx, trigger)
	if err != nil {
		return err
	}
	compiled := h.compileGitHubPROpenedMessage(payload, trigger, event)
	return h.deliverCompiled(ctx, payload, trigger, agent, compiled)
}

// compileGitHubPROpenedMessage frames the auto-review run. It reuses the mention
// message's resource key, reply directive, and Raw envelope, differing only in
// the opening line (a review request, not a mention) so the review lands in the
// same PR-scoped session as any follow-up mention.
func (h *AgentTriggerDispatchHandler) compileGitHubPROpenedMessage(payload AgentTriggerDispatchPayload, trigger model.AgentTrigger, event githubMentionEvent) compiledTriggerMessage {
	resourceKey := githubMentionResourceKey(event)

	var b strings.Builder
	b.WriteString(githubReplyDirective)
	b.WriteString("\n\nA pull request was opened and this trigger requests a code review.\n\n")
	if strings.TrimSpace(trigger.Instructions) != "" {
		b.WriteString("Instructions:\n")
		b.WriteString(strings.TrimSpace(trigger.Instructions))
		b.WriteString("\n\n")
	}
	b.WriteString("Event:\n")
	writeKV(&b, "provider", payload.Provider)
	writeKV(&b, "event_key", eventKey(payload.EventType, payload.EventAction))
	writeKV(&b, "repository", event.Repo)
	writeKV(&b, "pull_request", "#"+event.Number+" — "+event.Title)
	writeKV(&b, "url", event.URL)
	writeKV(&b, "opened_by", event.OpenedBy)
	writeKV(&b, "resource_key", resourceKey)

	if strings.TrimSpace(event.Body) != "" {
		b.WriteString("\nDescription:\n")
		b.WriteString(truncateForPrompt(event.Body, githubMentionBodyLimit))
		b.WriteByte('\n')
	}

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
