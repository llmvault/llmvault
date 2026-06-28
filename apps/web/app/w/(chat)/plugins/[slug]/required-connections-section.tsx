"use client"

import { useState } from "react"
import { Button, Popover, Spinner } from "@heroui/react"
import { Icon } from "@iconify/react"
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

export function RequiredConnectionsSection({
  requirements,
  missing,
  integrationsLoading,
  connectionsByProvider,
  isBusy,
  onConnect,
  onReconnect,
}: {
  requirements: PluginRequirement[]
  missing: PluginRequirement[]
  integrationsLoading: boolean
  connectionsByProvider: Map<string, Connection>
  isBusy: boolean
  onConnect: (requirement: PluginRequirement) => void
  onReconnect: (requirement: PluginRequirement, connection: Connection) => void
}) {
  return (
    <section className="flex flex-col gap-3">
      <div className="flex flex-col gap-1">
        <h2 className="text-base font-semibold text-foreground">
          Required connections
        </h2>
        {missing.length === 0 ? (
          <p className="text-sm leading-5 text-muted-foreground">
            All required connections are connected.
          </p>
        ) : null}
      </div>
      {missing.length > 0 ? (
        <div className="border-warning/40 bg-warning/10 flex gap-3 rounded-xl border p-4">
          <div className="bg-warning/15 text-warning flex h-10 w-10 shrink-0 items-center justify-center rounded-lg">
            <Icon icon="lucide:triangle-alert" className="h-5 w-5" />
          </div>
          <div className="min-w-0">
            <h3 className="text-sm font-medium text-foreground">
              Required connections missing
            </h3>
            <p className="mt-1 text-sm leading-5 text-muted-foreground">
              Add the required connections before adding this plugin.
            </p>
          </div>
        </div>
      ) : null}
      <div className="flex flex-col divide-y divide-border overflow-hidden rounded-xl border border-border bg-card">
        {requirements.map((requirement, index) => {
          const provider = requirement.provider ?? ""
          const isMissing = isRequirementMissing(requirement, missing)
          const canConnect =
            isMissing &&
            (isDatabaseRequirement(requirement) ||
              isIntegrationRequirement(requirement))
          const waitingForIntegrations =
            integrationsLoading && isIntegrationRequirement(requirement)
          const connectedConnection =
            !isMissing && isIntegrationRequirement(requirement)
              ? connectionsByProvider.get(provider)
              : undefined

          return (
            <div
              key={provider || index}
              className="flex items-center justify-between gap-3 px-3 py-2.5"
            >
              <div className="flex min-w-0 items-center gap-3">
                <RequirementLogo requirement={requirement} />
                <div className="min-w-0">
                  <p className="truncate text-sm font-medium text-foreground">
                    {provider ? providerLabel(provider) : "Connection"}
                  </p>
                  <p className="text-xs text-muted-foreground">
                    {connectionKindLabel(requirement)}
                  </p>
                </div>
              </div>
              {isMissing ? (
                <Button
                  size="sm"
                  variant="primary"
                  className="shrink-0 rounded-full"
                  isDisabled={!canConnect || isBusy || waitingForIntegrations}
                  onPress={() => onConnect(requirement)}
                >
                  {isBusy ? <Spinner color="current" size="sm" /> : null}
                  Connect
                </Button>
              ) : (
                <div className="flex shrink-0 items-center gap-1">
                  <span
                    aria-label="Connected"
                    className="flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-success text-success-foreground"
                  >
                    <Icon icon="lucide:check" className="h-3.5 w-3.5" />
                  </span>
                  {connectedConnection ? (
                    <RequiredConnectionOptionsMenu
                      provider={provider}
                      isBusy={isBusy}
                      onReconnect={() =>
                        onReconnect(requirement, connectedConnection)
                      }
                    />
                  ) : null}
                </div>
              )}
            </div>
          )
        })}
      </div>
    </section>
  )
}

function RequiredConnectionOptionsMenu({
  provider,
  isBusy,
  onReconnect,
}: {
  provider: string
  isBusy: boolean
  onReconnect: () => void
}) {
  const [open, setOpen] = useState(false)

  function reconnect() {
    if (isBusy) return
    setOpen(false)
    onReconnect()
  }

  return (
    <Popover isOpen={open} onOpenChange={setOpen}>
      <Popover.Trigger
        aria-label={`${providerLabel(provider)} connection options`}
        aria-disabled={isBusy ? "true" : undefined}
        data-open={open ? "true" : undefined}
        className="hover:bg-default data-[open=true]:bg-default flex h-7 w-7 items-center justify-center rounded-lg text-muted transition-colors aria-disabled:pointer-events-none aria-disabled:opacity-45"
      >
        <Icon icon="lucide:ellipsis" className="h-4 w-4" />
      </Popover.Trigger>
      {open ? (
        <Popover.Content
          placement="bottom end"
          offset={6}
          className="w-44 rounded-2xl border border-border p-1.5"
        >
          <Popover.Dialog className="flex w-full flex-col gap-0.5 p-0">
            <button
              type="button"
              disabled={isBusy}
              onClick={reconnect}
              className="hover:bg-default flex items-center gap-2.5 rounded-xl px-2.5 py-1.5 text-left text-sm transition-colors disabled:pointer-events-none disabled:opacity-45"
            >
              <Icon icon="lucide:refresh-cw" className="h-4 w-4 shrink-0" />
              Reconnect
            </button>
          </Popover.Dialog>
        </Popover.Content>
      ) : null}
    </Popover>
  )
}
