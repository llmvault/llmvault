"use client"

import Image from "next/image"
import { ListBox, Select } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { ModelNewBadge } from "@/components/model-new-badge"
import { isModelNew } from "@/lib/model-new"
import { modelLogoURL } from "@/lib/model-logos"
import { cn } from "@/lib/utils"
import {
  formatModelPrice,
  formatTokenLimit,
  modelCachePrice,
  modelInputPrice,
  modelKind,
  modelOutputPrice,
  providerForModel,
  type CatalogModel,
  type CatalogProviderOption,
} from "../_lib/catalog-data"

export type CatalogViewProps = {
  models: CatalogModel[]
  total: number
  providerCount: number
  providers: CatalogProviderOption[]
  providerID: string
  onProviderChange: (providerID: string) => void
  query: string
  onQueryChange: (query: string) => void
}

export function CatalogSearch({
  query,
  onQueryChange,
  count,
  total,
  providers,
  providerID,
  onProviderChange,
  inverse = false,
  className,
}: {
  query: string
  onQueryChange: (query: string) => void
  count: number
  total: number
  providers: CatalogProviderOption[]
  providerID: string
  onProviderChange: (providerID: string) => void
  inverse?: boolean
  className?: string
}) {
  return (
    <div
      role="search"
      className={cn(
        "flex min-h-14 items-center gap-3 border-b",
        inverse ? "border-[#4a443e]" : "border-border",
        className
      )}
    >
      <AppIcon
        icon="search"
        aria-hidden="true"
        className={cn("size-4", inverse ? "text-[#aaa39b]" : "text-muted")}
      />
      <input
        type="search"
        value={query}
        onChange={(event) => onQueryChange(event.target.value)}
        placeholder="Search models, families, or providers"
        aria-label="Search model catalog"
        className={cn(
          "min-w-0 flex-1 bg-transparent text-sm outline-none placeholder:text-current/50",
          inverse ? "text-[#f4efe9]" : "text-foreground"
        )}
      />
      <ProviderSelect
        providers={providers}
        providerID={providerID}
        onProviderChange={onProviderChange}
      />
      <span
        aria-live="polite"
        className={cn(
          "shrink-0 text-xs tabular-nums",
          inverse ? "text-[#aaa39b]" : "text-muted"
        )}
      >
        {count} / {total}
      </span>
    </div>
  )
}

function ProviderSelect({
  providers,
  providerID,
  onProviderChange,
}: {
  providers: CatalogProviderOption[]
  providerID: string
  onProviderChange: (providerID: string) => void
}) {
  const selectedKey = providerID || "all"
  const selectedProvider = providers.find(
    (provider) => provider.id === providerID
  )
  return (
    <Select
      aria-label="Filter models by provider"
      selectedKey={selectedKey}
      onSelectionChange={(key) => {
        if (key === null) return
        onProviderChange(key === "all" ? "" : String(key))
      }}
      className="w-36 shrink-0 sm:w-48"
    >
      <Select.Trigger className="h-9 w-full justify-between px-3 text-xs transition-colors">
        <span className="truncate">
          {selectedProvider?.name || "All providers"}
        </span>
        <Select.Indicator />
      </Select.Trigger>
      <Select.Popover className="w-52 p-1.5">
        <ListBox className="max-h-72 overflow-y-auto">
          <ListBox.Item id="all" textValue="All providers">
            All providers
          </ListBox.Item>
          {providers.map((provider) => (
            <ListBox.Item
              id={provider.id}
              key={provider.id}
              textValue={provider.name}
            >
              {provider.name}
            </ListBox.Item>
          ))}
        </ListBox>
      </Select.Popover>
    </Select>
  )
}

export function ModelIdentity({
  model,
  compact = false,
  inverse = false,
}: {
  model: CatalogModel
  compact?: boolean
  inverse?: boolean
}) {
  const logo = modelLogoURL(model.id)
  return (
    <div className="flex w-full min-w-0 items-center gap-3">
      <span
        className={cn(
          "flex shrink-0 items-center justify-center overflow-hidden rounded-sm",
          compact ? "size-8" : "size-10",
          inverse ? "bg-[#f4efe9]" : "bg-surface-secondary"
        )}
      >
        {logo ? (
          <Image
            src={logo}
            alt=""
            width={compact ? 24 : 30}
            height={compact ? 24 : 30}
            className="size-[72%] object-contain"
          />
        ) : (
          <span className="text-[9px] font-semibold text-[#423d38]">AI</span>
        )}
      </span>
      <span className="min-w-0 flex-1">
        <span className="flex min-w-0 items-center gap-2">
          <span className="min-w-0 truncate font-medium">
            {model.name || model.id}
          </span>
          {isModelNew(model) ? <ModelNewBadge /> : null}
        </span>
        <span
          className={cn(
            "mt-0.5 block truncate text-xs",
            inverse ? "text-[#aaa39b]" : "text-muted"
          )}
        >
          {model.id}
        </span>
      </span>
    </div>
  )
}

export function PricePair({
  model,
  providerID,
  inverse = false,
}: {
  model: CatalogModel
  providerID?: string
  inverse?: boolean
}) {
  return (
    <div className="grid grid-cols-2 gap-4 tabular-nums">
      <PriceDatum
        label="Input"
        value={formatModelPrice(modelInputPrice(model, providerID))}
        inverse={inverse}
      />
      <PriceDatum
        label="Output"
        value={formatModelPrice(modelOutputPrice(model, providerID))}
        inverse={inverse}
      />
    </div>
  )
}

function PriceDatum({
  label,
  value,
  inverse = false,
}: {
  label: string
  value: string
  inverse?: boolean
}) {
  return (
    <span>
      <span
        className={cn(
          "block text-[10px] tracking-[0.08em] uppercase",
          inverse ? "text-[#817a73]" : "text-muted"
        )}
      >
        {label}
      </span>
      <span className="mt-0.5 block text-sm font-medium">{value}</span>
    </span>
  )
}

export function ModelMeta({
  model,
  providerID,
  inverse = false,
}: {
  model: CatalogModel
  providerID?: string
  inverse?: boolean
}) {
  const provider = providerForModel(model, providerID)
  return (
    <div
      className={cn(
        "flex flex-wrap gap-x-4 gap-y-1 text-xs",
        inverse ? "text-[#aaa39b]" : "text-muted"
      )}
    >
      <span>{provider?.name || "Provider not listed"}</span>
      <span>{modelKind(model)}</span>
      <span>{formatTokenLimit(model.limit?.context)} context</span>
      {modelCachePrice(model, providerID) !== undefined ? (
        <span>
          {formatModelPrice(modelCachePrice(model, providerID))} cached input
        </span>
      ) : null}
      {(model.providers?.length ?? 0) > 1 ? (
        <span>{model.providers?.length} providers</span>
      ) : null}
    </div>
  )
}
