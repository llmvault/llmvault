"use client"

import { useMemo } from "react"
import { $api } from "@/lib/api/hooks"
import { cn } from "@/lib/utils"
import { CHAT_QUERY_STALE_TIME_MS } from "@/app/w/(chat)/_lib/chat-cache"
import {
  type ApiPlugin,
  pluginLogoProvider,
  pluginName,
  pluginSlug,
} from "@/app/w/(chat)/plugins/_lib"
import { PluginLogo } from "@/components/plugin-logo"

const MAX_PLUGINS = 3
const MAX_EXAMPLES_PER_PLUGIN = 3

interface PluginExamplePromptsProps {
  agentId?: string
  onSelect: (prompt: string) => void
}

interface PluginWithExamples {
  plugin: ApiPlugin
  examples: string[]
}

export function PluginExamplePrompts({
  agentId,
  onSelect,
}: PluginExamplePromptsProps) {
  const pluginsQuery = $api.useQuery(
    "get",
    "/v1/plugins",
    {},
    { retry: false, staleTime: CHAT_QUERY_STALE_TIME_MS }
  )

  const groups = useMemo<PluginWithExamples[]>(() => {
    if (!agentId) return []
    const plugins = pluginsQuery.data ?? []
    return plugins
      .filter(
        (plugin) =>
          plugin.examples?.length &&
          plugin.enabled_agent_ids?.includes(agentId)
      )
      .slice()
      .sort((a, b) => (b.created_at ?? "").localeCompare(a.created_at ?? ""))
      .slice(0, MAX_PLUGINS)
      .map((plugin) => ({
        plugin,
        examples: (plugin.examples ?? [])
          .map((prompt) => prompt.trim())
          .filter(Boolean)
          .slice(0, MAX_EXAMPLES_PER_PLUGIN),
      }))
      .filter((group) => group.examples.length > 0)
  }, [agentId, pluginsQuery.data])

  if (!groups.length) return null

  return (
    <div className="mx-auto w-full max-w-3xl space-y-3 px-4">
      {groups.map(({ plugin, examples }) => (
        <div key={pluginSlug(plugin)}>
          <div className="mb-1.5 flex items-center gap-1.5 px-1">
            <span
              className={cn(
                "flex h-4 w-4 shrink-0 items-center justify-center rounded",
                pluginLogoProvider(plugin) ? "bg-white" : "bg-card"
              )}
            >
              <PluginLogo plugin={plugin} size={12} iconSize={11} />
            </span>
            <span className="text-xs font-medium text-muted">
              {pluginName(plugin)}
            </span>
          </div>
          <div className="flex flex-wrap gap-2">
            {examples.map((prompt) => (
              <button
                key={prompt}
                type="button"
                onClick={() => onSelect(prompt)}
                className="rounded-full border border-border bg-surface px-3 py-1.5 text-left text-sm text-muted transition-colors hover:border-primary hover:bg-primary/5 hover:text-foreground"
              >
                {prompt}
              </button>
            ))}
          </div>
        </div>
      ))}
    </div>
  )
}
