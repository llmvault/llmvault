"use client"

import { type FormEvent, useState } from "react"
import NextLink from "next/link"
import { useRouter } from "next/navigation"
import { useQueryClient } from "@tanstack/react-query"
import { Button, Input, ListBox, Select, Spinner, toast } from "@heroui/react"
import { Icon } from "@iconify/react"
import { extractErrorMessage } from "@/lib/api/error"
import { $api } from "@/lib/api/hooks"
import type { ApiPlugin } from "@/app/w/(chat)/plugins/_lib"
import {
  AGENT_SANDBOX_IMAGE_OPTIONS,
  AGENT_SANDBOX_SIZE_OPTIONS,
  INSTALLED_AGENTS_QUERY_KEY,
  type AgentSandboxImage,
  type AgentSandboxSize,
} from "../_lib"
import { ModelSelect } from "./_model-select"
import { ToolsField } from "./_tools-field"
import { SubAgentsField } from "./_sub-agents-field"
import { PluginsField } from "./_plugins-field"
import {
  SUBAGENT_TASK_TOOL,
  buildCreateBody,
  emptyAgentForm,
  subAgentNameError,
  type AgentForm,
  type ModelSummary,
} from "./_lib"

const EMPTY_MODELS: ModelSummary[] = []
const EMPTY_PLUGINS: ApiPlugin[] = []

export default function CreateAgentPage() {
  const router = useRouter()
  const queryClient = useQueryClient()

  const modelsQuery = $api.useQuery("get", "/v1/agents/models")
  const pluginsQuery = $api.useQuery("get", "/v1/plugins")
  const createAgent = $api.useMutation("post", "/v1/agents")
  const enablePlugin = $api.useMutation("post", "/v1/agents/{id}/plugins/{slug}")

  const models = (modelsQuery.data as ModelSummary[] | undefined) ?? EMPTY_MODELS
  const plugins = (pluginsQuery.data as ApiPlugin[] | undefined) ?? EMPTY_PLUGINS

  const [form, setForm] = useState<AgentForm>(emptyAgentForm)
  const submitting = createAgent.isPending || enablePlugin.isPending

  // Derived state (no effects): default the model to the first available, and
  // keep subagent_task enabled whenever the agent has sub-agents.
  const effectiveModel = form.model || models[0]?.id || ""
  const hasSubAgents = form.subAgents.length > 0
  const parentTools = hasSubAgents
    ? { ...form.tools, [SUBAGENT_TASK_TOOL]: true }
    : form.tools
  const lockedTools = hasSubAgents ? [SUBAGENT_TASK_TOOL] : []
  const canSubmit =
    form.name.trim().length > 0 &&
    effectiveModel.length > 0 &&
    subAgentNameError(form.subAgents) === null &&
    !submitting

  function update(patch: Partial<AgentForm>) {
    setForm((prev) => ({ ...prev, ...patch }))
  }

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!canSubmit) return
    const nameError = subAgentNameError(form.subAgents)
    if (nameError) {
      toast.danger(nameError)
      return
    }
    try {
      const response = await createAgent.mutateAsync({
        body: buildCreateBody({ ...form, model: effectiveModel, tools: parentTools }),
      })
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
    <form onSubmit={handleSubmit} className="flex flex-col gap-8">
      <div>
        <NextLink
          href="/w/settings/agents"
          className="flex w-fit items-center gap-2 text-sm text-muted-foreground transition-colors hover:text-foreground"
        >
          <Icon icon="lucide:arrow-left" className="h-4 w-4" />
          Agents
        </NextLink>
        <h1 className="mt-4 text-2xl font-semibold text-foreground">
          Create agent
        </h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Set up a new workspace agent — its model, tools, plugins, and
          sub-agents.
        </p>
      </div>

      <Section title="Basics" description="Name and describe the agent.">
        <LabeledField label="Name" required>
          <Input
            value={form.name}
            onChange={(event) => update({ name: event.target.value })}
            placeholder="Research Lead"
            className="w-full"
            disabled={submitting}
          />
        </LabeledField>
        <LabeledField label="Description">
          <Input
            value={form.description}
            onChange={(event) => update({ description: event.target.value })}
            placeholder="What this agent is for"
            className="w-full"
            disabled={submitting}
          />
        </LabeledField>
      </Section>

      <Section title="Model" description="The default model this agent runs on.">
        <ModelSelect
          models={models}
          value={effectiveModel}
          onModelChange={(model) => update({ model, availableModels: [model] })}
          disabled={submitting || modelsQuery.isLoading}
          ariaLabel="Default model"
        />
      </Section>

      <Section
        title="Instructions"
        description="The system prompt that defines how this agent behaves."
      >
        <textarea
          value={form.instructions}
          onChange={(event) => update({ instructions: event.target.value })}
          placeholder="You are a research lead who delegates work to sub-agents…"
          rows={6}
          disabled={submitting}
          className="w-full resize-y rounded-md border border-border bg-card px-3 py-2 text-sm text-foreground outline-none transition-colors placeholder:text-muted-foreground focus:border-primary/40"
        />
      </Section>

      <Section
        title="Tools"
        description="Which runtime tools this agent may use. Defaults to all."
      >
        <ToolsField
          selection={parentTools}
          onToolsChange={(tools) => update({ tools })}
          lockedOn={lockedTools}
          disabled={submitting}
        />
      </Section>

      <Section
        title="Plugins"
        description="Workspace plugins this agent can access."
      >
        <PluginsField
          plugins={plugins}
          selectedSlugs={form.pluginSlugs}
          isLoading={pluginsQuery.isLoading}
          disabled={submitting}
          onPluginToggle={(slug, selected) =>
            update({
              pluginSlugs: selected
                ? [...form.pluginSlugs, slug]
                : form.pluginSlugs.filter((item) => item !== slug),
            })
          }
        />
      </Section>

      <Section
        title={`Sub-agents${hasSubAgents ? ` (${form.subAgents.length})` : ""}`}
        description="Specialists this agent can delegate scoped work to."
      >
        <SubAgentsField
          subAgents={form.subAgents}
          onSubAgentsChange={(subAgents) => update({ subAgents })}
          models={models}
          disabled={submitting}
        />
      </Section>

      <Section title="Advanced" description="Sandbox environment.">
        <div className="grid gap-4 sm:grid-cols-2">
          <LabeledField label="Image template">
            <OptionSelect
              ariaLabel="Sandbox image"
              value={form.sandboxImage}
              options={AGENT_SANDBOX_IMAGE_OPTIONS.map((option) => ({
                key: option.key,
                label: option.label,
                hint: option.description,
              }))}
              onValueChange={(value) =>
                update({ sandboxImage: value as AgentSandboxImage })
              }
              disabled={submitting}
            />
          </LabeledField>
          <LabeledField label="Sandbox size">
            <OptionSelect
              ariaLabel="Sandbox size"
              value={form.sandboxSize}
              options={AGENT_SANDBOX_SIZE_OPTIONS.map((option) => ({
                key: option.key,
                label: option.label,
                hint: option.specs,
              }))}
              onValueChange={(value) =>
                update({ sandboxSize: value as AgentSandboxSize })
              }
              disabled={submitting}
            />
          </LabeledField>
        </div>
      </Section>

      <div className="flex items-center justify-end gap-2 border-t border-border pt-4">
        <Button
          type="button"
          variant="tertiary"
          size="sm"
          isDisabled={submitting}
          onPress={() => router.push("/w/settings/agents")}
        >
          Cancel
        </Button>
        <Button type="submit" variant="primary" size="sm" isDisabled={!canSubmit}>
          {submitting ? <Spinner color="current" size="sm" /> : null}
          Create agent
        </Button>
      </div>
    </form>
  )
}

function Section({
  title,
  description,
  children,
}: {
  title: string
  description: string
  children: React.ReactNode
}) {
  return (
    <section className="flex flex-col gap-3">
      <div className="flex flex-col gap-1">
        <h2 className="text-sm font-semibold text-foreground">{title}</h2>
        <p className="text-sm leading-5 text-muted-foreground">{description}</p>
      </div>
      {children}
    </section>
  )
}

function LabeledField({
  label,
  required = false,
  children,
}: {
  label: string
  required?: boolean
  children: React.ReactNode
}) {
  return (
    <label className="flex flex-col gap-1.5">
      <span className="text-xs font-medium text-foreground">
        {label}
        {required ? <span className="text-danger"> *</span> : null}
      </span>
      {children}
    </label>
  )
}

function OptionSelect({
  ariaLabel,
  value,
  options,
  onValueChange,
  disabled,
}: {
  ariaLabel: string
  value: string
  options: Array<{ key: string; label: string; hint: string }>
  onValueChange: (value: string) => void
  disabled?: boolean
}) {
  const selected = options.find((option) => option.key === value) ?? options[0]
  return (
    <Select
      aria-label={ariaLabel}
      selectedKey={value}
      onSelectionChange={(key) => {
        if (key !== null) onValueChange(String(key))
      }}
      isDisabled={disabled}
      className="w-full"
    >
      <Select.Trigger className="h-9 w-full justify-between rounded-md px-3 text-sm transition-colors">
        <span className="truncate">{selected?.label}</span>
        <Select.Indicator />
      </Select.Trigger>
      <Select.Popover className="rounded-xl p-1.5">
        <ListBox>
          {options.map((option) => (
            <ListBox.Item
              key={option.key}
              id={option.key}
              textValue={`${option.label} ${option.hint}`}
            >
              <span className="flex min-w-0 flex-col gap-0.5 sm:flex-row sm:items-center sm:justify-between sm:gap-4">
                <span className="text-sm font-medium">{option.label}</span>
                <span className="text-xs text-muted-foreground">
                  {option.hint}
                </span>
              </span>
            </ListBox.Item>
          ))}
        </ListBox>
      </Select.Popover>
    </Select>
  )
}
