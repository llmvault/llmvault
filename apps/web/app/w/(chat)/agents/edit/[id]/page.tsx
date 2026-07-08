"use client"

import { use, useMemo, useState } from "react"
import NextLink from "next/link"
import { useRouter } from "next/navigation"
import { useQueryClient } from "@tanstack/react-query"
import { Button, Spinner, Tooltip, toast } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { extractErrorMessage } from "@/lib/api/error"
import { $api } from "@/lib/api/hooks"
import { pluginSlug, type ApiPlugin } from "@/app/w/(chat)/plugins/_lib"
import { INSTALLED_AGENTS_QUERY_KEY, pluginEnabledForAgent } from "../../_lib"
import { RemoveAgentDialog } from "../../_remove-agent-dialog"
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
  const deleteAgent = $api.useMutation("delete", "/v1/agents/{id}")
  const [confirmDelete, setConfirmDelete] = useState(false)

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
      router.push("/w/agents")
    } catch (error) {
      toast.danger(extractErrorMessage(error, "Could not save agent"))
    }
  }

  function handleDelete() {
    deleteAgent.mutate(
      { params: { path: { id } } },
      {
        onSuccess: () => {
          queryClient.invalidateQueries({ queryKey: INSTALLED_AGENTS_QUERY_KEY })
          toast.success(`${agent?.name?.trim() || "Agent"} deleted`)
          setConfirmDelete(false)
          router.push("/w/agents")
        },
        onError: (error) =>
          toast.danger(extractErrorMessage(error, "Could not delete agent")),
      }
    )
  }

  if (agentQuery.isLoading || agentPluginsQuery.isLoading) {
    return <CenteredSpinner />
  }
  if (!agent) {
    return (
      <StatusState
        icon="bot"
        title="Agent not found"
        detail="This agent may have been removed."
      />
    )
  }
  if (agent.catalog) {
    return (
      <StatusState
        icon="package"
        title="Catalog agent"
        detail="Catalog agents are managed from their catalog page."
        href={`/w/agents/${agent.catalog.slug}`}
        linkLabel="Open catalog agent"
      />
    )
  }

  return (
    <>
      <AgentFormView
        heading="Edit agent"
        subheading="Update this agent — its model, tools, plugins, and sub-agents."
        submitLabel="Save changes"
        initialForm={agentFormFromDetail(agent, enabledSlugs)}
        saving={saving}
        onSave={handleUpdate}
        headerAction={
          <DeleteAgentButton
            pending={deleteAgent.isPending}
            // The default Hivy agent is undeletable — the backend 409s. Disable
            // proactively with an explanation when we know that's the case.
            isDefault={Boolean(agent.is_default)}
            onPress={() => setConfirmDelete(true)}
          />
        }
      />
      <RemoveAgentDialog
        open={confirmDelete}
        pending={deleteAgent.isPending}
        heading="Delete agent"
        description={`This permanently removes ${
          agent.name?.trim() || "this agent"
        } and its configuration. This can't be undone.`}
        confirmLabel="Delete agent"
        onOpenChange={setConfirmDelete}
        onConfirm={handleDelete}
      />
    </>
  )
}

function DeleteAgentButton({
  pending,
  isDefault,
  onPress,
}: {
  pending: boolean
  isDefault: boolean
  onPress: () => void
}) {
  const button = (
    <Button
      variant="secondary"
      size="sm"
      className="text-danger"
      isDisabled={pending || isDefault}
      onPress={onPress}
    >
      {pending ? (
        <Spinner color="current" size="sm" />
      ) : (
        <AppIcon icon="trash-2" className="h-4 w-4" />
      )}
      Delete agent
    </Button>
  )

  if (!isDefault) return button

  return (
    <Tooltip delay={200} closeDelay={0}>
      <Tooltip.Trigger className="flex shrink-0">
        <span>{button}</span>
      </Tooltip.Trigger>
      <Tooltip.Content placement="top" offset={6} className="text-xs">
        The default Hivy agent can&apos;t be deleted.
      </Tooltip.Content>
    </Tooltip>
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
      <AppIcon icon={icon} className="h-7 w-7 text-muted-foreground" />
      <p className="mt-3 text-sm font-medium text-foreground">{title}</p>
      <p className="mt-1 text-sm text-muted-foreground">{detail}</p>
      <NextLink
        href={href ?? "/w/agents"}
        className="mt-4 text-sm text-muted-foreground transition-colors hover:text-foreground"
      >
        {linkLabel ?? "Back to agents"}
      </NextLink>
    </div>
  )
}
