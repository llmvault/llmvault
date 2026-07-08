"use client"

import { useCallback, useState } from "react"
import { useRouter } from "next/navigation"
import { useQueryClient } from "@tanstack/react-query"
import { toast } from "@heroui/react"
import { $api } from "@/lib/api/hooks"
import { queryKeys } from "@/lib/api/query-keys"
import { extractErrorMessage } from "@/lib/api/error"
import type { components } from "@/lib/api/schema"

type LoginRequest = components["schemas"]["loginRequest"]
type RegisterRequest = components["schemas"]["registerRequest"]
type ConfirmEmailRequest = components["schemas"]["confirmEmailRequest"]
type ResendConfirmationRequest =
  components["schemas"]["resendConfirmationRequest"]

export type PasswordAuthInput = Required<
  Pick<LoginRequest, "email" | "password">
>
export type PasswordSignupInput = PasswordAuthInput & { teamName: string }
export type ConfirmEmailInput = Required<
  Pick<ConfirmEmailRequest, "email" | "code">
>

/**
 * Builds the /auth/register body from the signup form fields. Extracted as a
 * pure function so the required team-name plumbing is unit-testable without
 * mounting the hook. `team_name` is required by the backend (empty → 400); we
 * trim it and let the server reject a blank value inline.
 */
export function buildRegisterBody(
  email: string,
  password: string,
  teamName: string
): RegisterRequest {
  const normalizedEmail = normalizeEmail(email)
  return {
    email: normalizedEmail,
    password,
    name: deriveNameFromEmail(normalizedEmail),
    team_name: teamName.trim(),
  }
}

export function safeAuthRedirect(
  rawNext: string | null | undefined,
  fallback = "/w"
) {
  const next = rawNext?.trim()
  if (!next || next.length > 2048) return fallback
  if (!next.startsWith("/") || next.startsWith("//") || /[\r\n]/.test(next)) {
    return fallback
  }
  return next
}

function normalizeEmail(email: string) {
  return email.trim().toLowerCase()
}

function deriveNameFromEmail(email: string) {
  const localPart = email.split("@")[0]?.trim()
  if (!localPart) return "Hivy user"

  return localPart
    .replace(/[._-]+/g, " ")
    .split(" ")
    .filter(Boolean)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(" ")
}

export function usePasswordLogin(nextPath = "/w") {
  const router = useRouter()
  const queryClient = useQueryClient()
  const [emailToConfirm, setEmailToConfirm] = useState<string | null>(null)
  const mutation = $api.useMutation("post", "/auth/login")
  const confirmMutation = $api.useMutation("post", "/auth/confirm-email")
  const resendMutation = $api.useMutation("post", "/auth/resend-confirmation")
  const redirectTo = safeAuthRedirect(nextPath)

  const login = useCallback(
    ({ email, password }: PasswordAuthInput) => {
      const normalizedEmail = normalizeEmail(email)
      if (!normalizedEmail || !password) return

      const body: LoginRequest = {
        email: normalizedEmail,
        password,
      }

      mutation.mutate(
        { body },
        {
          onSuccess: (response) => {
            queryClient.invalidateQueries({ queryKey: queryKeys.authMe() })
            if (response?.user?.email_confirmed === false) {
              setEmailToConfirm(normalizedEmail)
              toast.success("Check your email for a 6-digit confirmation code")
              return
            }
            router.replace(redirectTo)
          },
          onError: (error) => {
            toast.danger(extractErrorMessage(error, "Could not sign in"))
          },
        }
      )
    },
    [mutation, queryClient, redirectTo, router]
  )

  const confirmEmail = useCallback(
    ({ email, code }: ConfirmEmailInput) => {
      const normalizedEmail = normalizeEmail(email)
      const trimmedCode = code.trim()
      if (!normalizedEmail || !trimmedCode) return

      const body: ConfirmEmailRequest = {
        email: normalizedEmail,
        code: trimmedCode,
      }

      confirmMutation.mutate(
        { body },
        {
          onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: queryKeys.authMe() })
            router.replace(redirectTo)
          },
          onError: (error) => {
            toast.danger(extractErrorMessage(error, "Invalid or expired code"))
          },
        }
      )
    },
    [confirmMutation, queryClient, redirectTo, router]
  )

  const resendConfirmation = useCallback(() => {
    if (!emailToConfirm) return

    const body: ResendConfirmationRequest = {
      email: emailToConfirm,
    }

    resendMutation.mutate(
      { body },
      {
        onSuccess: () => {
          toast.success("A new confirmation code has been sent")
        },
        onError: (error) => {
          toast.danger(
            extractErrorMessage(error, "Could not resend confirmation code")
          )
        },
      }
    )
  }, [emailToConfirm, resendMutation])

  const changeEmail = useCallback(() => {
    setEmailToConfirm(null)
  }, [])

  return {
    login,
    confirmEmail,
    resendConfirmation,
    changeEmail,
    emailToConfirm,
    isPending: mutation.isPending,
    isConfirming: confirmMutation.isPending,
    isResending: resendMutation.isPending,
  }
}

export function usePasswordSignup(nextPath = "/w") {
  const router = useRouter()
  const queryClient = useQueryClient()
  const redirectTo = safeAuthRedirect(nextPath)
  const [emailToConfirm, setEmailToConfirm] = useState<string | null>(null)
  const registerMutation = $api.useMutation("post", "/auth/register")
  const confirmMutation = $api.useMutation("post", "/auth/confirm-email")
  const resendMutation = $api.useMutation("post", "/auth/resend-confirmation")

  const signup = useCallback(
    ({ email, password, teamName }: PasswordSignupInput) => {
      const normalizedEmail = normalizeEmail(email)
      if (!normalizedEmail || !password) return

      const body = buildRegisterBody(email, password, teamName)

      registerMutation.mutate(
        { body },
        {
          onSuccess: (response) => {
            queryClient.invalidateQueries({ queryKey: queryKeys.authMe() })
            if (response?.user?.email_confirmed) {
              router.replace(redirectTo)
              return
            }
            setEmailToConfirm(normalizedEmail)
            toast.success("Check your email for a 6-digit confirmation code")
          },
        }
      )
    },
    [queryClient, redirectTo, registerMutation, router]
  )

  const confirmEmail = useCallback(
    ({ email, code }: ConfirmEmailInput) => {
      const normalizedEmail = normalizeEmail(email)
      const trimmedCode = code.trim()
      if (!normalizedEmail || !trimmedCode) return

      const body: ConfirmEmailRequest = {
        email: normalizedEmail,
        code: trimmedCode,
      }

      confirmMutation.mutate(
        { body },
        {
          onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: queryKeys.authMe() })
            router.replace(redirectTo)
          },
          onError: (error) => {
            toast.danger(extractErrorMessage(error, "Invalid or expired code"))
          },
        }
      )
    },
    [confirmMutation, queryClient, redirectTo, router]
  )

  const resendConfirmation = useCallback(() => {
    if (!emailToConfirm) return

    const body: ResendConfirmationRequest = {
      email: emailToConfirm,
    }

    resendMutation.mutate(
      { body },
      {
        onSuccess: () => {
          toast.success("A new confirmation code has been sent")
        },
        onError: (error) => {
          toast.danger(
            extractErrorMessage(error, "Could not resend confirmation code")
          )
        },
      }
    )
  }, [emailToConfirm, resendMutation])

  const changeEmail = useCallback(() => {
    setEmailToConfirm(null)
  }, [])

  // Surfaced inline on the signup form (not a toast) so the required-team_name
  // 400 lands next to the fields the user must fix.
  const signupError = registerMutation.error
    ? extractErrorMessage(registerMutation.error, "Could not create account")
    : null

  return {
    signup,
    confirmEmail,
    resendConfirmation,
    changeEmail,
    emailToConfirm,
    signupError,
    isPending: registerMutation.isPending,
    isConfirming: confirmMutation.isPending,
    isResending: resendMutation.isPending,
  }
}
