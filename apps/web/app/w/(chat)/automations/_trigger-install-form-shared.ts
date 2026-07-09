import type { components } from "@/lib/api/schema"

type Connection = components["schemas"]["connectionResponse"]
export type AvailableResource = components["schemas"]["AvailableResource"]

export const slackReactionKey = "reaction_added"
export const slackChannelResourceType = "slack_channel"
const githubIssueMentionKey = "issue_mention"
export const githubPrMentionKey = "pr_mention"
// The code-reviews app also auto-reviews every new pull request (no mention).
export const githubPrOpenedKey = "pr_opened"
export const githubRepoResourceType = "repository"

// The primary GitHub App and the code-reviews GitHub App bind to distinct
// connection providers. Both drive repo-scoped mention install forms.
export const githubAppProvider = "github-app"
export const githubCodeReviewsProvider = "github-app-code-reviews"

// The GitHub mention keys (issue-only and pull-request-only) share the same
// repo-scoped install form.
export function isGithubMentionKey(key: string): boolean {
  return key === githubIssueMentionKey || key === githubPrMentionKey
}

// isGithubMentionProvider covers both GitHub App connection providers whose
// mention triggers render with the shared GitHub mention presentation.
export function isGithubMentionProvider(provider: string): boolean {
  return (
    provider === githubAppProvider || provider === githubCodeReviewsProvider
  )
}

export function triggerSourceSlug(
  provider: string,
  key: string,
  resourceKey: string,
  value: string
): string {
  return [provider, key, resourceKey, value].join(":")
}
