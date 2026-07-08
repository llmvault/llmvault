"use client"

import { Button, Link, Separator, Typography } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import Image from "next/image"
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

export function AuthLogo({ className }: { className?: string }) {
  return (
    <div
      className={`flex h-12 w-12 items-center justify-center overflow-hidden rounded-xl${
        className ? ` ${className}` : ""
      }`}
    >
      <Image
        src="/logo.png"
        alt="Hivy"
        width={1254}
        height={1254}
        unoptimized
        className="h-full w-full"
      />
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
        <AppIcon icon="google" />
        Continue with Google
      </Button>
      <Button
        className="w-full"
        variant="tertiary"
        size="lg"
        onPress={() => window.location.assign(withNext("/oauth/github"))}
      >
        <AppIcon icon="github" />
        Continue with GitHub
      </Button>
      <Button
        className="w-full"
        variant="tertiary"
        size="lg"
        onPress={() => window.location.assign(withNext("/oauth/x"))}
      >
        <AppIcon icon="twitter-x" />
        Continue with X
      </Button>
    </div>
  )
}

export function AuthNavHome() {
  return (
    <div className="fixed top-5 right-0 left-0 z-50 mx-auto flex max-w-5xl items-center px-4 md:px-0">
      <NextLink href="/" className="inline-flex items-center gap-1.5">
        <AppIcon icon="arrow-left" size={14} />
        Home
      </NextLink>
    </div>
  )
}
