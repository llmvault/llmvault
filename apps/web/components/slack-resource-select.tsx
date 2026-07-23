import { ListBox, Select } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import type { components } from "@/lib/api/schema"

type AvailableResource = components["schemas"]["AvailableResource"]

type SlackConnectionOption = {
  id?: string
  name?: string
  display_name?: string
  nango_connection_id?: string
}

export function SlackConnectionSelect({
  connections,
  value,
  onChange,
  isDisabled,
  placeholder = "Select workspace",
}: {
  connections: SlackConnectionOption[]
  value: string
  onChange: (value: string) => void
  isDisabled?: boolean
  placeholder?: string
}) {
  const selected = connections.find((connection) => connection.id === value)

  return (
    <Select
      aria-label="Slack workspace"
      selectedKey={value || null}
      onSelectionChange={(key) => {
        if (key !== null) onChange(String(key))
      }}
      isDisabled={isDisabled}
      className="w-full"
    >
      <Select.Trigger className="h-9 w-full justify-between px-3 text-sm transition-colors">
        <span className="flex min-w-0 items-center gap-2">
          <AppIcon icon="slack" className="h-4 w-4 shrink-0" />
          <span className="truncate">
            {selected ? connectionLabel(selected) : placeholder}
          </span>
        </span>
        <Select.Indicator />
      </Select.Trigger>
      <Select.Popover className="p-1.5">
        <ListBox className="max-h-72 overflow-y-auto">
          {connections.map((connection) => (
            <ListBox.Item
              key={connection.id}
              id={connection.id ?? ""}
              textValue={connectionLabel(connection)}
            >
              <span className="flex min-w-0 flex-col">
                <span className="truncate text-sm font-medium">
                  {connectionLabel(connection)}
                </span>
                {connection.nango_connection_id ? (
                  <span className="text-muted-foreground truncate text-xs">
                    {connection.nango_connection_id}
                  </span>
                ) : null}
              </span>
            </ListBox.Item>
          ))}
        </ListBox>
      </Select.Popover>
    </Select>
  )
}

export function SlackResourceSelect({
  resources,
  value,
  onChange,
  isDisabled,
  placeholder = "Select channel",
}: {
  resources: AvailableResource[]
  value: string
  onChange: (value: string) => void
  isDisabled?: boolean
  placeholder?: string
}) {
  const selected = resources.find((resource) => resource.id === value)

  return (
    <Select
      aria-label="Slack channel"
      selectedKey={value || null}
      onSelectionChange={(key) => {
        if (key !== null) onChange(String(key))
      }}
      isDisabled={isDisabled}
      className="w-full"
    >
      <Select.Trigger className="h-9 w-full justify-between px-3 text-sm transition-colors">
        <span className="flex min-w-0 items-center gap-2">
          <AppIcon icon="hash" className="h-4 w-4 shrink-0 text-muted" />
          <span className="truncate">
            {selected ? resourceName(selected) : placeholder}
          </span>
        </span>
        <Select.Indicator />
      </Select.Trigger>
      <Select.Popover className="p-1.5">
        <ListBox className="max-h-72 overflow-y-auto">
          {resources.map((resource) => (
            <ListBox.Item
              key={resource.id}
              id={resource.id ?? ""}
              textValue={resourceName(resource)}
            >
              <span className="flex min-w-0 items-center gap-2">
                <AppIcon icon="hash" className="h-4 w-4 shrink-0 text-muted" />
                <span className="truncate text-sm font-medium">
                  {resourceName(resource)}
                </span>
              </span>
            </ListBox.Item>
          ))}
        </ListBox>
      </Select.Popover>
    </Select>
  )
}

function connectionLabel(connection: SlackConnectionOption): string {
  return (
    connection.name ||
    connection.display_name ||
    connection.nango_connection_id ||
    connection.id ||
    "Slack"
  )
}

export function resourceName(resource: AvailableResource): string {
  return resource.name || resource.id || "Slack channel"
}
