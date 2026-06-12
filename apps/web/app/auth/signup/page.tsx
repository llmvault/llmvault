"use client"

import { Suspense, useState, type FormEvent } from "react"
import { Button, Input, InputOTP, Label, Spinner, Typography } from "@heroui/react"
import NextLink from "next/link"
import { useSearchParams } from "next/navigation"
import {
  safeAuthRedirect,
  usePasswordSignup,
  type PasswordAuthInput,
} from "@/hooks/use-password-auth"
import {
  AuthCard,
  AuthDivider,
  AuthFooter,
  AuthNavHome,
  OAuthButtons,
  PlaceholderLogo,
} from "../_components/shared"

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
    isPending,
    isConfirming,
    isResending,
  } = usePasswordSignup(nextPath)
  const [otpValue, setOtpValue] = useState("")

  const onSignupSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    const formData = new FormData(event.currentTarget)
    signup({
      email: String(formData.get("email") ?? ""),
      password: String(formData.get("password") ?? ""),
    } satisfies PasswordAuthInput)
  }

  const onConfirmSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (!emailToConfirm) return
    confirmEmail({ email: emailToConfirm, code: otpValue })
  }

  return (
    <>
      <AuthNavHome />

      <AuthCard>
        <div className="flex flex-col items-center gap-4">
          <PlaceholderLogo />
          <div className="text-center">
            <Typography.Heading level={2} className="text-center">
              Create your account
            </Typography.Heading>
            <Typography.Paragraph size="sm" color="muted" className="mt-1.5 text-center">
              Start free with 1,000 credits — no card required.
            </Typography.Paragraph>
          </div>
        </div>

        <div className="flex flex-col gap-6">
          {emailToConfirm ? (
            <form onSubmit={onConfirmSubmit} className="flex flex-col items-center gap-6">
              <div className="text-center">
                <Typography.Paragraph size="sm" color="muted">
                  Enter the 6-digit code sent to{" "}
                  <span>{emailToConfirm}</span>
                </Typography.Paragraph>
              </div>

              <InputOTP
                maxLength={6}
                value={otpValue}
                onChange={(value) => {
                  setOtpValue(value)
                  if (value.length === 6 && emailToConfirm) {
                    confirmEmail({ email: emailToConfirm, code: value })
                  }
                }}
                isDisabled={isConfirming}
              >
                <InputOTP.Group>
                  <InputOTP.Slot index={0} />
                  <InputOTP.Slot index={1} />
                  <InputOTP.Slot index={2} />
                </InputOTP.Group>
                <InputOTP.Separator />
                <InputOTP.Group>
                  <InputOTP.Slot index={3} />
                  <InputOTP.Slot index={4} />
                  <InputOTP.Slot index={5} />
                </InputOTP.Group>
              </InputOTP>

              <Button
                type="submit"
                size="lg"
                fullWidth
                isPending={isConfirming}
                isDisabled={isConfirming || otpValue.length !== 6}
              >
                {({ isPending }) => (
                  <>
                    {isPending ? <Spinner color="current" size="sm" /> : null}
                    {isPending ? "Confirming..." : "Confirm email"}
                  </>
                )}
              </Button>

              <div className="flex items-center gap-4">
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  onPress={resendConfirmation}
                  isPending={isResending}
                  isDisabled={isResending}
                >
                  {({ isPending }) => (
                    <>
                      {isPending ? <Spinner color="current" size="sm" /> : null}
                      {isPending ? "Sending..." : "Resend code"}
                    </>
                  )}
                </Button>
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  onPress={() => {
                    setOtpValue("")
                    changeEmail()
                  }}
                  isDisabled={isConfirming}
                >
                  Use a different email
                </Button>
              </div>
            </form>
          ) : (
            <>
              <OAuthButtons nextPath={nextPath} />
              <AuthDivider />
              <form
                onSubmit={onSignupSubmit}
                className="flex flex-col gap-3"
              >
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
    <Suspense fallback={<AuthCard><PlaceholderLogo /></AuthCard>}>
      <SignupPageContent />
    </Suspense>
  )
}
