"use client"

import Image from "next/image"
import { ListBox, Select, Spinner } from "@heroui/react"
import { modelLogoURL } from "@/lib/model-logos"
import {
  AGENT_SANDBOX_IMAGE_OPTIONS,
  AGENT_SANDBOX_SIZE_OPTIONS,
  type AgentSandboxImage,
  type AgentSandboxSize,
} from "../_lib"

export function AgentSettingsSection({
  availableModels,
  selectedModel,
  isBusy,
  onModelChange,
}: {
  availableModels: string[]
  selectedModel: string
  isBusy: boolean
  onModelChange: (model: string) => void
}) {
  return (
    <section className="flex flex-col gap-3">
      <div className="flex flex-col gap-1">
        <h2 className="text-sm font-semibold text-foreground">Default model</h2>
        <p className="text-sm leading-5 text-muted-foreground">
          Choose any available model for this agent.
        </p>
      </div>
      <Select
        aria-label="Default model"
        selectedKey={selectedModel || null}
        onSelectionChange={(key) => {
          if (key !== null) onModelChange(String(key))
        }}
        isDisabled={isBusy || availableModels.length === 0}
        className="w-full"
      >
        <Select.Trigger className="h-9 w-full justify-between px-3 text-sm transition-colors">
          <span className="flex min-w-0 items-center gap-2">
            {selectedModel ? <ModelLogo model={selectedModel} /> : null}
            <span className="truncate">
              {selectedModel ? selectedModel : <Select.Value />}
            </span>
          </span>
          {isBusy ? (
            <Spinner color="current" size="sm" />
          ) : (
            <Select.Indicator />
          )}
        </Select.Trigger>
        <Select.Popover className="p-1.5">
          <ListBox>
            {availableModels.map((model) => (
              <ListBox.Item key={model} id={model} textValue={model}>
                <span className="flex min-w-0 items-center gap-2">
                  <ModelLogo model={model} />
                  <span className="truncate">{model}</span>
                </span>
              </ListBox.Item>
            ))}
          </ListBox>
        </Select.Popover>
      </Select>
    </section>
  )
}

export function SandboxImageSection({
  selectedSandboxImage,
  isBusy,
  onSandboxImageChange,
}: {
  selectedSandboxImage: AgentSandboxImage
  isBusy: boolean
  onSandboxImageChange: (image: AgentSandboxImage) => void
}) {
  const selectedSandboxOption =
    AGENT_SANDBOX_IMAGE_OPTIONS.find(
      (option) => option.key === selectedSandboxImage
    ) ?? AGENT_SANDBOX_IMAGE_OPTIONS[0]

  return (
    <section className="flex flex-col gap-3">
      <div className="flex flex-col gap-1">
        <h2 className="text-sm font-semibold text-foreground">
          Image template
        </h2>
        <p className="text-sm leading-5 text-muted-foreground">
          Choose the base image used when this agent&apos;s sandbox is created.
        </p>
      </div>
      <Select
        aria-label="Image template"
        selectedKey={selectedSandboxImage}
        onSelectionChange={(key) => {
          if (key !== null) {
            onSandboxImageChange(String(key) as AgentSandboxImage)
          }
        }}
        isDisabled={isBusy}
        className="w-full"
      >
        <Select.Trigger className="h-9 w-full justify-between px-3 text-sm transition-colors">
          <span className="flex min-w-0 items-baseline gap-2">
            <span className="shrink-0">{selectedSandboxOption.label}</span>
            <span className="truncate text-xs text-muted-foreground">
              {selectedSandboxOption.description}
            </span>
          </span>
          {isBusy ? (
            <Spinner color="current" size="sm" />
          ) : (
            <Select.Indicator />
          )}
        </Select.Trigger>
        <Select.Popover className="p-1.5">
          <ListBox>
            {AGENT_SANDBOX_IMAGE_OPTIONS.map((option) => (
              <ListBox.Item
                key={option.key}
                id={option.key}
                textValue={`${option.label} ${option.description}`}
              >
                <span className="flex min-w-0 flex-col gap-0.5 sm:flex-row sm:items-center sm:justify-between sm:gap-4">
                  <span className="text-sm font-medium">{option.label}</span>
                  <span className="text-xs text-muted-foreground">
                    {option.description}
                  </span>
                </span>
              </ListBox.Item>
            ))}
          </ListBox>
        </Select.Popover>
      </Select>
    </section>
  )
}

export function SandboxSizeSection({
  selectedSandboxSize,
  isBusy,
  onSandboxSizeChange,
}: {
  selectedSandboxSize: AgentSandboxSize
  isBusy: boolean
  onSandboxSizeChange: (size: AgentSandboxSize) => void
}) {
  const selectedSandboxOption =
    AGENT_SANDBOX_SIZE_OPTIONS.find(
      (option) => option.key === selectedSandboxSize
    ) ?? AGENT_SANDBOX_SIZE_OPTIONS[0]

  return (
    <section className="flex flex-col gap-3">
      <div className="flex flex-col gap-1">
        <h2 className="text-sm font-semibold text-foreground">Sandbox size</h2>
        <p className="text-sm leading-5 text-muted-foreground">
          Choose the resources used when this agent&apos;s sandbox is created.
        </p>
      </div>
      <Select
        aria-label="Sandbox size"
        selectedKey={selectedSandboxSize}
        onSelectionChange={(key) => {
          if (key !== null) onSandboxSizeChange(String(key) as AgentSandboxSize)
        }}
        isDisabled={isBusy}
        className="w-full"
      >
        <Select.Trigger className="h-9 w-full justify-between px-3 text-sm transition-colors">
          <span className="flex min-w-0 items-baseline gap-2">
            <span className="shrink-0">{selectedSandboxOption.label}</span>
            <span className="truncate text-xs text-muted-foreground">
              {selectedSandboxOption.specs}
            </span>
          </span>
          {isBusy ? (
            <Spinner color="current" size="sm" />
          ) : (
            <Select.Indicator />
          )}
        </Select.Trigger>
        <Select.Popover className="p-1.5">
          <ListBox>
            {AGENT_SANDBOX_SIZE_OPTIONS.map((option) => (
              <ListBox.Item
                key={option.key}
                id={option.key}
                textValue={`${option.label} ${option.specs}`}
              >
                <span className="flex min-w-0 flex-col gap-0.5 sm:flex-row sm:items-center sm:justify-between sm:gap-4">
                  <span className="text-sm font-medium">{option.label}</span>
                  <span className="text-xs text-muted-foreground">
                    {option.specs}
                  </span>
                </span>
              </ListBox.Item>
            ))}
          </ListBox>
        </Select.Popover>
      </Select>
    </section>
  )
}

function ModelLogo({ model }: { model: string }) {
  const logoURL = modelLogoURL(model)
  if (!logoURL) {
    return (
      <span className="bg-default flex h-5 w-5 shrink-0 items-center justify-center rounded-md text-[9px] font-semibold text-muted-foreground">
        AI
      </span>
    )
  }

  return (
    <span className="flex h-5 w-5 shrink-0 items-center justify-center rounded-md bg-white p-0.5">
      <Image
        src={logoURL}
        alt=""
        width={20}
        height={20}
        className="size-full object-contain"
      />
    </span>
  )
}
