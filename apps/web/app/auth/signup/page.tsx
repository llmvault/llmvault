"use client"

import { Suspense, type FormEvent } from "react"
import { Button, Input, Label, Spinner, Typography } from "@heroui/react"
import NextLink from "next/link"
import { useSearchParams } from "next/navigation"
import {
  safeAuthRedirect,
  usePasswordSignup,
  type PasswordSignupInput,
} from "@/hooks/use-password-auth"
import {
  AuthCard,
  AuthDivider,
  AuthFooter,
  AuthLogo,
  AuthNavHome,
  OAuthButtons,
} from "../_components/shared"
import { EmailConfirmationForm } from "../_components/email-confirmation-form"

function SignupPageContent() {
  const searchParams = useSearchParams()
  const nextPath = safeAuthRedirect(searchParams.get("next"))
  const nextQuery =
    nextPath === "/w" ? "" : `?next=${encodeURIComponent(nextPath)}`
  const {
    signup,
    confirmEmail,
    resendConfirmation,
    changeEmail,
    emailToConfirm,
    signupError,
    isPending,
    isConfirming,
    isResending,
  } = usePasswordSignup(nextPath)

  const onSignupSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    const formData = new FormData(event.currentTarget)
    signup({
      email: String(formData.get("email") ?? ""),
      password: String(formData.get("password") ?? ""),
    } satisfies PasswordSignupInput)
  }

  return (
    <>
      <AuthNavHome />

      <AuthCard>
        <div className="flex flex-col items-center gap-4">
          <AuthLogo />
          <div className="text-center">
            <Typography.Heading level={2} className="text-center">
              Create your account
            </Typography.Heading>
            <Typography.Paragraph
              size="sm"
              color="muted"
              className="mt-1.5 text-center"
            >
              Start free with 1,000 credits — no card required.
            </Typography.Paragraph>
          </div>
        </div>

        <div className="flex flex-col gap-6">
          {emailToConfirm ? (
            <EmailConfirmationForm
              email={emailToConfirm}
              isConfirming={isConfirming}
              isResending={isResending}
              onConfirm={(code) =>
                confirmEmail({ email: emailToConfirm, code })
              }
              onResend={resendConfirmation}
              onChangeEmail={changeEmail}
            />
          ) : (
            <>
              <OAuthButtons nextPath={nextPath} />
              <AuthDivider />
              <form onSubmit={onSignupSubmit} className="flex flex-col gap-3">
                <div className="flex flex-col gap-2">
                  <Label htmlFor="email">Work email</Label>
                  <Input
                    id="email"
                    name="email"
                    type="email"
                    autoComplete="email"
                    required
                    placeholder="you@company.com"
                    disabled={isPending}
                  />
                </div>
                <div className="flex flex-col gap-2">
                  <Label htmlFor="password">Password</Label>
                  <Input
                    id="password"
                    name="password"
                    type="password"
                    autoComplete="new-password"
                    required
                    placeholder="Create a password"
                    disabled={isPending}
                  />
                </div>
                {signupError ? (
                  <Typography.Paragraph
                    size="sm"
                    className="text-danger"
                    role="alert"
                  >
                    {signupError}
                  </Typography.Paragraph>
                ) : null}
                <Button
                  type="submit"
                  size="lg"
                  fullWidth
                  isPending={isPending}
                  isDisabled={isPending}
                >
                  {({ isPending }) => (
                    <>
                      {isPending ? <Spinner color="current" size="sm" /> : null}
                      {isPending ? "Creating account..." : "Create account"}
                    </>
                  )}
                </Button>
              </form>
              <div className="text-center">
                <Typography.Paragraph size="sm" color="muted">
                  Already have an account?{" "}
                  <NextLink href={`/auth/login${nextQuery}`} className="link">
                    Sign in
                  </NextLink>
                </Typography.Paragraph>
              </div>
            </>
          )}
        </div>

        {/* Footer */}
        <AuthFooter />
      </AuthCard>
    </>
  )
}

export default function SignupPage() {
  return (
    <Suspense
      fallback={
        <AuthCard>
          <AuthLogo />
        </AuthCard>
      }
    >
      <SignupPageContent />
    </Suspense>
  )
}
