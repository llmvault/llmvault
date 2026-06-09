"use client"

import { useState } from "react"
import { Button, Input, InputOTP, Label, Typography } from "@heroui/react"
import NextLink from "next/link"
import {
  AuthCard,
  AuthDivider,
  AuthFooter,
  AuthNavHome,
  OAuthButtons,
  PlaceholderLogo,
} from "../_components/shared"

export default function SignupPage() {
  const [emailToConfirm, setEmailToConfirm] = useState<string | null>(null)
  const [otpValue, setOtpValue] = useState("")

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
            <div className="flex flex-col items-center gap-6">
              <div className="text-center">
                <Typography.Paragraph size="sm" color="muted">
                  Enter the 6-digit code sent to{" "}
                  <span>{emailToConfirm}</span>
                </Typography.Paragraph>
              </div>

              <InputOTP
                maxLength={6}
                value={otpValue}
                onChange={setOtpValue}
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

              <Button size="lg" fullWidth>
                Confirm email
              </Button>

              <div className="flex items-center gap-4">
                <Button variant="ghost" size="sm">
                  Resend code
                </Button>
                <Button
                  variant="ghost"
                  size="sm"
                  onPress={() => {
                    setOtpValue("")
                    setEmailToConfirm(null)
                  }}
                >
                  Use a different email
                </Button>
              </div>
            </div>
          ) : (
            <>
              <OAuthButtons />
              <AuthDivider />
              <form
                onSubmit={(e) => {
                  e.preventDefault()
                  setEmailToConfirm("demo@example.com")
                }}
                className="flex flex-col gap-3"
              >
                <div className="flex flex-col gap-2">
                  <Label htmlFor="email">Work email</Label>
                  <Input
                    id="email"
                    type="email"
                    autoComplete="email"
                    required
                    placeholder="you@company.com"
                  />
                </div>
                <div className="flex flex-col gap-2">
                  <Label htmlFor="password">Password</Label>
                  <Input
                    id="password"
                    type="password"
                    autoComplete="new-password"
                    required
                    placeholder="Create a password"
                  />
                </div>
                <Button type="submit" size="lg" fullWidth>
                  Create account
                </Button>
              </form>
              <div className="text-center">
                <Typography.Paragraph size="sm" color="muted">
                  Already have an account?{" "}
                  <NextLink href="/hero/auth/login" className="link">
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
