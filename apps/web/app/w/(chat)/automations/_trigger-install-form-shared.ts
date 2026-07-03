import type { components } from "@/lib/api/schema"

export type Connection = components["schemas"]["connectionResponse"]
export type AvailableResource = components["schemas"]["AvailableResource"]

export const slackReactionKey = "reaction_added"
export const slackChannelResourceType = "slack_channel"
export const githubMentionKey = "mention"
export const githubRepoResourceType = "repository"

export function triggerSourceSlug(
  provider: string,
  key: string,
  resourceKey: string,
  value: string
): string {
  return [provider, key, resourceKey, value].join(":")
}
