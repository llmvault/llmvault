import type { components } from "@/lib/api/schema"

type TeamConnection = components["schemas"]["teamConnectionResponse"]
type AvailableResource = components["schemas"]["AvailableResource"]

export const SLACK_CHANNEL_RESOURCE_TYPE = "slack_channel"

export function slackRouteSummary(
  workspaceName: string,
  channelName: string,
  agentName: string
): string {
  const channel = channelName.replace(/^#/, "")
  return `Any @hivy pings in Slack workspace ${workspaceName}, channel #${channel}, will be routed to ${agentName}.`
}

export function slackConnectionsForRouting(
  connections: readonly TeamConnection[]
): TeamConnection[] {
  return connections
    .filter(
      (connection) =>
        connection.id &&
        connection.kind === "integration" &&
        connection.provider === "slack"
    )
    .sort((left, right) =>
      slackConnectionName(left).localeCompare(slackConnectionName(right))
    )
}

export function slackChannelsForRouting(
  resources: readonly AvailableResource[]
): AvailableResource[] {
  const channels = new Map<string, AvailableResource>()
  for (const resource of resources) {
    if (
      !resource.id ||
      (resource.type && resource.type !== SLACK_CHANNEL_RESOURCE_TYPE)
    ) {
      continue
    }
    channels.set(resource.id, resource)
  }
  return [...channels.values()].sort((left, right) =>
    slackChannelName(left).localeCompare(slackChannelName(right))
  )
}

export function slackConnectionName(connection: TeamConnection): string {
  return connection.name || connection.id || ""
}

export function slackChannelName(resource: AvailableResource): string {
  return resource.name || resource.id || ""
}
