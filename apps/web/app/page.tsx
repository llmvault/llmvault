"use client"

import { Button, Link, Typography } from "@heroui/react"

const GITHUB_URL = "https://github.com/usehivy/hivy"

export default function RootPage() {
  return (
    <main className="relative flex min-h-screen items-center justify-center bg-background px-6 text-foreground">
      <nav className="absolute right-6 top-6 flex items-center gap-3">
        <Link href="/auth/login">
          <Button variant="tertiary" size="sm">
            Login
          </Button>
        </Link>
        <Link href="/auth/signup">
          <Button size="sm">Sign up</Button>
        </Link>
      </nav>

      <section className="flex max-w-2xl flex-col items-center text-center">
        <Typography.Heading
          level={1}
          className="font-heading text-5xl font-semibold tracking-normal text-foreground sm:text-6xl"
        >
          Hivy
        </Typography.Heading>
        <Typography.Paragraph
          size="base"
          color="muted"
          className="mt-5 max-w-2xl text-center text-lg leading-8 sm:text-xl"
        >
          Your ai coworker that gets work done.
        </Typography.Paragraph>
        <Link
          href={GITHUB_URL}
          target="_blank"
          rel="noreferrer"
          className="mt-8"
        >
          <Button size="lg">View on GitHub</Button>
        </Link>
      </section>
    </main>
  )
}
