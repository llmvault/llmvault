type ExternalSessionInput = {
  source?: string | null
  sourceResourceKey?: string | null
  source_resource_key?: string | null
}

export type ExternalSessionContinuation = {
  provider: string
  providerLabel: string
  url?: string
}

export function externalSessionContinuation(
  session: ExternalSessionInput
): ExternalSessionContinuation | null {
  if ((session.source ?? "").trim().toLowerCase() !== "external") return null

  const sourceKey =
    session.sourceResourceKey ?? session.source_resource_key ?? ""
  const parts = sourceKey.split(":")
  const provider = (parts[0] || "external").trim().toLowerCase()
  const providerLabel = externalProviderLabel(provider)
  const url = provider === "slack" ? slackThreadURL(parts) : undefined

  return { provider, providerLabel, url }
}

export function externalProviderLabel(provider: string) {
  switch (provider.trim().toLowerCase()) {
    case "slack":
      return "Slack"
    case "microsoft-teams":
    case "msteams":
    case "teams":
      return "Microsoft Teams"
    case "":
    case "external":
      return "external provider"
    default:
      return provider.trim()
  }
}

function slackThreadURL(parts: string[]) {
  if (parts.length < 5) return undefined
  const teamID = parts[2]?.trim()
  const channelID = parts[3]?.trim()
  const threadTS = parts.slice(4).join(":").trim()
  if (!teamID || !channelID || !threadTS) return undefined

  const messageID = `p${threadTS.replaceAll(".", "")}`
  return `https://app.slack.com/client/${encodeURIComponent(teamID)}/${encodeURIComponent(channelID)}/${encodeURIComponent(messageID)}`
}
