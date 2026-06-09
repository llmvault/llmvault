"use client"

import { Button, Input, Label, Typography } from "@heroui/react"
import NextLink from "next/link"
import {
  AuthCard,
  AuthDivider,
  AuthFooter,
  AuthNavHome,
  OAuthButtons,
  PlaceholderLogo,
} from "../_components/shared"

export default function LoginPage() {
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
          <OAuthButtons />
          <AuthDivider />
          <form className="flex flex-col gap-3">
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
                autoComplete="current-password"
                required
                placeholder="Enter your password"
              />
            </div>
            <Button type="submit" size="lg" fullWidth>
              Sign in
            </Button>
          </form>
          <div className="text-center">
            <Typography.Paragraph size="sm" color="muted">
              Don&apos;t have an account?{" "}
              <NextLink href="/hero/auth/signup" className="link">
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
