import createClient from "openapi-fetch"
import { clientConfig } from "@/lib/config/public-config"
import type { paths } from "./schema"

export function apiUrl(path: string = ""): string {
  const base = clientConfig().apiUrl
  return `${base}${path}`
}

export const api = createClient<paths>({
  baseUrl: "/api/proxy",
})
