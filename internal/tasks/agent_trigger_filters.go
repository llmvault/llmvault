package tasks

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

func (h *AgentTriggerDispatchHandler) shouldSkipTriggerDelivery(ctx context.Context, payload AgentTriggerDispatchPayload, webhookPayload map[string]any) (bool, string, error) {
	if !isGitHubCheckSuiteCompleted(payload) {
		return false, "", nil
	}
	if !githubCheckSuiteHasPullRequest(webhookPayload) {
		return true, "check_suite has no pull request", nil
	}
	if h == nil || h.db == nil || h.nangoClient == nil {
		return false, "", fmt.Errorf("github check suite trigger filter missing required dependencies")
	}
	authored, err := h.githubCheckSuitePullRequestCreatedByHivy(ctx, payload, webhookPayload)
	if err != nil {
		return false, "", err
	}
	if !authored {
		return true, "check_suite pull request was not created by Hivy", nil
	}
	return false, "", nil
}

func isGitHubCheckSuiteCompleted(payload AgentTriggerDispatchPayload) bool {
	// Primary-app only: check_suite is a build-CI signal. The code-reviews app's
	// copy of the same event carries no routing policy here.
	if !isGitHubPrimary(payload.Provider) {
		return false
	}
	return eventKey(payload.EventType, payload.EventAction) == "check_suite.completed"
}

func (h *AgentTriggerDispatchHandler) githubCheckSuitePullRequestCreatedByHivy(ctx context.Context, payload AgentTriggerDispatchPayload, webhookPayload map[string]any) (bool, error) {
	owner, ok := lookupTriggerPayloadPath(webhookPayload, "repository.owner.login")
	if !ok {
		return false, nil
	}
	repo, ok := lookupTriggerPayloadPath(webhookPayload, "repository.name")
	if !ok {
		return false, nil
	}
	prNumber, ok := lookupTriggerPayloadPath(webhookPayload, "check_suite.pull_requests.0.number")
	if !ok {
		return false, nil
	}

	var conn model.Connection
	if err := h.db.WithContext(ctx).
		Preload("Integration").
		Where("id = ? AND org_id = ? AND revoked_at IS NULL", payload.ConnectionID, payload.OrgID).
		First(&conn).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("load github connection for check suite trigger: %w", err)
	}

	path := fmt.Sprintf("/repos/%s/%s/pulls/%s",
		url.PathEscape(scalarString(owner)),
		url.PathEscape(scalarString(repo)),
		url.PathEscape(scalarString(prNumber)),
	)
	resp, err := h.nangoClient.ProxyRequest(ctx, http.MethodGet, conn.Integration.UniqueKey, conn.NangoConnectionID, path, nil, nil)
	if err != nil {
		return false, fmt.Errorf("fetch check suite pull request author: %w", err)
	}
	// The PR must have been opened by THIS app's bot for its check_suite to route
	// into a build session. Match the connection's exact bot handle rather than a
	// fuzzy "hivy" substring, so a code-reviews-authored PR (were it ever primary)
	// is not mistaken for a build PR.
	author, ok := lookupTriggerPayloadPath(resp, "user.login")
	return ok && githubLoginMatchesHandle(scalarString(author), conn.Integration.BotHandle), nil
}

func githubCheckSuiteHasPullRequest(payload map[string]any) bool {
	value, ok := lookupTriggerPayloadPath(payload, "check_suite.pull_requests.0.number")
	return ok && strings.TrimSpace(scalarString(value)) != ""
}

func lookupTriggerPayloadPath(payload map[string]any, path string) (any, bool) {
	if path == "" {
		return nil, false
	}
	segments := strings.Split(path, ".")
	var current any = payload
	for _, segment := range segments {
		switch typed := current.(type) {
		case map[string]any:
			next, ok := typed[segment]
			if !ok {
				return nil, false
			}
			current = next
		case []any:
			index := -1
			for _, ch := range segment {
				if ch < '0' || ch > '9' {
					return nil, false
				}
				if index < 0 {
					index = 0
				}
				index = index*10 + int(ch-'0')
			}
			if index < 0 || index >= len(typed) {
				return nil, false
			}
			current = typed[index]
		default:
			return nil, false
		}
	}
	return current, true
}

func scalarString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case nil:
		return ""
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}
