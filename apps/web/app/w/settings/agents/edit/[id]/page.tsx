"use client"

import { use, useMemo } from "react"
import NextLink from "next/link"
import { useRouter } from "next/navigation"
import { useQueryClient } from "@tanstack/react-query"
import { Spinner, toast } from "@heroui/react"
import { Icon } from "@iconify/react"
import { extractErrorMessage } from "@/lib/api/error"
import { $api } from "@/lib/api/hooks"
import { pluginSlug, type ApiPlugin } from "@/app/w/(chat)/plugins/_lib"
import { INSTALLED_AGENTS_QUERY_KEY, pluginEnabledForAgent } from "../../_lib"
import { AgentFormView } from "../../new/_agent-form"
import {
  agentFormFromDetail,
  buildCreateBody,
  type AgentDetail,
  type AgentForm,
} from "../../new/_lib"

const EMPTY_PLUGINS: ApiPlugin[] = []

export default function EditAgentPage({
  params,
}: {
  params: Promise<{ id: string }>
}) {
  const { id } = use(params)
  const router = useRouter()
  const queryClient = useQueryClient()

  const agentQuery = $api.useQuery("get", "/v1/agents/{id}", {
    params: { path: { id } },
  })
  const agentPluginsQuery = $api.useQuery("get", "/v1/agents/{id}/plugins", {
    params: { path: { id } },
  })
  const updateAgent = $api.useMutation("patch", "/v1/agents/{id}")
  const enablePlugin = $api.useMutation("post", "/v1/agents/{id}/plugins/{slug}")
  const disablePlugin = $api.useMutation(
    "delete",
    "/v1/agents/{id}/plugins/{slug}"
  )

  const agent = agentQuery.data as AgentDetail | undefined
  const agentPlugins =
    (agentPluginsQuery.data as ApiPlugin[] | undefined) ?? EMPTY_PLUGINS
  const enabledSlugs = useMemo(
    () =>
      agentPlugins
        .filter((plugin) => pluginEnabledForAgent(plugin, id))
        .map(pluginSlug)
        .filter(Boolean),
    [agentPlugins, id]
  )
  const saving =
    updateAgent.isPending || enablePlugin.isPending || disablePlugin.isPending

  async function handleUpdate(form: AgentForm) {
    try {
      await updateAgent.mutateAsync({
        params: { path: { id } },
        body: buildCreateBody(form),
      })
      const after = new Set(form.pluginSlugs)
      for (const slug of form.pluginSlugs) {
        if (enabledSlugs.includes(slug)) continue
        try {
          await enablePlugin.mutateAsync({ params: { path: { id, slug } } })
        } catch (error) {
          toast.danger(extractErrorMessage(error, `Could not enable ${slug}`))
        }
      }
      for (const slug of enabledSlugs) {
        if (after.has(slug)) continue
        try {
          await disablePlugin.mutateAsync({ params: { path: { id, slug } } })
        } catch (error) {
          toast.danger(extractErrorMessage(error, `Could not disable ${slug}`))
        }
      }
      queryClient.invalidateQueries({ queryKey: INSTALLED_AGENTS_QUERY_KEY })
      toast.success(`${form.name.trim()} saved`)
      router.push("/w/settings/agents")
    } catch (error) {
      toast.danger(extractErrorMessage(error, "Could not save agent"))
    }
  }

  if (agentQuery.isLoading || agentPluginsQuery.isLoading) {
    return <CenteredSpinner />
  }
  if (!agent) {
    return (
      <StatusState
        icon="lucide:bot"
        title="Agent not found"
        detail="This agent may have been removed."
      />
    )
  }
  if (agent.catalog) {
    return (
      <StatusState
        icon="lucide:package"
        title="Catalog agent"
        detail="Catalog agents are managed from their catalog page."
        href={`/w/settings/agents/${agent.catalog.slug}`}
        linkLabel="Open catalog agent"
      />
    )
  }

  return (
    <AgentFormView
      heading="Edit agent"
      subheading="Update this agent — its model, tools, plugins, and sub-agents."
      submitLabel="Save changes"
      initialForm={agentFormFromDetail(agent, enabledSlugs)}
      saving={saving}
      onSave={handleUpdate}
    />
  )
}

function CenteredSpinner() {
  return (
    <div className="flex min-h-64 items-center justify-center">
      <Spinner size="sm" />
    </div>
  )
}

function StatusState({
  icon,
  title,
  detail,
  href,
  linkLabel,
}: {
  icon: string
  title: string
  detail: string
  href?: string
  linkLabel?: string
}) {
  return (
    <div className="bg-card flex min-h-64 flex-col items-center justify-center rounded-xl border border-border px-6 text-center">
      <Icon icon={icon} className="h-7 w-7 text-muted-foreground" />
      <p className="mt-3 text-sm font-medium text-foreground">{title}</p>
      <p className="mt-1 text-sm text-muted-foreground">{detail}</p>
      <NextLink
        href={href ?? "/w/settings/agents"}
        className="mt-4 text-sm text-muted-foreground transition-colors hover:text-foreground"
      >
        {linkLabel ?? "Back to agents"}
      </NextLink>
    </div>
  )
}
