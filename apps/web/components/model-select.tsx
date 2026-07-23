"use client"

import Image from "next/image"
import { ListBox, Select } from "@heroui/react"
import { modelLogoURL } from "@/lib/model-logos"
import { isModelNew } from "@/lib/model-new"
import type { components } from "@/lib/api/schema"
import { ModelNewBadge } from "@/components/model-new-badge"

type ModelSummary = components["schemas"]["modelSummary"]

const INHERIT_KEY = "__inherit__"

export function ModelSelect({
  models,
  value,
  onModelChange,
  includeInherit = false,
  disabled = false,
  ariaLabel = "Model",
}: {
  models: ModelSummary[]
  value: string
  onModelChange: (model: string) => void
  includeInherit?: boolean
  disabled?: boolean
  ariaLabel?: string
}) {
  const selectedKey = value ? value : includeInherit ? INHERIT_KEY : null
  const selectedModel = models.find((model) => model.id === value)
  const label = value
    ? selectedModel?.name || value
    : includeInherit
      ? "Inherit from parent"
      : ""

  return (
    <Select
      aria-label={ariaLabel}
      selectedKey={selectedKey}
      onSelectionChange={(key) => {
        if (key === null) return
        onModelChange(String(key) === INHERIT_KEY ? "" : String(key))
      }}
      isDisabled={disabled}
      className="w-full"
    >
      <Select.Trigger className="h-9 w-full justify-between px-3 text-sm transition-colors">
        <span className="flex min-w-0 items-center gap-2">
          {value ? <ModelLogo model={value} /> : null}
          <span className="truncate">{label || <Select.Value />}</span>
          {isModelNew(selectedModel) ? <ModelNewBadge /> : null}
        </span>
        <Select.Indicator />
      </Select.Trigger>
      <Select.Popover className="p-1.5">
        <ListBox className="max-h-72 overflow-y-auto">
          {includeInherit ? (
            <ListBox.Item
              key={INHERIT_KEY}
              id={INHERIT_KEY}
              textValue="Inherit from parent"
            >
              <span className="text-muted-foreground text-sm">
                Inherit from parent
              </span>
            </ListBox.Item>
          ) : null}
          {models.map((model) => {
            const id = model.id || ""
            return (
              <ListBox.Item key={id} id={id} textValue={model.name || id}>
                <span className="flex min-w-0 items-center gap-2">
                  <ModelLogo model={id} />
                  <span className="truncate">{model.name || id}</span>
                  {isModelNew(model) ? <ModelNewBadge /> : null}
                </span>
              </ListBox.Item>
            )
          })}
        </ListBox>
      </Select.Popover>
    </Select>
  )
}

function ModelLogo({ model }: { model: string }) {
  const logoURL = modelLogoURL(model)
  if (!logoURL) {
    return (
      <span className="text-muted-foreground flex h-5 w-5 shrink-0 items-center justify-center rounded-md bg-default text-[9px] font-semibold">
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
