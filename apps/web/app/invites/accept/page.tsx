"use client"

import * as React from "react"
import { Suspense, useCallback, useEffect, useMemo, useRef, useState } from "react"
import { useRouter, useSearchParams } from "next/navigation"
import { useQueryClient } from "@tanstack/react-query"
import { Button, Chip, Spinner, toast } from "@heroui/react"
import { Logo } from "@/components/logo"
import { $api } from "@/lib/api/hooks"
import { switchActiveOrg } from "@/lib/auth/auth-context"
import { extractErrorMessage } from "@/lib/api/error"
import { localPart } from "@/lib/email"

function CenterCard({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex min-h-screen items-center justify-center bg-background px-4 py-10">
      <div className="w-full max-w-md">
        <div className="mb-6 flex justify-center">
          <Logo />
        </div>
        <div className="rounded-2xl border border-border bg-surface p-6 shadow-sm">
          {children}
        </div>
      </div>
    </div>
  )
}

function AcceptInviteContents() {
  const searchParams = useSearchParams()
  const router = useRouter()
  const token = searchParams.get("token") ?? ""
  const autoAccept = searchParams.get("auto") === "1"
  const autoAcceptStarted = useRef(false)

  // Invite preview (public)
  const previewQuery = $api.useQuery(
    "get",
    "/v1/invites/{token}",
    { params: { path: { token } } },
    { enabled: token.length > 0, retry: false }
  )

  // Current auth status — no retry, errors are treated as "logged out".
  const meQuery = $api.useQuery("get", "/auth/me", {}, { retry: false })
  const me = meQuery.data

  const acceptMutation = $api.useMutation("post", "/v1/invites/{token}/accept")
  const declineMutation = $api.useMutation("post", "/v1/invites/{token}/decline")
  const logoutMutation = $api.useMutation("post", "/auth/logout")

  const [accepted, setAccepted] = useState<{ orgName: string } | null>(null)
  const [declined, setDeclined] = useState(false)

  const returnPath = useMemo(() => {
    if (!token) return "/invites/accept"
    return `/invites/accept?token=${encodeURIComponent(token)}&auto=1`
  }, [token])
  const signinHref = `/auth/login?next=${encodeURIComponent(returnPath)}`
  const signupHref = `/auth/signup?next=${encodeURIComponent(returnPath)}`

  const queryClient = useQueryClient()
  const setActiveOrgAndRedirect = useCallback(
    async (orgID?: string, fallback = "/w") => {
      // Route through auth-context's single org-switch writer: it sets the
      // active-org cookie and does a full queryClient.invalidateQueries() so
      // the previous org's channels/sessions/agents never leak into the new
      // workspace (query keys aren't org-scoped). Navigate once it completes —
      // no arbitrary setTimeout, no hand-written cookie.
      await switchActiveOrg(queryClient, orgID)
      router.replace(fallback)
    },
    [router, queryClient]
  )

  const handleAccept = useCallback(() => {
    acceptMutation.mutate(
      { params: { path: { token } } },
      {
        onSuccess: (resp) => {
          setAccepted({ orgName: resp?.org_name ?? "" })
          void setActiveOrgAndRedirect(resp?.org_id)
        },
        onError: (error) => {
          toast.danger(extractErrorMessage(error, "Failed to accept invitation"))
        },
      }
    )
  }, [acceptMutation, token, setActiveOrgAndRedirect])

  const handleDecline = useCallback(() => {
    declineMutation.mutate(
      { params: { path: { token } } },
      {
        onSuccess: () => {
          setDeclined(true)
        },
        onError: (error) => {
          toast.danger(extractErrorMessage(error, "Failed to decline invitation"))
        },
      }
    )
  }, [declineMutation, token])

  const handleLogoutAndGoToAuth = useCallback(() => {
    logoutMutation.mutate(
      { body: {} },
      {
        onSettled: () => {
          router.replace(`/auth/login?next=${encodeURIComponent(returnPath)}`)
        },
      }
    )
  }, [logoutMutation, router, returnPath])

  const invite = previewQuery.data
  const inviteEmail = invite?.email ?? ""
  const orgName = invite?.org_name ?? "this workspace"
  const userEmail = (me?.user?.email ?? "").toLowerCase().trim()
  const expected = inviteEmail.toLowerCase().trim()

  useEffect(() => {
    if (!autoAccept || autoAcceptStarted.current || accepted || declined) return
    if (!token || !me?.user || !invite || userEmail !== expected) return

    autoAcceptStarted.current = true
    acceptMutation.mutate(
      { params: { path: { token } } },
      {
        onSuccess: (resp) => {
          setAccepted({ orgName: resp?.org_name ?? "" })
          void setActiveOrgAndRedirect(resp?.org_id)
        },
        onError: (error) => {
          autoAcceptStarted.current = false
          toast.danger(extractErrorMessage(error, "Failed to accept invitation"))
        },
      }
    )
  }, [
    acceptMutation,
    accepted,
    autoAccept,
    declined,
    expected,
    invite,
    me?.user,
    setActiveOrgAndRedirect,
    token,
    userEmail,
  ])

  // --- State rendering ---

  if (!token) {
    return (
      <CenterCard>
        <h1 className="text-lg font-semibold text-foreground">Invalid link</h1>
        <p className="mt-2 text-sm text-muted">
          This invitation link is missing its token.
        </p>
        <div className="mt-6">
          <Button fullWidth onPress={() => router.push("/auth/login")}>
            Go to sign in
          </Button>
        </div>
      </CenterCard>
    )
  }

  if (previewQuery.isLoading || meQuery.isLoading) {
    return (
      <CenterCard>
        <div className="flex flex-col items-center py-10">
          <Spinner size="sm" />
          <p className="mt-3 text-sm text-muted">Loading invitation…</p>
        </div>
      </CenterCard>
    )
  }

  if (previewQuery.isError || !previewQuery.data) {
    return (
      <CenterCard>
        <h1 className="text-lg font-semibold text-foreground">
          Invitation unavailable
        </h1>
        <p className="mt-2 text-sm text-muted">
          This invitation is no longer valid. It may have expired, been revoked,
          or already been used.
        </p>
        <div className="mt-6">
          <Button fullWidth onPress={() => router.push("/auth/login")}>
            Back to sign in
          </Button>
        </div>
      </CenterCard>
    )
  }

  if (declined) {
    return (
      <CenterCard>
        <h1 className="text-lg font-semibold text-foreground">
          Invitation declined
        </h1>
        <p className="mt-2 text-sm text-muted">
          You&apos;ve declined the invitation to {orgName}.
        </p>
        <div className="mt-6">
          <Button fullWidth onPress={() => router.push("/w")}>
            Go to workspace
          </Button>
        </div>
      </CenterCard>
    )
  }

  if (accepted) {
    return (
      <CenterCard>
        <h1 className="text-lg font-semibold text-foreground">
          Welcome to {accepted.orgName || orgName}
        </h1>
        <p className="mt-2 text-sm text-muted">
          You&apos;re now a member. Redirecting you to the workspace…
        </p>
      </CenterCard>
    )
  }

  // Not logged in
  if (meQuery.isError || !me?.user) {
    return (
      <CenterCard>
        <h1 className="text-lg font-semibold text-foreground">
          {invite?.inviter_name ?? "Someone"} invited you to {orgName}
        </h1>
        <p className="mt-2 flex flex-wrap items-center gap-1.5 text-sm text-muted">
          Sent to{" "}
          <span className="font-medium text-foreground">{inviteEmail}</span> ·
          role
          <Chip size="sm">{invite?.role}</Chip>
        </p>
        <div className="mt-6 flex flex-col gap-2">
          <Button fullWidth onPress={() => router.push(signinHref)}>
            Sign in to accept
          </Button>
          <Button
            variant="outline"
            fullWidth
            onPress={() => router.push(signupHref)}
          >
            Create account
          </Button>
        </div>
        <p className="mt-4 text-center text-[11px] text-muted">
          After signing in, you&apos;ll come back here to accept the invitation.
        </p>
      </CenterCard>
    )
  }

  // Logged in, check email match
  if (userEmail !== expected) {
    return (
      <CenterCard>
        <h1 className="text-lg font-semibold text-foreground">Wrong account</h1>
        <p className="mt-2 text-sm text-muted">
          This invite was sent to{" "}
          <span className="font-medium text-foreground">{inviteEmail}</span>.
          You&apos;re signed in as{" "}
          <span className="font-medium text-foreground">{me.user.email}</span>.
        </p>
        <div className="mt-6">
          <Button
            fullWidth
            isPending={logoutMutation.isPending}
            isDisabled={logoutMutation.isPending}
            onPress={handleLogoutAndGoToAuth}
          >
            {logoutMutation.isPending ? (
              <Spinner color="current" size="sm" />
            ) : null}
            Sign out and sign in as {localPart(inviteEmail)}
          </Button>
        </div>
      </CenterCard>
    )
  }

  // Logged in, matching email
  const actionsBusy = acceptMutation.isPending || declineMutation.isPending
  return (
    <CenterCard>
      <h1 className="text-lg font-semibold text-foreground">Join {orgName}</h1>
      <p className="mt-2 flex flex-wrap items-center gap-1.5 text-sm text-muted">
        {invite?.inviter_name ?? "An admin"} invited you to {orgName} as
        <Chip size="sm">{invite?.role}</Chip>
      </p>
      <div className="mt-6 flex flex-col gap-2">
        <Button
          fullWidth
          isPending={acceptMutation.isPending}
          isDisabled={actionsBusy}
          onPress={handleAccept}
        >
          {acceptMutation.isPending ? (
            <Spinner color="current" size="sm" />
          ) : null}
          Join {orgName}
        </Button>
        <Button
          variant="outline"
          fullWidth
          isPending={declineMutation.isPending}
          isDisabled={actionsBusy}
          onPress={handleDecline}
        >
          {declineMutation.isPending ? (
            <Spinner color="current" size="sm" />
          ) : null}
          Decline
        </Button>
      </div>
    </CenterCard>
  )
}

export default function AcceptInvitePage() {
  return (
    <Suspense
      fallback={
        <CenterCard>
          <div className="flex justify-center py-10">
            <Spinner size="sm" />
          </div>
        </CenterCard>
      }
    >
      <AcceptInviteContents />
    </Suspense>
  )
}
