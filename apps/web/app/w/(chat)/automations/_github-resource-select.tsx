import { ListBox, Select } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import type { components } from "@/lib/api/schema"

type Connection = components["schemas"]["connectionResponse"]
type AvailableResource = components["schemas"]["AvailableResource"]

export function GithubConnectionSelect({
  connections,
  value,
  onChange,
}: {
  connections: Connection[]
  value: string
  onChange: (value: string) => void
}) {
  const selected = connections.find((connection) => connection.id === value)

  return (
    <Select
      aria-label="GitHub connection"
      selectedKey={value || null}
      onSelectionChange={(key) => {
        if (key !== null) onChange(String(key))
      }}
      className="w-full"
    >
      <Select.Trigger className="h-9 w-full justify-between rounded-md px-3 text-sm transition-colors">
        <span className="flex min-w-0 items-center gap-2">
          <AppIcon icon="github" className="h-4 w-4 shrink-0" />
          <span className="truncate">
            {selected ? connectionLabel(selected) : "Select connection"}
          </span>
        </span>
        <Select.Indicator />
      </Select.Trigger>
      <Select.Popover className="rounded-xl p-1.5">
        <ListBox>
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
                <span className="text-muted-foreground truncate text-xs">
                  {connection.nango_connection_id}
                </span>
              </span>
            </ListBox.Item>
          ))}
        </ListBox>
      </Select.Popover>
    </Select>
  )
}

export function GithubRepoSelect({
  resources,
  value,
  onChange,
}: {
  resources: AvailableResource[]
  value: string
  onChange: (value: string) => void
}) {
  const selected = resources.find((resource) => resource.id === value)

  return (
    <Select
      aria-label="GitHub repository"
      selectedKey={value || null}
      onSelectionChange={(key) => {
        if (key !== null) onChange(String(key))
      }}
      className="w-full"
    >
      <Select.Trigger className="h-9 w-full justify-between rounded-md px-3 text-sm transition-colors">
        <span className="flex min-w-0 items-center gap-2">
          <AppIcon icon="git-branch" className="h-4 w-4 shrink-0 text-muted" />
          <span className="truncate">
            {selected ? repoName(selected) : "Select repository"}
          </span>
        </span>
        <Select.Indicator />
      </Select.Trigger>
      <Select.Popover className="rounded-xl p-1.5">
        <ListBox>
          {resources.map((resource) => (
            <ListBox.Item
              key={resource.id}
              id={resource.id ?? ""}
              textValue={repoName(resource)}
            >
              <span className="flex min-w-0 items-center gap-2">
                <AppIcon
                  icon="git-branch"
                  className="h-4 w-4 shrink-0 text-muted"
                />
                <span className="truncate text-sm font-medium">
                  {repoName(resource)}
                </span>
              </span>
            </ListBox.Item>
          ))}
        </ListBox>
      </Select.Popover>
    </Select>
  )
}

function connectionLabel(connection: Connection): string {
  return (
    connection.display_name ||
    connection.nango_connection_id ||
    connection.id ||
    "GitHub"
  )
}

// Repositories discover with id=full_name ("owner/repo") and name=repo; the
// full name is the unambiguous label.
export function repoName(resource: AvailableResource): string {
  return resource.id || resource.name || "Repository"
}
