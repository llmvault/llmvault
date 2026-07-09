package tasks

import "strings"

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
		event.ReviewID = event.StableID
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
		event.ReviewID = payloadNumber(wp, "comment.pull_request_review_id")
		event.InReplyToID = payloadNumber(wp, "comment.in_reply_to_id")
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

// suppressReviewBoundComment reports whether a pull_request_review_comment.created
// event is already covered by the aggregated pull_request_review.submitted turn
// and should not route on its own. A comment that belongs to a submitted review
// (pull_request_review_id set) is suppressed UNLESS it is a reply in an existing
// thread (in_reply_to_id set), which is conversational back-and-forth the agent
// must still see. The review turn re-fetches every comment from the API, so a
// suppressed comment is never permanently dropped as long as its review arrives.
func suppressReviewBoundComment(event prRouteEvent) bool {
	if strings.TrimSpace(event.InReplyToID) != "" {
		return false
	}
	return strings.TrimSpace(event.ReviewID) != ""
}
