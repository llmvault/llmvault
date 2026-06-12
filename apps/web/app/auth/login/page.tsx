"use client"

import { Button, Input, Label, Spinner, Typography } from "@heroui/react"
import NextLink from "next/link"
import { Suspense, type FormEvent } from "react"
import { useSearchParams } from "next/navigation"
import {
  safeAuthRedirect,
  usePasswordLogin,
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

function LoginPageContent() {
  const searchParams = useSearchParams()
  const nextPath = safeAuthRedirect(searchParams.get("next"))
  const nextQuery =
    nextPath === "/w" ? "" : `?next=${encodeURIComponent(nextPath)}`
  const { login, isPending } = usePasswordLogin(nextPath)

  const onSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    const formData = new FormData(event.currentTarget)
    login({
      email: String(formData.get("email") ?? ""),
      password: String(formData.get("password") ?? ""),
    } satisfies PasswordAuthInput)
  }

  return (
    <>
      <AuthNavHome />

      <AuthCard>
        <div className="flex flex-col items-center gap-4">
          <PlaceholderLogo />
          <div className="text-center">
            <Typography.Heading level={2} className="text-center">
              Sign in to hivy
            </Typography.Heading>
            <Typography.Paragraph size="sm" color="muted" className="mt-1.5 text-center">
              Welcome back. Sign in to manage your AI coworkers.
            </Typography.Paragraph>
          </div>
        </div>

        <div className="flex flex-col gap-6">
          <OAuthButtons nextPath={nextPath} />
          <AuthDivider />
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
                disabled={isPending}
              />
            </div>
            <div className="flex flex-col gap-2">
              <Label htmlFor="password">Password</Label>
              <Input
                id="password"
                name="password"
                type="password"
                autoComplete="current-password"
                required
                placeholder="Enter your password"
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
                  {isPending ? "Signing in..." : "Sign in"}
                </>
              )}
            </Button>
          </form>
          <div className="text-center">
            <Typography.Paragraph size="sm" color="muted">
              Don&apos;t have an account?{" "}
              <NextLink href={`/auth/signup${nextQuery}`} className="link">
                Sign up
              </NextLink>
            </Typography.Paragraph>
          </div>
        </div>

        <AuthFooter />
      </AuthCard>
    </>
  )
}

export default function LoginPage() {
  return (
    <Suspense fallback={<AuthCard><PlaceholderLogo /></AuthCard>}>
      <LoginPageContent />
    </Suspense>
  )
}
