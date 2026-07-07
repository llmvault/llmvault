package handler

import (
	"encoding/json"
	"fmt"
	"strings"
)

func unwrapNangoPayload(payload json.RawMessage) ([]byte, map[string]string) {
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(payload, &probe); err != nil {
		return payload, nil
	}
	dataField, hasData := probe["data"]
	headersField, hasHeaders := probe["headers"]
	if !hasData || !hasHeaders {
		return payload, nil
	}
	headers := make(map[string]string)
	var headerProbe map[string]any
	if err := json.Unmarshal(headersField, &headerProbe); err == nil {
		for key, value := range headerProbe {
			if str, ok := value.(string); ok {
				headers[strings.ToLower(key)] = str
			}
		}
	}
	return dataField, headers
}

func inferEventFromHeaders(provider string, headers map[string]string) (eventType, eventAction string) {
	if len(headers) == 0 {
		return "", ""
	}
	switch {
	case provider == "github" || strings.HasPrefix(provider, "github"):
		eventType = headers["x-github-event"]
	}
	return eventType, ""
}

// stableGitHubDeliveryID derives a dedup key from the payload for events
// whose (type, action) fires exactly once per object. GitHub's delivery GUID
// lives in an HTTP header that is not guaranteed to survive Nango's webhook
// forwarding, so these payload keys are the reliable fallback. Actions that
// can recur on the same object (edited, synchronize, labeled, ...) must NOT
// be added here — a payload key would wrongly collapse distinct events.
//
// check_suite.completed is the one recurring exception: a re-run completes the
// SAME suite id again, so the key includes the conclusion — a re-run that
// changes the outcome re-delivers, identical redeliveries dedup.
func stableGitHubDeliveryID(eventType, eventAction string, body []byte) string {
	key := eventType + "." + eventAction
	var idPath string
	switch key {
	case "issue_comment.created":
		idPath = "comment"
	case "issues.opened":
		idPath = "issue"
	case "pull_request.opened":
		idPath = "pull_request"
	case "pull_request_review.submitted":
		idPath = "review"
	case "pull_request_review_comment.created":
		idPath = "comment"
	case "check_suite.completed":
		idPath = "check_suite"
	default:
		return ""
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(body, &probe); err != nil {
		return ""
	}
	raw, ok := probe[idPath]
	if !ok {
		return ""
	}
	var obj struct {
		ID         int64  `json:"id"`
		Conclusion string `json:"conclusion"`
	}
	if err := json.Unmarshal(raw, &obj); err != nil || obj.ID == 0 {
		return ""
	}
	if key == "check_suite.completed" {
		return fmt.Sprintf("github:%s:%d:%s", key, obj.ID, obj.Conclusion)
	}
	return fmt.Sprintf("github:%s:%d", key, obj.ID)
}

func inferGitHubEventFromPayload(body []byte) (eventType, eventAction string) {
	var probe struct {
		Action      string          `json:"action"`
		Issue       json.RawMessage `json:"issue"`
		Comment     json.RawMessage `json:"comment"`
		PullRequest json.RawMessage `json:"pull_request"`
		Review      json.RawMessage `json:"review"`
		WorkflowRun json.RawMessage `json:"workflow_run"`
		WorkflowJob json.RawMessage `json:"workflow_job"`
		CheckRun    json.RawMessage `json:"check_run"`
		CheckSuite  json.RawMessage `json:"check_suite"`
		Release     json.RawMessage `json:"release"`
		Discussion  json.RawMessage `json:"discussion"`
		Deployment  json.RawMessage `json:"deployment"`
		Ref         string          `json:"ref"`
		RefType     string          `json:"ref_type"`
		Before      string          `json:"before"`
		After       string          `json:"after"`
	}
	if err := json.Unmarshal(body, &probe); err != nil {
		return "", ""
	}
	eventAction = probe.Action

	switch {
	case len(probe.WorkflowJob) > 0:
		return "workflow_job", eventAction
	case len(probe.WorkflowRun) > 0:
		return "workflow_run", eventAction
	case len(probe.CheckRun) > 0:
		return "check_run", eventAction
	case len(probe.CheckSuite) > 0:
		return "check_suite", eventAction
	case len(probe.Review) > 0 && len(probe.PullRequest) > 0:
		return "pull_request_review", eventAction
	case len(probe.PullRequest) > 0 && len(probe.Comment) > 0:
		return "pull_request_review_comment", eventAction
	case len(probe.Comment) > 0 && len(probe.Issue) > 0:
		return "issue_comment", eventAction
	case len(probe.Issue) > 0:
		return "issues", eventAction
	case len(probe.PullRequest) > 0:
		return "pull_request", eventAction
	case len(probe.Release) > 0:
		return "release", eventAction
	case len(probe.Discussion) > 0 && len(probe.Comment) > 0:
		return "discussion_comment", eventAction
	case len(probe.Discussion) > 0:
		return "discussion", eventAction
	case len(probe.Deployment) > 0:
		return "deployment_status", eventAction
	case probe.Before != "" && probe.After != "":
		return "push", ""
	case probe.RefType != "" && eventAction == "":
		return "", ""
	}
	return "", ""
}
