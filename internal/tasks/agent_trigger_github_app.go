package tasks

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

// The two surviving GitHub App providers. Both apps are installed on the same
// repositories and therefore receive the SAME webhooks (two copies, one per
// app). Routing POLICY differs by app: only the primary app auto-routes PR
// events into the build session that opened the PR, and mention matching is
// per-app exact against the app's own bot handle.
const (
	githubPrimaryProvider     = "github-app"              // bot login "usehivy"
	githubCodeReviewsProvider = "github-app-code-reviews" // bot login "usehivy-reviews"
)

// isGitHubPrimary reports whether the provider is the primary GitHub App, the
// one whose PR events auto-route into build sessions and whose mentions ack with
// an eyes reaction from the auto-route path.
func isGitHubPrimary(provider string) bool {
	return provider == githubPrimaryProvider
}

// isGitHubCodeReviews reports whether the provider is the code-reviews GitHub
// App. Its mention triggers always create/reuse their own review session and
// never hijack the build agent's session.
func isGitHubCodeReviews(provider string) bool {
	return provider == githubCodeReviewsProvider
}

// isGitHubPROpenedTrigger reports whether the trigger is the curated
// auto-review trigger (fire on pull_request.opened, no mention required). It is
// code-reviews-only: only the github-app-code-reviews app opens review sessions
// on every PR, and binding it to the primary app would double-fire review runs
// against build sessions. Kept separate from isGitHubMentionTrigger so the
// mention guard ladder (which requires an @handle) never runs for it.
func isGitHubPROpenedTrigger(provider string, trigger model.AgentTrigger) bool {
	return trigger.TriggerKey == model.TriggerKeyGitHubPROpened && isGitHubCodeReviews(provider)
}

// githubHivyBotHandles are Hivy's two surviving GitHub App bot logins,
// normalized ([bot] stripped, lowercased). Used only as a static loop guard —
// isGitHubBotAuthor already skips every bot, so this is defense in depth against
// the two Hivy identities pinging each other through mentions.
var githubHivyBotHandles = []string{"usehivy", "usehivy-reviews"}

// isGitHubHivyBotLogin reports whether a webhook author is one of Hivy's own
// GitHub App bots, by exact normalized handle. Unlike the removed
// isHivyIdentityValue it does not substring-match "hivy", so unrelated accounts
// that merely contain "hivy" are never treated as Hivy.
func isGitHubHivyBotLogin(login string) bool {
	n := normalizeGitHubLogin(login)
	if n == "" {
		return false
	}
	for _, handle := range githubHivyBotHandles {
		if n == handle {
			return true
		}
	}
	return false
}

// githubTextMentionsHandle reports whether the text @mentions the given bot
// handle as a full handle token, case-insensitively. An empty handle never
// matches — a misconfigured integration (no bot handle) must not fire triggers.
// This replaces the fuzzy contains-"hivy" match: "@usehivy-reviews" no longer
// fires the primary app's triggers, and "@usehivy" no longer fires the
// code-reviews app's triggers.
func githubTextMentionsHandle(text, handle string) bool {
	want := normalizeGitHubLogin(handle)
	if want == "" {
		return false
	}
	for _, match := range githubHandlePattern.FindAllStringSubmatch(text, -1) {
		if strings.EqualFold(match[1], want) {
			return true
		}
	}
	return false
}

// githubLoginMatchesHandle reports whether a webhook login equals a bot handle,
// normalizing both (lowercase, [bot] suffix stripped). An empty handle never
// matches.
func githubLoginMatchesHandle(login, handle string) bool {
	want := normalizeGitHubLogin(handle)
	return want != "" && normalizeGitHubLogin(login) == want
}

// githubConnectionBotHandle loads the delivering connection's integration bot
// handle (e.g. "usehivy" or "usehivy-reviews"). It is best-effort: a nil db, a
// missing/revoked connection, or an empty handle all return "". Callers decide
// what "" means: mention matching treats it as no-match + warn (misconfigured
// integration), while the auto-route self-guard treats it as "cannot be self".
func (h *AgentTriggerDispatchHandler) githubConnectionBotHandle(ctx context.Context, payload AgentTriggerDispatchPayload) string {
	if h == nil || h.db == nil {
		return ""
	}
	var conn model.Connection
	if err := h.db.WithContext(ctx).
		Preload("Integration").
		Where("id = ? AND org_id = ? AND revoked_at IS NULL", payload.ConnectionID, payload.OrgID).
		First(&conn).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			logging.CaptureWithFields(ctx, fmt.Errorf("github: load connection bot handle: %w", err), map[string]any{
				"org_id": payload.OrgID.String(),
			})
		}
		return ""
	}
	return conn.Integration.BotHandle
}
