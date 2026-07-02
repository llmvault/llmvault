"use client"

import { use, useMemo } from "react"
import NextLink from "next/link"
import { AppIcon } from "@/components/icon"
import { $api } from "@/lib/api/hooks"
import { TriggerInstallForm } from "@/app/w/(chat)/automations/_trigger-install-form"
import {
  automationFromCatalog,
  type AutomationItem,
} from "@/app/w/(chat)/automations/_data"

export default function AutomationInstallPage({
  params,
}: {
  params: Promise<{ id: string }>
}) {
  const { id } = use(params)
  const triggersQuery = $api.useQuery("get", "/v1/catalog/triggers")
  const automations = useMemo(
    () =>
      (triggersQuery.data?.data ?? []).map((item) =>
        automationFromCatalog(item, "Triggers")
      ),
    [triggersQuery.data?.data]
  )
  const automation = automations.find((item) => item.id === id)

  if (triggersQuery.isLoading) {
    return <InstallShell content={<InstallSkeleton />} />
  }

  if (triggersQuery.isError) {
    return (
      <InstallShell
        content={
          <InstallErrorState onRetry={() => void triggersQuery.refetch()} />
        }
      />
    )
  }

  if (!automation) {
    return (
      <InstallShell
        content={
          <div className="bg-card flex min-h-64 flex-col items-center justify-center rounded-xl border border-border px-6 text-center">
            <AppIcon icon="clock-alert" className="h-7 w-7 text-muted" />
            <p className="mt-3 text-sm font-medium text-foreground">
              Trigger not found
            </p>
            <p className="mt-1 text-sm text-muted">
              This trigger may have been removed from the catalog.
            </p>
            <NextLink
              href="/w/automations"
              className="hover:text-muted-foreground mt-4 text-sm font-medium text-foreground transition-colors"
            >
              Back to automations
            </NextLink>
          </div>
        }
      />
    )
  }

  return (
    <InstallShell
      content={
        <div className="flex flex-col gap-8">
          <NextLink
            href="/w/automations/triggers/new"
            className="text-muted-foreground inline-flex w-fit items-center gap-1.5 text-sm font-medium transition-colors hover:text-foreground"
          >
            <AppIcon icon="arrow-left" className="h-4 w-4" />
            Automations
          </NextLink>

          <header className="flex min-w-0 items-center gap-3">
            <AutomationLogo automation={automation} />
            <div className="min-w-0">
              <h1 className="text-xl font-semibold text-foreground">
                Install trigger
              </h1>
              <p className="text-muted-foreground mt-1 max-w-xl text-sm leading-5">
                {automation.description}
              </p>
            </div>
          </header>

          <TriggerInstallForm automation={automation} />
        </div>
      }
    />
  )
}

function InstallShell({ content }: { content: React.ReactNode }) {
  return (
    <div className="h-full overflow-y-auto bg-background text-foreground">
      <div className="mx-auto w-full max-w-2xl px-6 py-12">{content}</div>
    </div>
  )
}

function AutomationLogo({ automation }: { automation: AutomationItem }) {
  return (
    <div
      className="flex h-12 w-12 shrink-0 items-center justify-center rounded-xl"
      style={{ backgroundColor: automation.iconColor }}
    >
      <AppIcon icon={automation.icon} className="h-6 w-6 text-white" />
    </div>
  )
}

function InstallSkeleton() {
  return (
    <div className="flex flex-col gap-8">
      <div className="h-5 w-32 animate-pulse rounded bg-default" />
      <div className="flex items-start gap-3">
        <div className="h-12 w-12 animate-pulse rounded-xl bg-default" />
        <div className="min-w-0 flex-1">
          <div className="h-6 w-44 animate-pulse rounded bg-default" />
          <div className="mt-3 h-4 w-full max-w-lg animate-pulse rounded bg-default" />
        </div>
      </div>
      {Array.from({ length: 4 }).map((_, index) => (
        <section key={index} className="flex flex-col gap-3">
          <div className="h-4 w-28 animate-pulse rounded bg-default" />
          <div className="h-4 w-full max-w-md animate-pulse rounded bg-default" />
          <div className="h-9 animate-pulse rounded-md bg-default" />
        </section>
      ))}
    </div>
  )
}

function InstallErrorState({ onRetry }: { onRetry: () => void }) {
  return (
    <div className="bg-card flex min-h-64 flex-col items-center justify-center rounded-xl border border-border px-6 text-center">
      <AppIcon
        icon="triangle-alert"
        className="text-muted-foreground h-7 w-7"
      />
      <p className="mt-3 text-sm font-medium text-foreground">
        Could not load trigger
      </p>
      <button
        type="button"
        onClick={onRetry}
        className="hover:text-muted-foreground mt-3 text-sm font-medium text-foreground transition-colors"
      >
        Retry
      </button>
    </div>
  )
}
