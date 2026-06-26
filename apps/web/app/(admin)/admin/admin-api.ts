import { extractErrorMessage } from "@/lib/api/error"

export const ADMIN_QUERY_KEYS = {
  integrations: ["get", "/v1/admin/integrations"] as const,
  providers: ["get", "/v1/admin/llm-providers"] as const,
  systemCredentials: ["get", "/v1/admin/system-credentials"] as const,
}

export const adminSecretHeader = (secret: string) => ({
  "X-Hivy-Admin-Secret": secret,
})

export function errorMessage(error: unknown, fallback: string) {
  return extractErrorMessage(error, fallback)
}
