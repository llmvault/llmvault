import type { components } from "@/lib/api/schema"

export type AvailableIntegration =
  components["schemas"]["integrationAvailableResponse"]

type ConnectionConfigField = components["schemas"]["ConnectionConfigField"]

export interface IntegrationConnectionField {
  key: string
  title: string
  description: string
  placeholder: string
  pattern?: string
}

export function integrationNeedsForm(
  integration: AvailableIntegration
): boolean {
  const config = integration.nango_config
  const authMode = config?.auth_mode
  const installation = config?.installation

  if (authMode === "API_KEY" || authMode === "BASIC") return true
  if (installation === "outbound") return true

  return integrationConnectionFields(integration).length > 0
}

export function integrationConnectionFields(
  integration: AvailableIntegration
): IntegrationConnectionField[] {
  const connectionConfig = integration.nango_config?.connection_config as
    | Record<string, ConnectionConfigField>
    | undefined
  if (!connectionConfig) return []

  return Object.entries(connectionConfig)
    .filter(([, field]) => !field.automated)
    .map(([key, field]) => ({
      key,
      title: field.title || key,
      description: field.description || "",
      placeholder: field.example || "",
      pattern: field.pattern,
    }))
}
