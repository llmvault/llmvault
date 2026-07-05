import type { components } from "@/lib/api/schema"

export type Connection = components["schemas"]["connectionResponse"]
export type AvailableResource = components["schemas"]["AvailableResource"]

export const slackReactionKey = "reaction_added"
export const slackChannelResourceType = "slack_channel"
export const githubIssueMentionKey = "issue_mention"
export const githubPrMentionKey = "pr_mention"
export const githubRepoResourceType = "repository"

// The GitHub mention keys (issue-only and pull-request-only) share the same
// repo-scoped install form.
export function isGithubMentionKey(key: string): boolean {
  return key === githubIssueMentionKey || key === githubPrMentionKey
}

export function triggerSourceSlug(
  provider: string,
  key: string,
  resourceKey: string,
  value: string
): string {
  return [provider, key, resourceKey, value].join(":")
}
