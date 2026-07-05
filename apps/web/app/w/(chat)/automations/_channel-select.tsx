"use client"

import { useMemo } from "react"
import { ListBox, Select } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { $api } from "@/lib/api/hooks"
import type { components } from "@/lib/api/schema"

export type Channel = components["schemas"]["channelResponse"]

/**
 * Native Hivy channels the current user can see. External-provider channels
 * (Slack channels, auto-created repo channels, etc.) are excluded — a trigger's
 * session channel must be a plain Hivy channel, not an external event source.
 */
export function useHivyChannels() {
  const query = $api.useQuery("get", "/v1/channels", {
    params: { query: { limit: 100 } },
  })
  const channels = useMemo(
    () =>
      ((query.data?.data ?? []) as Channel[]).filter(
        (channel) =>
          Boolean(channel.id) &&
          !channel.external_provider &&
          !channel.external_connection_id
      ),
    [query.data?.data]
  )
  return { channels, isLoading: query.isLoading, isError: query.isError }
}

export function ChannelSelect({
  channels,
  value,
  onChange,
}: {
  channels: Channel[]
  value: string
  onChange: (value: string) => void
}) {
  const selected = channels.find((channel) => channel.id === value)

  return (
    <Select
      aria-label="Channel"
      selectedKey={value || null}
      onSelectionChange={(key) => {
        if (key !== null) onChange(String(key))
      }}
      className="w-full"
    >
      <Select.Trigger className="h-9 w-full justify-between px-3 text-sm transition-colors">
        <span className="flex min-w-0 items-center gap-2">
          <AppIcon icon="hash" className="h-4 w-4 shrink-0 text-muted" />
          <span className="truncate">
            {selected ? selected.name : "Select channel"}
          </span>
        </span>
        <Select.Indicator />
      </Select.Trigger>
      <Select.Popover className="p-1.5">
        <ListBox>
          {channels.map((channel) => (
            <ListBox.Item
              key={channel.id}
              id={channel.id ?? ""}
              textValue={channel.name ?? ""}
            >
              <span className="flex min-w-0 items-center gap-2">
                <AppIcon icon="hash" className="h-4 w-4 shrink-0 text-muted" />
                <span className="truncate text-sm font-medium">
                  {channel.name}
                </span>
              </span>
            </ListBox.Item>
          ))}
        </ListBox>
      </Select.Popover>
    </Select>
  )
}
