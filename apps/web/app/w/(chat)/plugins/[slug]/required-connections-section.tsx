"use client"

import { useState, type ReactNode } from "react"
import { Button, Popover, Spinner } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import type { components } from "@/lib/api/schema"
import {
  RequirementLogo,
  connectionKindLabel,
  isDatabaseRequirement,
  isIntegrationRequirement,
  isRequirementMissing,
  providerLabel,
} from "@/app/w/(chat)/plugins/[slug]/plugin-detail-helpers"
import type { PluginRequirement } from "@/app/w/(chat)/plugins/_lib"

type Connection = components["schemas"]["connectionResponse"]
type DatabaseConnection = components["schemas"]["databaseConnectionResponse"]

export type ConnectionDisconnectTarget =
  | {
      kind: "integration"
      provider: string
      requirement: PluginRequirement
      connection: Connection
    }
  | {
      kind: "database"
      provider: string
      requirement: PluginRequirement
      connection: DatabaseConnection
    }

export function RequiredConnectionsSection({
  requirements,
  missing,
  integrationsLoading,
  connectionsByProvider,
  databaseConnectionsByProvider,
  isBusy,
  disconnectDisabled,
  canManage = true,
  onConnect,
  onReconnect,
  onRename,
  onDisconnect,
}: {
  requirements: PluginRequirement[]
  missing: PluginRequirement[]
  integrationsLoading: boolean
  connectionsByProvider: Map<string, Connection[]>
  databaseConnectionsByProvider: Map<string, DatabaseConnection[]>
  isBusy: boolean
  disconnectDisabled: boolean
  // canManage gates connecting/disconnecting to org admins. Non-admins still
  // see connection status but can't change it; the backend enforces this too.
  canManage?: boolean
  onConnect: (requirement: PluginRequirement) => void
  onReconnect: (requirement: PluginRequirement, connection: Connection) => void
  onRename: (
    kind: "integration" | "database",
    connection: Connection | DatabaseConnection
  ) => void
  onDisconnect: (target: ConnectionDisconnectTarget) => void
}) {
  return (
    <section className="flex flex-col gap-3">
      <div className="flex flex-col gap-1">
        <h2 className="text-base font-semibold text-foreground">
          Required connections
        </h2>
        {missing.length === 0 ? (
          <p className="text-muted-foreground text-sm leading-5">
            All required connections are connected.
          </p>
        ) : null}
      </div>
      {missing.length > 0 ? (
        <div className="flex gap-3 rounded-xl border border-warning/40 bg-warning/10 p-4">
          <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-warning/15 text-warning">
            <AppIcon icon="triangle-alert" className="h-5 w-5" />
          </div>
          <div className="min-w-0">
            <h3 className="text-sm font-medium text-foreground">
              Required connections missing
            </h3>
            <p className="text-muted-foreground mt-1 text-sm leading-5">
              Add the required connections before adding this plugin.
            </p>
          </div>
        </div>
      ) : null}
      <div className="bg-card flex flex-col divide-y divide-border overflow-hidden rounded-xl border border-border">
        {requirements.map((requirement, index) => {
          const provider = requirement.provider ?? ""
          const isMissing = isRequirementMissing(requirement, missing)
          const canConnect =
            isDatabaseRequirement(requirement) ||
            isIntegrationRequirement(requirement)
          const waitingForIntegrations =
            integrationsLoading && isIntegrationRequirement(requirement)
          const connectedConnections = isIntegrationRequirement(requirement)
            ? (connectionsByProvider.get(provider) ?? [])
            : []
          const connectedDatabases = isDatabaseRequirement(requirement)
            ? (databaseConnectionsByProvider.get(provider) ?? [])
            : []
          const connectionCount =
            connectedConnections.length + connectedDatabases.length

          return (
            <div
              key={provider || index}
              className="flex flex-col gap-2.5 px-3 py-2.5"
            >
              <div className="flex items-center justify-between gap-3">
                <div className="flex min-w-0 items-center gap-3">
                  <RequirementLogo requirement={requirement} />
                  <div className="min-w-0">
                    <p className="truncate text-sm font-medium text-foreground">
                      {provider ? providerLabel(provider) : "Connection"}
                    </p>
                    <p className="text-muted-foreground text-xs">
                      {connectionKindLabel(requirement)}
                    </p>
                  </div>
                </div>
                <Button
                  size="sm"
                  variant={isMissing ? "primary" : "secondary"}
                  className="shrink-0 rounded-full"
                  isDisabled={
                    !canManage ||
                    !canConnect ||
                    isBusy ||
                    waitingForIntegrations
                  }
                  onPress={() => onConnect(requirement)}
                >
                  {isBusy ? <Spinner color="current" size="sm" /> : null}
                  {connectionCount > 0 ? "Add connection" : "Connect"}
                </Button>
              </div>
              {connectedConnections.map((connection) => (
                <ConnectionRow
                  key={connection.id}
                  name={
                    connection.name ||
                    connection.slug ||
                    providerLabel(provider)
                  }
                  needsName={connection.needs_name === true}
                  menu={
                    canManage ? (
                      <RequiredConnectionOptionsMenu
                        provider={provider}
                        isBusy={isBusy}
                        disconnectDisabled={disconnectDisabled}
                        onRename={() => onRename("integration", connection)}
                        onReconnect={() => onReconnect(requirement, connection)}
                        onDisconnect={() =>
                          onDisconnect({
                            kind: "integration",
                            provider,
                            requirement,
                            connection,
                          })
                        }
                      />
                    ) : null
                  }
                />
              ))}
              {connectedDatabases.map((connection) => (
                <ConnectionRow
                  key={connection.id}
                  name={
                    connection.name ||
                    connection.slug ||
                    providerLabel(provider)
                  }
                  needsName={connection.needs_name === true}
                  menu={
                    canManage ? (
                      <RequiredConnectionOptionsMenu
                        provider={provider}
                        isBusy={isBusy}
                        disconnectDisabled={disconnectDisabled}
                        onRename={() => onRename("database", connection)}
                        onDisconnect={() =>
                          onDisconnect({
                            kind: "database",
                            provider,
                            requirement,
                            connection,
                          })
                        }
                      />
                    ) : null
                  }
                />
              ))}
            </div>
          )
        })}
      </div>
    </section>
  )
}

function ConnectionRow({
  name,
  needsName,
  menu,
}: {
  name: string
  needsName: boolean
  menu: ReactNode
}) {
  return (
    <div className="ml-11 flex items-center justify-between rounded-lg bg-muted/20 px-2.5 py-1.5">
      <div className="flex min-w-0 items-center gap-2">
        <span className="flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-success text-success-foreground">
          <AppIcon icon="check" className="h-3.5 w-3.5" />
        </span>
        <span className="truncate text-sm text-foreground">{name}</span>
        {needsName ? (
          <span className="rounded-full bg-warning/15 px-2 py-0.5 text-xs text-warning">
            Rename
          </span>
        ) : null}
      </div>
      {menu}
    </div>
  )
}

function RequiredConnectionOptionsMenu({
  provider,
  isBusy,
  disconnectDisabled,
  onReconnect,
  onRename,
  onDisconnect,
}: {
  provider: string
  isBusy: boolean
  disconnectDisabled: boolean
  onReconnect?: () => void
  onRename?: () => void
  onDisconnect?: () => void
}) {
  const [open, setOpen] = useState(false)

  function reconnect() {
    if (isBusy || !onReconnect) return
    setOpen(false)
    onReconnect()
  }

  function disconnect() {
    if (isBusy || disconnectDisabled || !onDisconnect) return
    setOpen(false)
    onDisconnect()
  }

  return (
    <Popover isOpen={open} onOpenChange={setOpen}>
      <Popover.Trigger
        aria-label={`${providerLabel(provider)} connection options`}
        aria-disabled={isBusy ? "true" : undefined}
        data-open={open ? "true" : undefined}
        className="flex h-7 w-7 items-center justify-center rounded-lg text-muted transition-colors hover:bg-default aria-disabled:pointer-events-none aria-disabled:opacity-45 data-[open=true]:bg-default"
      >
        <AppIcon icon="ellipsis" className="h-4 w-4" />
      </Popover.Trigger>
      {open ? (
        <Popover.Content
          placement="bottom end"
          offset={6}
          className="w-44 rounded-2xl border border-border p-1.5"
        >
          <Popover.Dialog className="flex w-full flex-col gap-0.5 p-0">
            {onRename ? (
              <button
                type="button"
                disabled={isBusy}
                onClick={() => {
                  setOpen(false)
                  onRename()
                }}
                className="flex items-center gap-2.5 rounded-xl px-2.5 py-1.5 text-left text-sm transition-colors hover:bg-default disabled:pointer-events-none disabled:opacity-45"
              >
                <AppIcon icon="pencil" className="h-4 w-4 shrink-0" />
                Rename
              </button>
            ) : null}
            {onReconnect ? (
              <button
                type="button"
                disabled={isBusy}
                onClick={reconnect}
                className="flex items-center gap-2.5 rounded-xl px-2.5 py-1.5 text-left text-sm transition-colors hover:bg-default disabled:pointer-events-none disabled:opacity-45"
              >
                <AppIcon icon="refresh-cw" className="h-4 w-4 shrink-0" />
                Reconnect
              </button>
            ) : null}
            {onDisconnect ? (
              <button
                type="button"
                disabled={isBusy || disconnectDisabled}
                title={
                  disconnectDisabled
                    ? "Remove this plugin before disconnecting."
                    : undefined
                }
                onClick={disconnect}
                className="flex items-center gap-2.5 rounded-xl px-2.5 py-1.5 text-left text-sm text-danger transition-colors hover:bg-default disabled:pointer-events-none disabled:opacity-45"
              >
                <AppIcon icon="unlink" className="h-4 w-4 shrink-0" />
                Disconnect
              </button>
            ) : null}
          </Popover.Dialog>
        </Popover.Content>
      ) : null}
    </Popover>
  )
}
