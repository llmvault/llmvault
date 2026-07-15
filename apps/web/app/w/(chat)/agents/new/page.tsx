"use client"

import posthog from "posthog-js"
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
  const saving = createAgent.isPending

  async function handleCreate(form: AgentForm) {
    try {
      await createAgent.mutateAsync({ body: buildCreateBody(form) })
      queryClient.invalidateQueries({ queryKey: INSTALLED_AGENTS_QUERY_KEY })
      posthog.capture("agent_created", {
        agent_model: form.model,
        has_instructions: form.instructions.trim().length > 0,
        sub_agent_count: form.subAgents.length,
        sandbox_image: form.sandboxImage,
        sandbox_size: form.sandboxSize,
      })
      toast.success(`${form.name.trim()} created`)
      router.push("/w/agents")
    } catch (error) {
      toast.danger(extractErrorMessage(error, "Could not create agent"))
    }
  }

  return (
    <AgentFormView
      heading="Create agent"
      subheading="Set up a new workspace agent — its model, tools, and sub-agents."
      submitLabel="Create agent"
      initialForm={emptyAgentForm()}
      saving={saving}
      onSave={handleCreate}
    />
  )
}
