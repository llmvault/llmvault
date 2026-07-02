"use client"

import {
  Button,
  Input,
  Label,
  Spinner,
  Typography,
  toast,
} from "@heroui/react"
import NextLink from "next/link"
import { Suspense, useState, type FormEvent } from "react"
import { useRouter, useSearchParams } from "next/navigation"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"
import {
  AuthCard,
  AuthFooter,
  AuthLogo,
  AuthNavHome,
} from "../_components/shared"

function ResetPasswordPageContent() {
  const router = useRouter()
  const searchParams = useSearchParams()
  const token = searchParams.get("token") ?? ""
  const [errorMessage, setErrorMessage] = useState<string | null>(null)
  const resetPassword = $api.useMutation("post", "/auth/reset-password")

  const onSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    setErrorMessage(null)
    const formData = new FormData(event.currentTarget)
    const password = String(formData.get("password") ?? "")
    const confirmPassword = String(formData.get("confirm-password") ?? "")

    if (password.length < 8) {
      setErrorMessage("Password must be at least 8 characters.")
      return
    }
    if (password !== confirmPassword) {
      setErrorMessage("Passwords don't match.")
      return
    }

    resetPassword.mutate(
      { body: { token, new_password: password } },
      {
        onSuccess: () => {
          toast.success("Password reset. Sign in with your new password.")
          router.replace("/auth/login")
        },
        onError: (error) => {
          setErrorMessage(
            extractErrorMessage(error, "Could not reset your password")
          )
        },
      }
    )
  }

  return (
    <>
      <AuthNavHome />

      <AuthCard>
        <div className="flex flex-col items-center gap-4">
          <AuthLogo />
          <div className="text-center">
            <Typography.Heading level={2} className="text-center">
              {token ? "Reset your password" : "Invalid reset link"}
            </Typography.Heading>
            <Typography.Paragraph
              size="sm"
              color="muted"
              className="mt-1.5 text-center"
            >
              {token
                ? "Choose a new password for your account."
                : "This password reset link is missing or malformed. Request a new one below."}
            </Typography.Paragraph>
          </div>
        </div>

        <div className="flex flex-col gap-6">
          {token ? (
            <form onSubmit={onSubmit} className="flex flex-col gap-3">
              <div className="flex flex-col gap-2">
                <Label htmlFor="password">New password</Label>
                <Input
                  id="password"
                  name="password"
                  type="password"
                  autoComplete="new-password"
                  required
                  minLength={8}
                  placeholder="At least 8 characters"
                  disabled={resetPassword.isPending}
                />
              </div>
              <div className="flex flex-col gap-2">
                <Label htmlFor="confirm-password">Confirm new password</Label>
                <Input
                  id="confirm-password"
                  name="confirm-password"
                  type="password"
                  autoComplete="new-password"
                  required
                  minLength={8}
                  placeholder="Re-enter your new password"
                  disabled={resetPassword.isPending}
                />
              </div>
              {errorMessage ? (
                <Typography.Paragraph size="sm" className="text-danger">
                  {errorMessage}
                </Typography.Paragraph>
              ) : null}
              <Button
                type="submit"
                size="lg"
                fullWidth
                isPending={resetPassword.isPending}
                isDisabled={resetPassword.isPending}
              >
                {({ isPending }) => (
                  <>
                    {isPending ? <Spinner color="current" size="sm" /> : null}
                    {isPending ? "Resetting password..." : "Reset password"}
                  </>
                )}
              </Button>
            </form>
          ) : (
            <Button
              size="lg"
              fullWidth
              onPress={() => router.push("/auth/forgot-password")}
            >
              Request a new link
            </Button>
          )}
          <div className="text-center">
            <Typography.Paragraph size="sm" color="muted">
              Remembered your password?{" "}
              <NextLink href="/auth/login" className="link">
                Sign in
              </NextLink>
            </Typography.Paragraph>
          </div>
        </div>

        <AuthFooter />
      </AuthCard>
    </>
  )
}

export default function ResetPasswordPage() {
  return (
    <Suspense
      fallback={
        <AuthCard>
          <AuthLogo />
        </AuthCard>
      }
    >
      <ResetPasswordPageContent />
    </Suspense>
  )
}
