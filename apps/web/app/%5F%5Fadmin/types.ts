export type AdminIntegrationDefinition = {
  id?: string
  provider?: string
  nango_provider?: string
  unique_key?: string
  display_name?: string
  enabled?: boolean
  required?: boolean
  supports_rag_source?: boolean
  auth_mode?: string
  credential_fields?: AdminCredentialField[]
  fixed_credentials?: Array<{ name: string; label: string; value: string }>
  existing?: {
    id?: string
    unique_key?: string
    display_name?: string
    managed?: boolean
    active_connections?: number
    updated_at?: string
  } | null
}

export type AdminCredentialField = {
  name: string
  label: string
  required: boolean
  secret?: boolean
  multiline?: boolean
  placeholder?: string
}

export type SystemCredential = {
  id: string
  label?: string
  base_url: string
  auth_scheme: string
  provider_id?: string
  remaining?: number | null
  refill_amount?: number | null
  refill_interval?: string | null
  created_at: string
  revoked_at?: string | null
}

export type LLMProvider = {
  id: string
  name: string
  base_url?: string
  default_auth_scheme: string
  model_ids?: string[]
}

export type PageResponse<T> = {
  data?: T[]
  has_more?: boolean
  next_cursor?: string
}

export type LoadState = "idle" | "loading" | "ready" | "error"
export type AdminTab = "integrations" | "credentials"

export const emptyCredentialForm = {
  provider_id: "",
  label: "",
  base_url: "",
  auth_scheme: "bearer",
  api_key: "",
}

export type CredentialForm = typeof emptyCredentialForm
