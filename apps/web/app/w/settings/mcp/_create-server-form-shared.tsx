import type { CreateMcpServerInput, McpServerScope } from "./_lib/mcp-api"

export interface CreateServerSubmission {
  server: CreateMcpServerInput
  startOAuth: boolean
  oauthRegistration?: {
    clientId?: string
    clientSecret?: string
    scopes?: string[]
  }
  oauthPrincipalType?: "user" | "org_service"
}

export interface CreateServerFormProps {
  initialScope: McpServerScope
  isAdmin: boolean
  isPending: boolean
  onCancel: () => void
  onCreate: (submission: CreateServerSubmission) => Promise<void>
}
