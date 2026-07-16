import type { components } from "@/lib/api/schema"

export type SystemCredential = Required<
  components["schemas"]["credentialResponse"]
>
export type LLMProvider = Required<
  components["schemas"]["adminLLMProviderResponse"]
>

export const emptyCredentialForm = {
  provider_id: "",
  label: "",
  base_url: "",
  auth_scheme: "bearer",
  api_key: "",
}

export type CredentialForm = typeof emptyCredentialForm
