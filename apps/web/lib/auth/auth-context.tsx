"use client"

import posthog from "posthog-js"
import {
  createContext,
  useContext,
  useCallback,
  useState,
  useEffect,
  useRef,
} from "react"
import { useRouter } from "next/navigation"
import { useQueryClient } from "@tanstack/react-query"
import { $api } from "@/lib/api/hooks"
import { queryKeys } from "@/lib/api/query-keys"
import type { components } from "@/lib/api/schema"
import { clearSessionSandboxAccess } from "@/app/w/(chat)/_lib/session-sandbox-access"
import {
  stopAllSessionNotices,
  stopAllSessionStreams,
} from "@/app/w/(chat)/_stores/session-stream-manager"
import { clearPersistedSessionWorkspaces } from "@/app/w/(chat)/_stores/session-workspace-store"
import {
  getActiveOrgIdFromCookie,
  switchActiveOrg,
} from "@/lib/auth/workspace-switch"

export { switchActiveOrg } from "@/lib/auth/workspace-switch"

type User = components["schemas"]["userResponse"]
type Org = components["schemas"]["orgMemberDTO"]

/**
 * auth-context remains the UI owner of active-org state. The transition helper
 * owns the cookie, query isolation, and resumable live-session handoff.
 */
interface AuthContextValue {
  user: User | null
  orgs: Org[]
  activeOrg: Org | null
  setActiveOrg: (org: Org) => Promise<void>
  addOrg: (org: Org) => Promise<void>
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
  const logoutMutation = $api.useMutation("post", "/auth/logout")
  const hasRedirected = useRef(false)

  const data = meQuery.data
  const isError = meQuery.isError
  const isLoading = meQuery.isLoading

  const user = (data?.user as User) ?? null
  const orgs = (data?.orgs as Org[]) ?? []

  const [activeOrgId, setActiveOrgId] = useState<string | null>(() =>
    getActiveOrgIdFromCookie()
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
    if (user?.id) {
      posthog.identify(user.id)
    }
  }, [user?.id])

  useEffect(() => {
    const nextOrgId = activeOrg?.id
    if (nextOrgId && nextOrgId !== activeOrgId) {
      void switchActiveOrg(queryClient, nextOrgId, {
        previousOrgId: activeOrgId,
        activate: () => setActiveOrgId(nextOrgId),
      })
    }
  }, [activeOrg?.id, activeOrgId, queryClient])

  const setActiveOrg = useCallback(
    async (org: Org) => {
      if (!org.id || org.id === activeOrg?.id) return
      router.replace("/w")
      await switchActiveOrg(queryClient, org.id, {
        previousOrgId: activeOrg?.id,
        activate: () => setActiveOrgId(org.id ?? null),
      })
    },
    [activeOrg?.id, queryClient, router]
  )

  const addOrg = useCallback(
    async (org: Org) => {
      if (!org.id) return
      await queryClient.invalidateQueries({ queryKey: queryKeys.authMe() })
      router.replace("/w")
      await switchActiveOrg(queryClient, org.id, {
        previousOrgId: activeOrg?.id,
        activate: () => setActiveOrgId(org.id ?? null),
      })
    },
    [activeOrg?.id, queryClient, router]
  )

  const logout = useCallback(async () => {
    await logoutMutation.mutateAsync({ body: {} })
    posthog.reset()
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
