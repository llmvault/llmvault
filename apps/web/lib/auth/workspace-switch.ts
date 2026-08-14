import type { QueryClient } from "@tanstack/react-query"
import { queryKeys } from "@/lib/api/query-keys"
import { clearSessionSandboxAccess } from "@/app/w/(chat)/_lib/session-sandbox-access"
import {
  resumeSessionConnectionsForOrg,
  suspendSessionConnectionsForOrg,
} from "@/app/w/(chat)/_stores/session-stream-manager"

const ACTIVE_ORG_COOKIE = "hivy_active_org"

interface WorkspaceSwitchOptions {
  previousOrgId?: string | null
  activate?: () => void
}

export function getActiveOrgIdFromCookie(): string | null {
  if (typeof document === "undefined") return null
  const match = document.cookie.match(
    new RegExp(`(?:^|; )${ACTIVE_ORG_COOKIE}=([^;]+)`)
  )
  return match ? decodeURIComponent(match[1]) : null
}

function setActiveOrgCookie(orgId: string) {
  if (typeof document === "undefined") return
  document.cookie = `${ACTIVE_ORG_COOKIE}=${encodeURIComponent(orgId)}; path=/; max-age=${60 * 60 * 24 * 365}; samesite=lax`
}

export function isAuthQueryKey(queryKey: readonly unknown[]) {
  const authKey = queryKeys.authMe()
  return queryKey[0] === authKey[0] && queryKey[1] === authKey[1]
}

export async function switchActiveOrg(
  queryClient: QueryClient,
  orgId: string | undefined | null,
  options: WorkspaceSwitchOptions = {}
): Promise<void> {
  if (!orgId) return

  await queryClient.cancelQueries()
  if (options.previousOrgId && options.previousOrgId !== orgId) {
    suspendSessionConnectionsForOrg(options.previousOrgId)
  }
  clearSessionSandboxAccess()
  setActiveOrgCookie(orgId)
  options.activate?.()

  queryClient.removeQueries({
    predicate: (query) => !isAuthQueryKey(query.queryKey),
  })
  await queryClient.invalidateQueries()
  resumeSessionConnectionsForOrg(orgId, queryClient)
}
