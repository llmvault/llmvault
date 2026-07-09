"use client"

import { useMemo } from "react"
import { Header, ListBox, Select } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { $api } from "@/lib/api/hooks"
import type { components } from "@/lib/api/schema"

type Channel = components["schemas"]["channelResponse"]

export function useHivyChannels() {
  const query = $api.useQuery("get", "/v1/channels", {
    params: { query: { limit: 100 } },
  })
  const channels = useMemo(
    () => (query.data?.data ?? []).filter((channel) => Boolean(channel.id)),
    [query.data?.data]
  )
  return { channels, isLoading: query.isLoading, isError: query.isError }
}

function channelTeamGroups(channels: Channel[]) {
  const groups = new Map<string, { title: string; channels: Channel[] }>()
  for (const channel of channels) {
    const key = channel.team_id ?? ""
    const group = groups.get(key)
    if (group) {
      group.channels.push(channel)
    } else {
      groups.set(key, { title: channel.team_name ?? "", channels: [channel] })
    }
  }
  return [...groups.entries()]
    .map(([key, group]) => ({ key: key || "no-team", ...group }))
    .sort((a, b) => a.title.localeCompare(b.title))
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
  const groups = useMemo(() => channelTeamGroups(channels), [channels])

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
          {groups.map((group) => (
            <ListBox.Section key={group.key}>
              {group.title ? (
                <Header className="px-2 pt-1.5 pb-1 text-xs font-medium text-muted">
                  {group.title}
                </Header>
              ) : null}
              {group.channels.map((channel) => (
                <ListBox.Item
                  key={channel.id}
                  id={channel.id ?? ""}
                  textValue={channel.name ?? ""}
                >
                  <span className="flex min-w-0 items-center gap-2">
                    <AppIcon
                      icon="hash"
                      className="h-4 w-4 shrink-0 text-muted"
                    />
                    <span className="truncate text-sm font-medium">
                      {channel.name}
                    </span>
                  </span>
                </ListBox.Item>
              ))}
            </ListBox.Section>
          ))}
        </ListBox>
      </Select.Popover>
    </Select>
  )
}
