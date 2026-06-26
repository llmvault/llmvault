import type { components } from "@/lib/api/schema"

export type AdminIntegrationDefinition =
  components["schemas"]["AdminDefinition"]
export type AdminCredentialField = Required<
  components["schemas"]["AdminCredentialField"]
>
export type SystemCredential = Required<
  components["schemas"]["credentialResponse"]
>
export type LLMProvider = Required<
  components["schemas"]["adminLLMProviderResponse"]
>

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
