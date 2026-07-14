"use client"

import { useEffect, useMemo, useState } from "react"
import Link from "next/link"
import { usePathname } from "next/navigation"
import { AppIcon } from "@/components/icon"
import { Logo } from "@/components/logo"
import { ThemeModeToggle } from "@/components/theme-mode-toggle"
import { DOC_SECTIONS, searchDocPages, type DocPage } from "../_lib/navigation"

const GITHUB_URL = "https://github.com/usehivy/hivy"

export function DocsShell({ children }: { children: React.ReactNode }) {
  const pathname = usePathname()
  const [mobileOpen, setMobileOpen] = useState(false)
  const [query, setQuery] = useState("")
  const results = useMemo(() => searchDocPages(query), [query])

  useEffect(() => {
    function onKeyDown(event: KeyboardEvent) {
      if (event.key === "Escape") {
        setMobileOpen(false)
        setQuery("")
      }
    }
    window.addEventListener("keydown", onKeyDown)
    return () => window.removeEventListener("keydown", onKeyDown)
  }, [])

  return (
    <div className="min-h-dvh bg-background text-foreground">
      <header className="sticky top-0 z-40 border-b border-border bg-background">
        <div className="flex h-14 items-center gap-4 px-4 sm:px-6">
          <button
            type="button"
            aria-label="Open documentation navigation"
            aria-expanded={mobileOpen}
            onClick={() => setMobileOpen(true)}
            className="flex h-9 w-9 items-center justify-center rounded-md text-muted transition-colors hover:bg-default hover:text-foreground focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-focus lg:hidden"
          >
            <AppIcon icon="panel-left-open" className="h-4 w-4" />
          </button>

          <Link
            href="/docs"
            className="flex shrink-0 items-center gap-3 rounded-md focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-focus"
          >
            <Logo className="h-7" />
            <span className="h-5 w-px bg-border" aria-hidden="true" />
            <span className="text-sm font-medium">Documentation</span>
          </Link>

          <div className="relative ml-auto hidden w-full max-w-md md:block">
            <SearchInput value={query} onChange={setQuery} />
            {query.trim() ? (
              <SearchResults
                results={results}
                onNavigate={() => setQuery("")}
              />
            ) : null}
          </div>

          <nav
            aria-label="Documentation utilities"
            className="ml-auto flex items-center gap-1 md:ml-0"
          >
            <Link
              href="/w"
              aria-label="Open Hivy"
              className="inline-flex h-9 items-center gap-2 rounded-md px-3 text-sm font-medium text-muted transition-colors hover:bg-default hover:text-foreground focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-focus"
            >
              <span className="hidden sm:inline">Open Hivy</span>
              <AppIcon icon="arrow-right" className="h-4 w-4" />
            </Link>
            <Link
              href={GITHUB_URL}
              target="_blank"
              rel="noreferrer"
              aria-label="Hivy on GitHub"
              className="hidden h-9 w-9 items-center justify-center rounded-md text-muted transition-colors hover:bg-default hover:text-foreground focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-focus sm:flex"
            >
              <AppIcon icon="github" className="h-4 w-4" />
            </Link>
          </nav>
        </div>
      </header>

      <div className="grid w-full lg:grid-cols-[20rem_minmax(0,1fr)]">
        <aside className="sticky top-14 hidden h-[calc(100dvh-3.5rem)] flex-col border-r border-border lg:flex">
          <div className="min-h-0 flex-1 overflow-y-auto px-4 py-7">
            <DocsNavigation pathname={pathname} />
          </div>
          <DocsThemeControl />
        </aside>

        <div className="min-w-0">
          <div className="mx-auto grid w-full max-w-[1376px] xl:grid-cols-[minmax(0,1fr)_20rem]">
            <main className="min-w-0 px-5 py-12 sm:px-8 lg:px-12 lg:pt-12 lg:pb-16 xl:px-16">
              {children}
            </main>

            <aside className="sticky top-14 hidden h-[calc(100dvh-3.5rem)] overflow-y-auto border-l border-border px-6 py-10 xl:block">
              <SectionRail pathname={pathname} />
            </aside>
          </div>
        </div>
      </div>

      {mobileOpen ? (
        <div className="fixed inset-0 z-50 lg:hidden">
          <button
            type="button"
            aria-label="Close documentation navigation"
            className="absolute inset-0 bg-background/70"
            onClick={() => setMobileOpen(false)}
          />
          <aside className="relative flex h-full w-[min(23rem,92vw)] flex-col overflow-hidden border-r border-border bg-background shadow-xl">
            <div className="min-h-0 flex-1 overflow-y-auto px-5 py-4">
              <div className="flex items-center justify-between">
                <Link
                  href="/docs"
                  onClick={() => setMobileOpen(false)}
                  className="rounded-md focus-visible:outline-2 focus-visible:outline-focus"
                >
                  <Logo className="h-7" />
                </Link>
                <button
                  type="button"
                  aria-label="Close documentation navigation"
                  onClick={() => setMobileOpen(false)}
                  className="flex h-9 w-9 items-center justify-center rounded-md text-muted transition-colors hover:bg-default hover:text-foreground focus-visible:outline-2 focus-visible:outline-focus"
                >
                  <AppIcon icon="x" className="h-4 w-4" />
                </button>
              </div>
              <div className="mt-5 md:hidden">
                <SearchInput value={query} onChange={setQuery} />
                {query.trim() ? (
                  <SearchResults
                    results={results}
                    inline
                    onNavigate={() => {
                      setMobileOpen(false)
                      setQuery("")
                    }}
                  />
                ) : null}
              </div>
              <div className="mt-7">
                <DocsNavigation
                  pathname={pathname}
                  onNavigate={() => {
                    setMobileOpen(false)
                    setQuery("")
                  }}
                />
              </div>
            </div>
            <DocsThemeControl />
          </aside>
        </div>
      ) : null}
    </div>
  )
}

function DocsThemeControl() {
  return (
    <div className="flex shrink-0 items-center justify-between border-t border-border px-5 py-2.5">
      <span className="text-xs font-medium text-muted">Theme</span>
      <ThemeModeToggle />
    </div>
  )
}

function SearchInput({
  value,
  onChange,
}: {
  value: string
  onChange: (value: string) => void
}) {
  return (
    <label className="relative block">
      <span className="sr-only">Search documentation</span>
      <AppIcon
        icon="search"
        className="pointer-events-none absolute top-1/2 left-3 h-4 w-4 -translate-y-1/2 text-muted"
      />
      <input
        type="search"
        value={value}
        onChange={(event) => onChange(event.target.value)}
        placeholder="Search documentation"
        className="h-9 w-full rounded-md border border-border bg-surface pr-3 pl-9 text-sm text-foreground outline-none placeholder:text-muted focus:border-focus focus:ring-1 focus:ring-focus"
      />
    </label>
  )
}

function SearchResults({
  results,
  inline = false,
  onNavigate,
}: {
  results: Array<DocPage & { section: string }>
  inline?: boolean
  onNavigate?: () => void
}) {
  return (
    <div
      className={
        inline
          ? "mt-2 overflow-hidden rounded-lg border border-border bg-surface"
          : "absolute top-11 right-0 left-0 z-50 max-h-96 overflow-y-auto rounded-lg border border-border bg-overlay p-1.5 shadow-xl"
      }
    >
      {results.length ? (
        results.slice(0, 8).map((page) => (
          <Link
            key={page.slug}
            href={`/docs/${page.slug}`}
            onClick={onNavigate}
            className="block rounded-md px-3 py-2.5 transition-colors hover:bg-default focus-visible:bg-default focus-visible:outline-none"
          >
            <span className="block text-sm font-medium text-foreground">
              {page.title}
            </span>
            <span className="mt-0.5 block text-xs text-muted">
              {page.section}
            </span>
          </Link>
        ))
      ) : (
        <p className="px-3 py-5 text-center text-sm text-muted">
          No pages found
        </p>
      )}
    </div>
  )
}

function DocsNavigation({
  pathname,
  onNavigate,
}: {
  pathname: string
  onNavigate?: () => void
}) {
  return (
    <nav aria-label="Documentation pages">
      <div className="space-y-8">
        {DOC_SECTIONS.map((section) => (
          <section key={section.title}>
            <h2 className="max-w-[13rem] px-2 text-[11px] font-semibold tracking-[0.12em] text-foreground uppercase">
              {section.title}
            </h2>
            <ul className="mt-3 space-y-0.5">
              {section.pages.map((page) => {
                const href = `/docs/${page.slug}`
                const active = pathname === href
                return (
                  <li key={page.slug}>
                    <DocsNavigationLink
                      href={href}
                      title={page.title}
                      active={active}
                      onNavigate={onNavigate}
                    />
                  </li>
                )
              })}
            </ul>
          </section>
        ))}
      </div>
    </nav>
  )
}

function DocsNavigationLink({
  href,
  title,
  active,
  onNavigate,
}: {
  href: string
  title: string
  active: boolean
  onNavigate?: () => void
}) {
  return (
    <Link
      href={href}
      onClick={onNavigate}
      aria-current={active ? "page" : undefined}
      className={`flex min-h-8 items-center gap-2.5 rounded-lg border px-2.5 py-1 text-[13px] leading-5 transition-colors focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-focus ${
        active
          ? "border-border bg-surface-secondary font-medium text-foreground shadow-sm"
          : "border-transparent text-muted hover:bg-default hover:text-foreground"
      }`}
    >
      <span
        className={`h-1.5 w-1.5 shrink-0 rounded-full ${
          active ? "bg-accent" : "bg-transparent"
        }`}
        aria-hidden="true"
      />
      <span className="max-w-[13rem] min-w-0 whitespace-normal">{title}</span>
    </Link>
  )
}

function SectionRail({ pathname }: { pathname: string }) {
  const section = DOC_SECTIONS.find((candidate) =>
    candidate.pages.some((page) => pathname === `/docs/${page.slug}`)
  )

  if (!section) {
    return (
      <div>
        <p className="text-xs font-medium tracking-wide text-muted uppercase">
          Documentation
        </p>
        <p className="mt-3 text-sm leading-6 text-muted">
          Start with the product model, then move into the part of Hivy you are
          setting up.
        </p>
      </div>
    )
  }

  return (
    <nav aria-label="Pages in this section">
      <p className="text-xs font-medium tracking-wide text-muted uppercase">
        In this section
      </p>
      <ul className="mt-3 space-y-1">
        {section.pages.map((page) => {
          const href = `/docs/${page.slug}`
          const active = pathname === href
          return (
            <li key={page.slug}>
              <Link
                href={href}
                aria-current={active ? "page" : undefined}
                className={`flex items-center gap-2 rounded-md border px-2.5 py-2 text-sm leading-5 transition-colors focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-focus ${
                  active
                    ? "border-border bg-surface-secondary font-medium text-foreground"
                    : "border-transparent text-muted hover:bg-default hover:text-foreground"
                }`}
              >
                <span
                  className={`h-1.5 w-1.5 shrink-0 rounded-full ${
                    active ? "bg-accent" : "bg-transparent"
                  }`}
                  aria-hidden="true"
                />
                <span>{page.title}</span>
              </Link>
            </li>
          )
        })}
      </ul>
    </nav>
  )
}
