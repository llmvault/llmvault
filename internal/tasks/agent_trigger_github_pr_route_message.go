package tasks

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// prRouteDirective heads every auto-routed PR message. It mirrors
// githubReplyDirective's framing but stresses that the message is automated and
// that the user cannot read chat replies — the agent must act on GitHub.
const prRouteDirective = `<system_message>
This is an AUTOMATED notification from Hivy about GitHub activity on a pull request that THIS session created. It was not written by a human in this chat. The user cannot see your chat replies — anything a human (a reviewer or the PR author) should read must be posted on GitHub itself, as a pull request comment or a review reply. If the git-github skill is not already loaded, load it first (the git-github skill) so you have the tools to inspect the pull request and respond on GitHub.
</system_message>`

// compilePRRouteMessage renders the agent-facing message for a PR-scoped event
// routed into the session that opened the PR. ResourceKey follows the same
// convention as githubMentionResourceKey so PR events share the PR's session.
func compilePRRouteMessage(payload AgentTriggerDispatchPayload, event prRouteEvent, prNumber string) compiledTriggerMessage {
	resourceKey := "github/" + event.Repo + "/pull/" + prNumber
	var b strings.Builder
	b.WriteString(prRouteDirective)
	b.WriteString("\n\n")

	switch event.EventKey {
	case "check_suite.completed":
		writeCheckSuiteMessage(&b, payload, event, prNumber, resourceKey)
	case "pull_request_review.submitted":
		writeReviewMessage(&b, payload, event, prNumber, resourceKey)
	default:
		writeCommentMessage(&b, payload, event, prNumber, resourceKey)
	}

	return compiledTriggerMessage{
		Text:        b.String(),
		ResourceKey: resourceKey,
		Raw: map[string]any{
			"source":       triggerConversationSource,
			"provider":     payload.Provider,
			"event_key":    event.EventKey,
			"delivery_id":  payload.DeliveryID,
			"resource_key": resourceKey,
			"received_at":  time.Now().UTC().Format(time.RFC3339),
			"repo":         event.Repo,
			"pr_number":    prNumber,
		},
	}
}

func writeCheckSuiteMessage(b *strings.Builder, payload AgentTriggerDispatchPayload, event prRouteEvent, prNumber, resourceKey string) {
	b.WriteString("CI checks completed on the pull request this session opened.\n\n")
	b.WriteString("Event:\n")
	writeKV(b, "provider", payload.Provider)
	writeKV(b, "event_key", event.EventKey)
	writeKV(b, "repository", event.Repo)
	writeKV(b, "pull_request", "#"+prNumber)
	writeKV(b, "head_branch", event.HeadBranch)
	writeKV(b, "head_sha", event.HeadSHA)
	if s := event.CheckSummary; s != nil {
		writeKV(b, "check_suites", strconv.Itoa(s.Total))
		writeKV(b, "passed", strconv.Itoa(s.Success))
		writeKV(b, "failed", strconv.Itoa(s.Failure))
		if s.Neutral > 0 {
			writeKV(b, "neutral", strconv.Itoa(s.Neutral))
		}
		writeKV(b, "overall", s.Overall)
	} else {
		writeKV(b, "conclusion", event.Conclusion)
		writeKV(b, "status", event.Status)
	}
	writeKV(b, "resource_key", resourceKey)

	b.WriteString("\nGuidance:\n")
	if checkSuiteOverall(event) == "success" {
		b.WriteString("All checks passed. Continue with your task. If a brief status note would help reviewers, post it on the pull request; otherwise no GitHub action is required.\n")
		return
	}
	branch := event.HeadBranch
	if strings.TrimSpace(branch) == "" {
		branch = "the PR branch"
	}
	b.WriteString("The checks did not all pass (" + checkSuiteConclusionText(event) + "). Investigate the failing checks with the gh CLI (for example `gh pr checks " + prNumber + "` and `gh run view --log-failed`), fix the underlying problem, and push the fix to " + branch + ". Report what you changed on the pull request.\n")
}

// checkSuiteOverall reports the aggregate outcome, preferring the multi-suite
// summary so a green last suite cannot mask an earlier red one, falling back to
// the single suite's conclusion when the aggregate was unavailable.
func checkSuiteOverall(event prRouteEvent) string {
	if event.CheckSummary != nil {
		return event.CheckSummary.Overall
	}
	if strings.EqualFold(event.Conclusion, "success") {
		return "success"
	}
	return "failure"
}

func checkSuiteConclusionText(event prRouteEvent) string {
	if s := event.CheckSummary; s != nil {
		return fmt.Sprintf("overall: failure, %d of %d suites failed", s.Failure, s.Total)
	}
	conclusion := strings.TrimSpace(event.Conclusion)
	if conclusion == "" {
		conclusion = "not successful"
	}
	return "conclusion: " + conclusion
}

func writeReviewMessage(b *strings.Builder, payload AgentTriggerDispatchPayload, event prRouteEvent, prNumber, resourceKey string) {
	b.WriteString("A reviewer submitted a review on the pull request this session opened.\n\n")
	b.WriteString("Event:\n")
	writeKV(b, "provider", payload.Provider)
	writeKV(b, "event_key", event.EventKey)
	writeKV(b, "repository", event.Repo)
	writeKV(b, "pull_request", "#"+prNumber)
	writeKV(b, "reviewer", event.Reviewer)
	writeKV(b, "state", event.ReviewState)
	writeKV(b, "resource_key", resourceKey)

	hasBody := strings.TrimSpace(event.ReviewBody) != ""
	if hasBody {
		b.WriteString("\nReview:\n")
		b.WriteString(truncateForPrompt(event.ReviewBody, githubMentionBodyLimit))
		b.WriteByte('\n')
	}
	if len(event.ReviewComments) > 0 {
		b.WriteString("\nInline comments:\n")
		writeReviewComments(b, event.ReviewComments)
	}

	b.WriteString("\nGuidance:\n")
	if !hasBody && len(event.ReviewComments) == 0 {
		if strings.EqualFold(event.ReviewState, "approved") {
			b.WriteString("The reviewer approved the pull request with no written feedback. No changes are required; continue with your task.\n")
			return
		}
		b.WriteString("The reviewer submitted a review with no written feedback and no inline comments. No action is required for this review.\n")
		return
	}
	b.WriteString("Address the review feedback — make the requested code changes if changes were requested — then reply to the review on GitHub so the reviewer sees your response.\n")
}

func writeReviewComments(b *strings.Builder, comments []githubReviewComment) {
	for i, c := range comments {
		loc := strings.TrimSpace(c.Path)
		if c.Line > 0 {
			loc += ":" + strconv.Itoa(c.Line)
		}
		if loc == "" {
			loc = "(unlocated)"
		}
		b.WriteString(strconv.Itoa(i+1) + ". " + loc + "\n")
		if strings.TrimSpace(c.DiffHunk) != "" {
			b.WriteString(truncateForPrompt(c.DiffHunk, githubMentionCommentLimit))
			b.WriteByte('\n')
		}
		if strings.TrimSpace(c.Body) != "" {
			b.WriteString(truncateForPrompt(c.Body, githubMentionCommentLimit))
			b.WriteByte('\n')
		}
	}
}

func writeCommentMessage(b *strings.Builder, payload AgentTriggerDispatchPayload, event prRouteEvent, prNumber, resourceKey string) {
	kind := "comment"
	if event.IsInline {
		kind = "inline review comment"
	}
	b.WriteString("A new " + kind + " was posted on the pull request this session opened.\n\n")
	b.WriteString("Event:\n")
	writeKV(b, "provider", payload.Provider)
	writeKV(b, "event_key", event.EventKey)
	writeKV(b, "repository", event.Repo)
	writeKV(b, "pull_request", "#"+prNumber)
	writeKV(b, "author", event.Author)
	if event.IsInline {
		writeKV(b, "path", event.Path)
		writeKV(b, "line", event.Line)
	}
	writeKV(b, "resource_key", resourceKey)

	if event.IsInline && strings.TrimSpace(event.DiffHunk) != "" {
		b.WriteString("\nDiff hunk:\n")
		b.WriteString(truncateForPrompt(event.DiffHunk, githubMentionCommentLimit))
		b.WriteByte('\n')
	}
	if strings.TrimSpace(event.Body) != "" {
		b.WriteString("\nComment:\n")
		b.WriteString(truncateForPrompt(event.Body, githubMentionBodyLimit))
		b.WriteByte('\n')
	}
	b.WriteString("\nGuidance:\n")
	b.WriteString("Read the comment and respond on GitHub. Apply the requested changes if any, then reply on the pull request.\n")
}
