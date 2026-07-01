"use client"

import { useRouter } from "next/navigation"
import { useQueryClient } from "@tanstack/react-query"
import { toast } from "@heroui/react"
import { extractErrorMessage } from "@/lib/api/error"
import { $api } from "@/lib/api/hooks"
import { INSTALLED_AGENTS_QUERY_KEY } from "../_lib"
import { AgentFormView } from "./_agent-form"
import { buildCreateBody, emptyAgentForm, type AgentForm } from "./_lib"

export default function CreateAgentPage() {
  const router = useRouter()
  const queryClient = useQueryClient()
  const createAgent = $api.useMutation("post", "/v1/agents")
  const enablePlugin = $api.useMutation("post", "/v1/agents/{id}/plugins/{slug}")
  const saving = createAgent.isPending || enablePlugin.isPending

  async function handleCreate(form: AgentForm) {
    try {
      const response = await createAgent.mutateAsync({ body: buildCreateBody(form) })
      const agentID = response.agent?.id
      if (agentID) {
        for (const slug of form.pluginSlugs) {
          try {
            await enablePlugin.mutateAsync({
              params: { path: { id: agentID, slug } },
            })
          } catch (error) {
            toast.danger(extractErrorMessage(error, `Could not enable ${slug}`))
          }
        }
      }
      queryClient.invalidateQueries({ queryKey: INSTALLED_AGENTS_QUERY_KEY })
      toast.success(`${form.name.trim()} created`)
      router.push("/w/settings/agents")
    } catch (error) {
      toast.danger(extractErrorMessage(error, "Could not create agent"))
    }
  }

  return (
    <AgentFormView
      heading="Create agent"
      subheading="Set up a new workspace agent — its model, tools, plugins, and sub-agents."
      submitLabel="Create agent"
      initialForm={emptyAgentForm()}
      saving={saving}
      onSave={handleCreate}
    />
  )
}
