"use client"

import { use, useMemo } from "react"
import NextLink from "next/link"
import { Button, Skeleton } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { $api } from "@/lib/api/hooks"
import { TriggerInstallForm } from "@/app/w/(chat)/automations/_trigger-install-form"
import {
  automationFromCatalog,
  type AutomationItem,
} from "@/app/w/(chat)/automations/_data"

export default function EditTriggerPage({
  params,
}: {
  params: Promise<{ id: string }>
}) {
  const { id } = use(params)
  const triggerQuery = $api.useQuery("get", "/v1/triggers/{id}", {
    params: { path: { id } },
  })
  const catalogQuery = $api.useQuery("get", "/v1/catalog/triggers")
  const trigger = triggerQuery.data?.trigger
  const automation = useMemo(() => {
    if (!trigger) return undefined
    const item = (catalogQuery.data?.data ?? []).find(
      (candidate) =>
        candidate.integration?.provider === trigger.provider &&
        candidate.trigger?.key === trigger.trigger_key
    )
    return item ? automationFromCatalog(item, "Triggers") : undefined
  }, [catalogQuery.data?.data, trigger])
  const isLoading = triggerQuery.isLoading || catalogQuery.isLoading
  const isError = triggerQuery.isError || catalogQuery.isError

  if (isLoading) {
    return <TriggerPageShell content={<TriggerPageSkeleton />} />
  }

  if (isError) {
    return (
      <TriggerPageShell
        content={
          <TriggerPageError
            title="Could not load trigger"
            onRetry={() => {
              void triggerQuery.refetch()
              void catalogQuery.refetch()
            }}
          />
        }
      />
    )
  }

  if (!trigger || !automation) {
    return (
      <TriggerPageShell
        content={
          <TriggerNotFound
            title="Trigger not found"
            body="This trigger may have been removed."
          />
        }
      />
    )
  }

  return (
    <TriggerPageShell
      content={
        <div className="flex flex-col gap-8">
          <NextLink
            href="/w/automations"
            className="text-muted-foreground inline-flex w-fit items-center gap-1.5 text-sm font-medium transition-colors hover:text-foreground"
          >
            <AppIcon icon="arrow-left" className="h-4 w-4" />
            Automations
          </NextLink>

          <header className="flex min-w-0 items-center gap-3">
            <AutomationLogo automation={automation} />
            <div className="min-w-0">
              <h1 className="text-lg font-semibold text-foreground">
                Edit trigger
              </h1>
              <p className="text-muted-foreground mt-1 max-w-xl text-sm leading-5">
                {automation.description}
              </p>
            </div>
          </header>

          <TriggerInstallForm automation={automation} trigger={trigger} />
        </div>
      }
    />
  )
}

function TriggerPageShell({ content }: { content: React.ReactNode }) {
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

function TriggerPageSkeleton() {
  return (
    <div className="flex flex-col gap-8">
      <Skeleton className="h-5 w-32 rounded" />
      <div className="flex items-start gap-3">
        <Skeleton className="h-12 w-12" />
        <div className="min-w-0 flex-1">
          <Skeleton className="h-6 w-44 rounded" />
          <Skeleton className="mt-3 h-4 w-full max-w-lg rounded" />
        </div>
      </div>
      {Array.from({ length: 4 }).map((_, index) => (
        <section key={index} className="flex flex-col gap-3">
          <Skeleton className="h-4 w-28 rounded" />
          <Skeleton className="h-4 w-full max-w-md rounded" />
          <Skeleton className="h-9" />
        </section>
      ))}
    </div>
  )
}

function TriggerPageError({
  title,
  onRetry,
}: {
  title: string
  onRetry: () => void
}) {
  return (
    <div className="bg-card flex min-h-64 flex-col items-center justify-center rounded-xl border border-border px-6 text-center">
      <AppIcon
        icon="triangle-alert"
        className="text-muted-foreground h-7 w-7"
      />
      <p className="mt-3 text-sm font-medium text-foreground">{title}</p>
      <Button variant="ghost" size="sm" className="mt-3" onPress={onRetry}>
        <AppIcon icon="refresh-cw" className="h-4 w-4" />
        Retry
      </Button>
    </div>
  )
}

function TriggerNotFound({ title, body }: { title: string; body: string }) {
  return (
    <div className="bg-card flex min-h-64 flex-col items-center justify-center rounded-xl border border-border px-6 text-center">
      <AppIcon icon="clock-alert" className="h-7 w-7 text-muted" />
      <p className="mt-3 text-sm font-medium text-foreground">{title}</p>
      <p className="mt-1 text-sm text-muted">{body}</p>
      <NextLink
        href="/w/automations"
        className="hover:text-muted-foreground mt-4 text-sm font-medium text-foreground transition-colors"
      >
        Back to automations
      </NextLink>
    </div>
  )
}
