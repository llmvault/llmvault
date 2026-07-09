"use client"

import {
  createContext,
  useContext,
  useCallback,
  useState,
  useEffect,
  useRef,
} from "react"
import { useRouter } from "next/navigation"
import { useQueryClient, type QueryClient } from "@tanstack/react-query"
import { $api } from "@/lib/api/hooks"
import { queryKeys } from "@/lib/api/query-keys"
import type { components } from "@/lib/api/schema"
import { clearSessionSandboxAccess } from "@/app/w/(chat)/_lib/session-sandbox-access"
import {
  stopAllSessionNotices,
  stopAllSessionStreams,
} from "@/app/w/(chat)/_stores/session-stream-manager"
import { clearPersistedSessionWorkspaces } from "@/app/w/(chat)/_stores/session-workspace-store"

type User = components["schemas"]["userResponse"]
type Org = components["schemas"]["orgMemberDTO"]
type Plan = components["schemas"]["planDTO"]

const ACTIVE_ORG_COOKIE = "hivy_active_org"

function getOrgIdFromCookie(): string | null {
  if (typeof document === "undefined") return null
  const match = document.cookie.match(
    new RegExp(`(?:^|; )${ACTIVE_ORG_COOKIE}=([^;]+)`)
  )
  return match ? decodeURIComponent(match[1]) : null
}

function setOrgIdCookie(orgId: string) {
  document.cookie = `${ACTIVE_ORG_COOKIE}=${encodeURIComponent(orgId)}; path=/; max-age=${60 * 60 * 24 * 365}; samesite=lax`
}

/**
 * Single writer for the active-org switch. auth-context owns the org cookie —
 * nothing else should ever write `hivy_active_org` directly. Writes the cookie
 * and does a FULL cache invalidation so no query from the previous org survives
 * (query keys are not org-scoped). Callers that live outside AuthProvider
 * (e.g. the invite-accept page) use this helper; callers inside use
 * `setActiveOrg` from `useAuth`, which delegates here.
 */
export async function switchActiveOrg(
  queryClient: QueryClient,
  orgId: string | undefined | null
): Promise<void> {
  if (!orgId) return
  setOrgIdCookie(orgId)
  await queryClient.invalidateQueries()
}

interface AuthContextValue {
  user: User | null
  orgs: Org[]
  activeOrg: Org | null
  plans: Plan[]
  setActiveOrg: (org: Org) => void
  addOrg: (org: Org) => void
  logout: () => Promise<void>
  isLoading: boolean
}

const AuthContext = createContext<AuthContextValue | null>(null)

export function AuthProvider({
  children,
  signInPath = "/auth/login",
}: {
  children: React.ReactNode
  signInPath?: string
}) {
  const router = useRouter()
  const queryClient = useQueryClient()
  const meQuery = $api.useQuery("get", "/auth/me", {}, { retry: false })
  const plansQuery = $api.useQuery("get", "/v1/plans", {}, { retry: false })
  const logoutMutation = $api.useMutation("post", "/auth/logout")
  const hasRedirected = useRef(false)

  const data = meQuery.data
  const isError = meQuery.isError
  const isLoading = meQuery.isLoading || plansQuery.isLoading

  const user = (data?.user as User) ?? null
  const orgs = (data?.orgs as Org[]) ?? []
  const plans = (plansQuery.data as Plan[] | undefined) ?? []

  const [activeOrgId, setActiveOrgId] = useState<string | null>(() =>
    getOrgIdFromCookie()
  )

  const activeOrg =
    orgs.find((org) => org.id === activeOrgId) ?? orgs[0] ?? null

  useEffect(() => {
    if (isError && !hasRedirected.current) {
      hasRedirected.current = true
      router.replace(signInPath)
    }
  }, [isError, router, signInPath])

  useEffect(() => {
    const nextOrgId = activeOrg?.id
    if (nextOrgId && nextOrgId !== activeOrgId) {
      queueMicrotask(() => setActiveOrgId(nextOrgId))
      setOrgIdCookie(nextOrgId)
    }
  }, [activeOrg?.id, activeOrgId])

  const setActiveOrg = useCallback(
    (org: Org) => {
      if (org.id) {
        setActiveOrgId(org.id)
        void switchActiveOrg(queryClient, org.id)
      }
    },
    [queryClient]
  )

  const addOrg = useCallback(
    (org: Org) => {
      queryClient.invalidateQueries({ queryKey: queryKeys.authMe() })
      if (org.id) {
        setActiveOrgId(org.id)
        setOrgIdCookie(org.id)
      }
    },
    [queryClient]
  )

  const logout = useCallback(async () => {
    await logoutMutation.mutateAsync({ body: {} })
    stopAllSessionStreams()
    stopAllSessionNotices()
    clearSessionSandboxAccess()
    queryClient.clear()
    await clearPersistedSessionWorkspaces()
    router.replace(signInPath)
  }, [logoutMutation, queryClient, router, signInPath])

  return (
    <AuthContext.Provider
      value={{
        user,
        orgs,
        activeOrg,
        plans,
        setActiveOrg,
        addOrg,
        logout,
        isLoading,
      }}
    >
      {children}
    </AuthContext.Provider>
  )
}

export function useAuth() {
  const ctx = useContext(AuthContext)
  if (!ctx) throw new Error("useAuth must be used within AuthProvider")
  return ctx
}
