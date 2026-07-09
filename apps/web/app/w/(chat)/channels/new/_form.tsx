import Link from "next/link"
import { ListBox, Select } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { cn } from "@/lib/utils"
import type { components } from "@/lib/api/schema"
import { ProviderIcon } from "@/app/w/settings/knowledge/_provider-icon"

type Connection = components["schemas"]["connectionResponse"]
type AvailableResource = components["schemas"]["AvailableResource"]
type Team = components["schemas"]["teamResponse"]

export const SLACK_CHANNEL_RESOURCE_TYPE = "slack_channel"

export type ChannelProviderMeta = {
  provider: string
  label: string
  icon: string
  description: string
  connectionProviders: string[]
  pluginSlug: string
}

export type ChannelProviderStatus = {
  available: boolean
  hint: string
  showConnect: boolean
}

export const CHANNEL_PROVIDERS: ChannelProviderMeta[] = [
  {
    provider: "slack",
    label: "Slack",
    icon: "slack",
    description: "Link a connected Slack channel",
    connectionProviders: ["slack"],
    pluginSlug: "slack",
  },
]

export function connectionsForProvider(
  meta: ChannelProviderMeta | null,
  connections: Connection[]
): Connection[] {
  if (!meta) return []
  return connections.filter(
    (connection) =>
      connection.provider &&
      !connection.revoked_at &&
      meta.connectionProviders.includes(connection.provider)
  )
}

export function connectionLabel(connection: Connection) {
  return (
    connection.display_name ||
    connection.nango_connection_id ||
    connection.id ||
    "Slack"
  )
}

export function ProviderCards({
  value,
  onChange,
  statusFor,
}: {
  value: string
  onChange: (value: ChannelProviderMeta) => void
  statusFor: (meta: ChannelProviderMeta) => ChannelProviderStatus
}) {
  return (
    <div className="flex flex-col gap-2">
      {CHANNEL_PROVIDERS.map((meta) => {
        const selected = value === meta.provider
        const status = statusFor(meta)
        const available = status.available
        return (
          <button
            key={meta.provider}
            type="button"
            disabled={!available}
            onClick={() => available && onChange(meta)}
            aria-pressed={selected}
            className={cn(
              "flex items-center gap-3 rounded-xl border bg-surface px-4 py-3 text-left transition-colors",
              !available && "cursor-not-allowed opacity-55",
              selected
                ? "border-primary ring-1 ring-primary"
                : available && "border-border hover:border-muted-foreground/40",
              !selected && !available && "border-border"
            )}
          >
            <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-default">
              <ProviderIcon icon={meta.icon} className="h-5 w-5" />
            </div>
            <div className="flex min-w-0 flex-1 flex-col gap-0.5">
              <span className="text-sm font-medium text-foreground">
                {meta.label}
              </span>
              <span className="truncate text-xs text-muted-foreground">
                {status.hint}
              </span>
            </div>
            {status.showConnect ? (
              <Link
                href="/w/settings"
                onClick={(event) => event.stopPropagation()}
                className="text-primary hover:text-primary/80 shrink-0 text-xs font-medium transition-colors"
              >
                Connect
              </Link>
            ) : available && selected ? (
              <AppIcon
                icon="circle-check"
                className="h-5 w-5 shrink-0 text-success"
              />
            ) : null}
          </button>
        )
      })}
    </div>
  )
}

export function ResourceRow({
  resource,
  selected,
  onSelect,
}: {
  resource: AvailableResource
  selected: boolean
  onSelect: () => void
}) {
  const label = resource.name || resource.id || "Channel"
  return (
    <button
      type="button"
      aria-pressed={selected}
      className={cn(
        "flex items-center gap-3 border-b border-border px-3 py-2.5 text-left transition-colors last:border-b-0",
        selected ? "bg-primary/10" : "hover:bg-default"
      )}
      onClick={onSelect}
    >
      <AppIcon icon="hash" className="h-4 w-4 shrink-0 text-muted" />
      <div className="min-w-0 flex-1">
        <p className="truncate text-sm font-medium text-foreground">{label}</p>
      </div>
      {selected ? (
        <AppIcon icon="check" className="text-primary h-4 w-4 shrink-0" />
      ) : null}
    </button>
  )
}

export function TeamSelect({
  teams,
  isLoading,
  selectedTeamID,
  onChange,
}: {
  teams: Team[]
  isLoading: boolean
  selectedTeamID: string
  onChange: (teamID: string) => void
}) {
  const selected = teams.find((team) => team.id === selectedTeamID)
  const selectedLabel = isLoading
    ? "Loading teams"
    : selected?.name || "Select a team"

  return (
    <Select
      aria-label="Team"
      selectedKey={selectedTeamID || null}
      onSelectionChange={(key) => {
        if (key !== null) onChange(String(key))
      }}
      isDisabled={isLoading}
      className="w-full"
    >
      <Select.Trigger className="h-9 w-full justify-between px-3 text-sm transition-colors">
        <span className="truncate">{selectedLabel}</span>
        <Select.Indicator />
      </Select.Trigger>
      <Select.Popover className="p-1.5">
        <ListBox>
          {teams.map((team) => (
            <ListBox.Item
              key={team.id}
              id={team.id ?? ""}
              textValue={team.name || team.id || "Team"}
            >
              <span className="text-sm font-medium">
                {team.name || "Untitled team"}
              </span>
            </ListBox.Item>
          ))}
        </ListBox>
      </Select.Popover>
    </Select>
  )
}

export function LockedTeamRow({ label }: { label: string }) {
  return (
    <div className="bg-field-background flex h-9 items-center gap-2 rounded-lg border border-border px-3 text-sm">
      <AppIcon icon="users" className="h-4 w-4 shrink-0 text-muted" />
      <span className="min-w-0 flex-1 truncate font-medium text-foreground">
        {label}
      </span>
    </div>
  )
}

export function StatusRow({ label }: { label: string }) {
  return (
    <div className="flex min-h-24 items-center justify-center px-3 py-4 text-sm text-muted">
      {label}
    </div>
  )
}
