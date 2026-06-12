"use client"

import { Button, Link, Separator, Typography } from "@heroui/react"
import { Icon } from "@iconify/react"
import NextLink from "next/link"
import { apiUrl } from "@/lib/api/client"

export function AuthCard({ children }: { children: React.ReactNode }) {
  return (
    <div className="relative flex min-h-screen items-center justify-center px-4 py-12">
      <div className="relative z-10 w-full max-w-105 bg-transparent">
        <div className="flex flex-col gap-8 py-8 px-6">
          {children}
        </div>
      </div>
    </div>
  )
}

export function PlaceholderLogo() {
  return (
    <div className="flex h-12 w-12 items-center justify-center">
      <svg width="32" height="32" viewBox="0 0 32 32" fill="none" xmlns="http://www.w3.org/2000/svg">
        <rect width="32" height="32" rx="8" fill="currentColor" fillOpacity="0.1" />
        <text x="16" y="22" textAnchor="middle" fill="currentColor" fontSize="16" fontWeight="600">H</text>
      </svg>
    </div>
  )
}

export function AuthDivider() {
  return (
    <div className="flex items-center gap-4">
      <Separator className="flex-1" />
      <Typography.Paragraph size="xs" color="muted">or</Typography.Paragraph>
      <Separator className="flex-1" />
    </div>
  )
}

export function AuthFooter() {
  return (
    <div className="text-center">
      <Typography.Paragraph size="xs" color="muted">
        By continuing, you agree to our{" "}
        <Link href="/terms">
          Terms
        </Link>{" "}
        and{" "}
        <Link href="/legal">
          Privacy Policy
        </Link>
        .
      </Typography.Paragraph>
    </div>
  )
}

export function OAuthButtons({ nextPath = "/w" }: { nextPath?: string }) {
  const withNext = (path: string) => {
    if (nextPath === "/w") return apiUrl(path)
    return apiUrl(`${path}?next=${encodeURIComponent(nextPath)}`)
  }

  return (
    <div className="flex flex-col gap-3">
      <Button
        className="w-full"
        variant="tertiary"
        size="lg"
        onPress={() => window.location.assign(withNext("/oauth/google"))}
      >
        <Icon icon="devicon:google" />
        Continue with Google
      </Button>
      <Button
        className="w-full"
        variant="tertiary"
        size="lg"
        onPress={() => window.location.assign(withNext("/oauth/github"))}
      >
        <Icon icon="mdi:github" />
        Continue with GitHub
      </Button>
      <Button
        className="w-full"
        variant="tertiary"
        size="lg"
        onPress={() => window.location.assign(withNext("/oauth/x"))}
      >
        <Icon icon="ri:twitter-x-fill" />
        Continue with X
      </Button>
    </div>
  )
}

export function AuthNavHome() {
  return (
    <div className="fixed top-5 right-0 left-0 z-50 mx-auto flex max-w-5xl items-center px-4 md:px-0">
      <NextLink href="/" className="inline-flex items-center gap-1.5">
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
          <path d="M19 12H5M12 19l-7-7 7-7"/>
        </svg>
        Home
      </NextLink>
    </div>
  )
}
