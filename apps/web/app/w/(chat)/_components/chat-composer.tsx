"use client"

import { useState, type ComponentType, type ReactNode } from "react"
import { Button, Popover, Spinner } from "@heroui/react"
import { Icon } from "@iconify/react"
import { modelById } from "@/app/w/(chat)/_lib/agents"
import {
  agentDisplayName,
  agentIcon,
  agentModel,
  channelDisplayName,
  type SidebarAgentResponse,
  type SidebarChannelResponse,
} from "@/app/w/(chat)/_lib/sidebar-data"

const EFFORTS = ["Low", "Medium", "High"]

type DisplayModel = {
  id: string
  label: string
  provider: string
  Icon?: ComponentType<{ className?: string }>
}

export interface ChatComposerProps {
  channel?: SidebarChannelResponse
  channels: SidebarChannelResponse[]
  channelsLoading: boolean
  channelsError: boolean
  agent?: SidebarAgentResponse
  agents: SidebarAgentResponse[]
  agentsLoading: boolean
  agentsError: boolean
  modelId: string
  submitting: boolean
  onChannelChange: (channel: SidebarChannelResponse) => void
  onAgentChange: (agent: SidebarAgentResponse) => void
  onModelChange: (modelId: string) => void
  onSend: (text: string, effort: string) => boolean | Promise<boolean>
  placeholder: string
}

export function ChatComposer({
  channel,
  channels,
  channelsLoading,
  channelsError,
  agent,
  agents,
  agentsLoading,
  agentsError,
  modelId,
  submitting,
  onChannelChange,
  onAgentChange,
  onModelChange,
  onSend,
  placeholder,
}: ChatComposerProps) {
  const [value, setValue] = useState("")
  const [channelOpen, setChannelOpen] = useState(false)
  const [agentOpen, setAgentOpen] = useState(false)
  const [modelOpen, setModelOpen] = useState(false)
  const [effort, setEffort] = useState("High")
  const selectedModel = displayModel(modelId)
  const currentAgentModel = agentModel(agent)
  const modelOptions = currentAgentModel
    ? [displayModel(currentAgentModel)]
    : [selectedModel]
  const canSend = value.trim().length > 0 && Boolean(channel?.id)

  const submit = async () => {
    if (!canSend || submitting) {
      return
    }

    const text = value.trim()
    setValue("")
    try {
      const sent = await onSend(text, effort)
      if (sent) {
        return
      }
      setValue((current) => (current === "" ? text : current))
    } catch {
      setValue((current) => (current === "" ? text : current))
    }
  }

  return (
    <div className="bg-surface flex flex-col gap-2 rounded-3xl border border-border px-3 pt-2.5 pb-2 shadow-sm">
      <textarea
        rows={3}
        value={value}
        placeholder={placeholder}
        onChange={(event) => setValue(event.target.value)}
        onKeyDown={(event) => {
          if (event.key === "Enter" && !event.shiftKey) {
            event.preventDefault()
            void submit()
          }
        }}
        className="min-h-24 w-full resize-none bg-transparent px-2 text-[15px] outline-none placeholder:text-muted"
      />

      <div className="flex items-center gap-1">
        <Button variant="ghost" size="sm" isIconOnly aria-label="Attach">
          <Icon icon="lucide:plus" className="h-4 w-4 text-muted" />
        </Button>

        <Picker
          open={channelOpen}
          setOpen={setChannelOpen}
          label="Select channel"
          icon="lucide:hash"
          value={
            channelsLoading
              ? "Loading channels"
              : channel
                ? channelDisplayName(channel)
                : "Select channel"
          }
          width="w-64"
        >
          {channelsLoading ? (
            <PickerText>Loading channels</PickerText>
          ) : channelsError ? (
            <PickerText>Could not load channels</PickerText>
          ) : channels.length ? (
            channels.map((entry) => (
              <PickerButton
                key={entry.id ?? channelDisplayName(entry)}
                icon="lucide:hash"
                selected={entry.id === channel?.id}
                onPress={() => {
                  onChannelChange(entry)
                  setChannelOpen(false)
                }}
              >
                {channelDisplayName(entry)}
              </PickerButton>
            ))
          ) : (
            <PickerText>No channels</PickerText>
          )}
        </Picker>

        <Picker
          open={agentOpen}
          setOpen={setAgentOpen}
          label="Select agent"
          icon={agentIcon(agent)}
          value={
            agentsLoading
              ? "Loading agents"
              : agent
                ? agentDisplayName(agent)
                : "Select agent"
          }
          width="w-72"
        >
          {agentsLoading ? (
            <PickerText>Loading agents</PickerText>
          ) : agentsError ? (
            <PickerText>Could not load agents</PickerText>
          ) : agents.length ? (
            agents.map((entry) => (
              <PickerButton
                key={entry.id ?? agentDisplayName(entry)}
                icon={agentIcon(entry)}
                selected={entry.id === agent?.id}
                onPress={() => {
                  onAgentChange(entry)
                  setAgentOpen(false)
                }}
              >
                {agentDisplayName(entry)}
              </PickerButton>
            ))
          ) : (
            <PickerText>No agents</PickerText>
          )}
        </Picker>

        <Picker
          open={modelOpen}
          setOpen={setModelOpen}
          label="Select model"
          model={selectedModel}
          value={selectedModel.label}
          suffix={effort}
          width="w-64"
        >
          <span className="px-2.5 pt-1.5 pb-1 text-xs text-muted">
            Models available to {agent ? agentDisplayName(agent) : "agent"}
          </span>
          {modelOptions.map((entry) => (
            <PickerButton
              key={entry.id}
              model={entry}
              selected={entry.id === modelId}
              onPress={() => onModelChange(entry.id)}
              description={entry.provider}
            >
              {entry.label}
            </PickerButton>
          ))}
          <span className="px-2.5 pt-2 pb-1 text-xs text-muted">
            Reasoning effort
          </span>
          {EFFORTS.map((entry) => (
            <PickerButton
              key={entry}
              selected={entry === effort}
              onPress={() => {
                setEffort(entry)
                setModelOpen(false)
              }}
            >
              {entry}
            </PickerButton>
          ))}
        </Picker>

        <div className="flex-1" />

        <Button
          variant="primary"
          size="sm"
          isIconOnly
          aria-label="Send"
          isDisabled={!canSend || submitting}
          onPress={() => void submit()}
          className="rounded-full"
        >
          {submitting ? (
            <Spinner color="current" size="sm" />
          ) : (
            <Icon icon="lucide:arrow-up" className="h-4 w-4" />
          )}
        </Button>
      </div>
    </div>
  )
}

function Picker({
  open,
  setOpen,
  label,
  icon,
  model,
  value,
  suffix,
  width,
  children,
}: {
  open: boolean
  setOpen: (open: boolean) => void
  label: string
  icon?: string
  model?: DisplayModel
  value: string
  suffix?: string
  width: string
  children: ReactNode
}) {
  return (
    <Popover isOpen={open} onOpenChange={setOpen}>
      <Popover.Trigger
        aria-label={label}
        className="hover:bg-default flex max-w-[240px] items-center gap-1.5 rounded-lg px-2 py-1.5 text-sm transition-colors"
      >
        {model ? (
          <ModelIcon model={model} />
        ) : (
          <Icon icon={icon ?? "lucide:circle"} className="h-4 w-4 text-muted" />
        )}
        <span className="min-w-0 truncate font-medium">{value}</span>
        {suffix ? <span className="shrink-0 text-muted">{suffix}</span> : null}
        <Icon
          icon="lucide:chevron-down"
          className="h-3.5 w-3.5 shrink-0 text-muted"
        />
      </Popover.Trigger>
      <Popover.Content
        className={`${width} rounded-2xl border border-border p-1.5`}
      >
        <Popover.Dialog className="flex w-full flex-col gap-0.5 p-0">
          {children}
        </Popover.Dialog>
      </Popover.Content>
    </Popover>
  )
}

function PickerButton({
  icon,
  model,
  selected,
  onPress,
  description,
  children,
}: {
  icon?: string
  model?: DisplayModel
  selected?: boolean
  onPress: () => void
  description?: string
  children: ReactNode
}) {
  return (
    <button
      type="button"
      onClick={onPress}
      className="hover:bg-default flex items-center gap-2.5 rounded-xl px-2.5 py-1.5 text-left text-sm transition-colors"
    >
      {model ? (
        <ModelIcon model={model} />
      ) : icon ? (
        <Icon icon={icon} className="h-4 w-4 text-muted" />
      ) : null}
      <span className="flex min-w-0 flex-1 flex-col">
        <span className="truncate">{children}</span>
        {description ? (
          <span className="text-xs text-muted">{description}</span>
        ) : null}
      </span>
      {selected ? <Icon icon="lucide:check" className="h-4 w-4" /> : null}
    </button>
  )
}

function PickerText({ children }: { children: ReactNode }) {
  return <span className="px-2.5 py-1.5 text-sm text-muted">{children}</span>
}

function displayModel(id: string): DisplayModel {
  try {
    return modelById(id)
  } catch {
    return {
      id,
      label: id,
      provider: "Agent model",
    }
  }
}

function ModelIcon({ model }: { model: DisplayModel }) {
  const IconComponent = model.Icon
  if (IconComponent) {
    return <IconComponent className="h-4 w-4 shrink-0" />
  }
  return <Icon icon="lucide:brain" className="h-4 w-4 shrink-0 text-muted" />
}
