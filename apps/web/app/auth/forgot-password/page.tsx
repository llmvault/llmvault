"use client"

import { Button, Input, Label, Spinner, Typography } from "@heroui/react"
import NextLink from "next/link"
import { useState, type FormEvent } from "react"
import { toast } from "@heroui/react"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"
import {
  AuthCard,
  AuthFooter,
  AuthLogo,
  AuthNavHome,
} from "../_components/shared"

export default function ForgotPasswordPage() {
  const [submittedEmail, setSubmittedEmail] = useState<string | null>(null)
  const forgotPassword = $api.useMutation("post", "/auth/forgot-password")

  const onSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    const formData = new FormData(event.currentTarget)
    const email = String(formData.get("email") ?? "")
      .trim()
      .toLowerCase()
    if (!email) return

    forgotPassword.mutate(
      { body: { email } },
      {
        onSuccess: () => {
          setSubmittedEmail(email)
        },
        onError: (error) => {
          toast.danger(
            extractErrorMessage(error, "Could not send the reset link")
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
              {submittedEmail ? "Check your email" : "Forgot your password?"}
            </Typography.Heading>
            <Typography.Paragraph
              size="sm"
              color="muted"
              className="mt-1.5 text-center"
            >
              {submittedEmail
                ? `If an account exists for ${submittedEmail}, we've sent a password reset link. It expires in 1 hour.`
                : "Enter your email and we'll send you a link to reset your password."}
            </Typography.Paragraph>
          </div>
        </div>

        <div className="flex flex-col gap-6">
          {submittedEmail ? (
            <Button
              size="lg"
              fullWidth
              variant="tertiary"
              onPress={() => setSubmittedEmail(null)}
            >
              Use a different email
            </Button>
          ) : (
            <form onSubmit={onSubmit} className="flex flex-col gap-3">
              <div className="flex flex-col gap-2">
                <Label htmlFor="email">Work email</Label>
                <Input
                  id="email"
                  name="email"
                  type="email"
                  autoComplete="email"
                  required
                  placeholder="you@company.com"
                  disabled={forgotPassword.isPending}
                />
              </div>
              <Button
                type="submit"
                size="lg"
                fullWidth
                isPending={forgotPassword.isPending}
                isDisabled={forgotPassword.isPending}
              >
                {({ isPending }) => (
                  <>
                    {isPending ? <Spinner color="current" size="sm" /> : null}
                    {isPending ? "Sending link..." : "Send reset link"}
                  </>
                )}
              </Button>
            </form>
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
