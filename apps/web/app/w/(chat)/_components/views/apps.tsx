"use client"

import { Skeleton } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { $api } from "@/lib/api/hooks"
import { usePanelAppTargetStore } from "@/app/w/(chat)/_stores/panel-app-target-store"

/**
 * Renders the app targeted via the app-target store inside the shared right
 * panel — the same panel that hosts SheetsView. Session-independent: it needs
 * only the target app id (launch is keyed by app id).
 */
export function AppsView() {
  const target = usePanelAppTargetStore((state) => state.target)

  if (!target) {
    return (
      <AppsState
        icon="layout-grid"
        title="No app open"
        message="Pick an app from the Apps page to launch it here."
      />
    )
  }

  return (
    <AppLauncher
      key={target.appId}
      appId={target.appId}
      appName={target.appName}
    />
  )
}

function AppLauncher({ appId, appName }: { appId: string; appName: string }) {
  const launchQuery = $api.useQuery(
    "get",
    "/v1/apps/{appID}/launch",
    { params: { path: { appID: appId } } },
    { retry: false, staleTime: 60_000 }
  )

  const launch = launchQuery.data
  const base = launch?.app_url || launch?.preview_url || ""
  const mountUrl =
    base && launch?.token
      ? `${base.replace(/\/$/, "")}/auth/callback?token=${encodeURIComponent(launch.token)}`
      : ""

  return (
    <div className="flex min-h-0 flex-1 flex-col bg-background">
      <div className="flex shrink-0 items-center gap-2 border-b border-border px-3 py-2">
        <div className="flex h-6 w-6 shrink-0 items-center justify-center rounded-md bg-violet-600 text-white">
          <AppIcon icon="layout-grid" className="h-3.5 w-3.5" />
        </div>
        <span className="min-w-0 flex-1 truncate text-sm font-medium text-foreground">
          {appName}
        </span>
        {mountUrl ? (
          <a
            href={mountUrl}
            target="_blank"
            rel="noreferrer"
            aria-label="Open in new tab"
            className="flex h-6 w-6 items-center justify-center rounded-md text-muted transition-colors hover:bg-default hover:text-foreground"
          >
            <AppIcon icon="arrow-up-right" className="h-4 w-4" />
          </a>
        ) : null}
      </div>

      <div className="min-h-0 flex-1">
        {launchQuery.isPending ? (
          <Skeleton className="h-full w-full" />
        ) : launchQuery.isError || !mountUrl ? (
          <AppsState
            icon="circle-alert"
            title="App is not running"
            message="This app has no running deployment or preview to open. Deploy it, or start a preview, then try again."
          />
        ) : (
          <iframe
            key={mountUrl}
            src={mountUrl}
            title={appName}
            className="h-full w-full border-0"
            sandbox="allow-scripts allow-forms allow-same-origin allow-popups allow-modals"
          />
        )}
      </div>
    </div>
  )
}

function AppsState({
  icon,
  title,
  message,
}: {
  icon: string
  title: string
  message?: string
}) {
  return (
    <div className="flex h-full flex-col items-center justify-center gap-2 bg-background px-6 text-center">
      <AppIcon icon={icon} className="h-7 w-7 text-muted" />
      <p className="text-sm font-medium text-foreground">{title}</p>
      {message ? <p className="max-w-xs text-xs text-muted">{message}</p> : null}
    </div>
  )
}
