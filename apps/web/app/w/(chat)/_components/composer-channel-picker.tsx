"use client"

import { useMemo, useState } from "react"
import {
  channelDisplayName,
  type SidebarChannelResponse,
} from "@/app/w/(chat)/_lib/sidebar-data"
import { Picker, PickerButton } from "./chat-picker"
import { PickerText } from "./chat-picker-text"

export function ComposerChannelPicker({
  channel,
  channels,
  channelsLoading,
  channelsError,
  onChannelChange,
}: {
  channel?: SidebarChannelResponse
  channels: SidebarChannelResponse[]
  channelsLoading: boolean
  channelsError: boolean
  onChannelChange?: (channel: SidebarChannelResponse) => void
}) {
  const [channelOpen, setChannelOpen] = useState(false)
  const [channelQuery, setChannelQuery] = useState("")
  const filteredChannels = useMemo(() => {
    const query = channelQuery.trim().toLowerCase()
    if (!query) return channels
    return channels.filter((entry) =>
      channelDisplayName(entry).toLowerCase().includes(query)
    )
  }, [channels, channelQuery])

  return (
    <Picker
      open={channelOpen}
      setOpen={(open) => {
        setChannelOpen(open)
        if (!open) setChannelQuery("")
      }}
      label="Select channel"
      icon="hash"
      value={
        channelsLoading
          ? "Loading channels"
          : channel
            ? channelDisplayName(channel)
            : "Select channel"
      }
      width="w-64"
    >
      <input
        type="text"
        value={channelQuery}
        onChange={(event) => setChannelQuery(event.target.value)}
        placeholder="Search channels"
        aria-label="Search channels"
        className="mx-1 mt-0.5 mb-1 rounded-lg border border-border bg-transparent px-2 py-1.5 text-sm outline-none placeholder:text-muted focus:border-primary"
      />
      <div className="flex max-h-64 flex-col gap-0.5 overflow-y-auto">
        {channelsLoading ? (
          <PickerText>Loading channels</PickerText>
        ) : channelsError ? (
          <PickerText>Could not load channels</PickerText>
        ) : filteredChannels.length ? (
          filteredChannels.map((entry) => (
            <PickerButton
              key={entry.id ?? channelDisplayName(entry)}
              icon="hash"
              selected={entry.id === channel?.id}
              onPress={() => {
                onChannelChange?.(entry)
                setChannelOpen(false)
              }}
            >
              {channelDisplayName(entry)}
            </PickerButton>
          ))
        ) : (
          <PickerText>No matching channels</PickerText>
        )}
      </div>
    </Picker>
  )
}
