"use client"

import { type FormEvent, useState } from "react"
import NextLink from "next/link"
import { useRouter } from "next/navigation"
import { Button, Input, ListBox, Select, Spinner, toast } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { $api } from "@/lib/api/hooks"
import type { ApiPlugin } from "@/app/w/(chat)/plugins/_lib"
import {
  AGENT_SANDBOX_IMAGE_OPTIONS,
  AGENT_SANDBOX_SIZE_OPTIONS,
  type AgentSandboxImage,
  type AgentSandboxSize,
} from "../_lib"
import { ModelSelect } from "@/components/model-select"
import { ToolsField } from "./_tools-field"
import { SubAgentsField } from "./_sub-agents-field"
import { PluginsField } from "./_plugins-field"
import { TeamSelect } from "./_team-select"
import {
  DEFAULT_AGENT_MODEL,
  SUBAGENT_TASK_TOOL,
  subAgentNameError,
  toolGroupsFor,
  type AgentForm,
  type ModelSummary,
} from "./_lib"

const AGENT_TOOL_GROUPS = toolGroupsFor("agent")
const EMPTY_MODELS: ModelSummary[] = []
const EMPTY_PLUGINS: ApiPlugin[] = []

// Shared form used by both the create and edit pages so they stay identical.
// The parent owns the API call (create vs update) via onSave and reflects
// progress through the `saving` prop.
export function AgentFormView({
  heading,
  subheading,
  submitLabel,
  initialForm,
  saving,
  onSave,
  headerAction,
}: {
  heading: string
  subheading: string
  submitLabel: string
  initialForm: AgentForm
  saving: boolean
  onSave: (form: AgentForm) => void
  headerAction?: React.ReactNode
}) {
  const router = useRouter()
  const modelsQuery = $api.useQuery("get", "/v1/agents/models")
  const pluginsQuery = $api.useQuery("get", "/v1/plugins")
  const models = (modelsQuery.data as ModelSummary[] | undefined) ?? EMPTY_MODELS
  const plugins = (pluginsQuery.data as ApiPlugin[] | undefined) ?? EMPTY_PLUGINS

  const [form, setForm] = useState<AgentForm>(initialForm)

  // Derived state (no effects): default the model to deepseek-v4-flash when
  // available (else the first model), and keep subagent_task enabled whenever
  // the agent has sub-agents.
  const defaultModel =
    models.find((model) => model.id === DEFAULT_AGENT_MODEL)?.id ||
    models[0]?.id ||
    ""
  const effectiveModel = form.model || defaultModel
  const hasSubAgents = form.subAgents.length > 0
  const parentTools = hasSubAgents
    ? { ...form.tools, [SUBAGENT_TASK_TOOL]: true }
    : form.tools
  const lockedTools = hasSubAgents ? [SUBAGENT_TASK_TOOL] : []
  const canSubmit =
    form.name.trim().length > 0 &&
    effectiveModel.length > 0 &&
    subAgentNameError(form.subAgents) === null &&
    !saving

  function update(patch: Partial<AgentForm>) {
    setForm((prev) => ({ ...prev, ...patch }))
  }

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!canSubmit) return
    const nameError = subAgentNameError(form.subAgents)
    if (nameError) {
      toast.danger(nameError)
      return
    }
    onSave({ ...form, model: effectiveModel, tools: parentTools })
  }

  return (
    <form onSubmit={handleSubmit} className="flex flex-col gap-8">
      <div>
        <NextLink
          href="/w/settings/agents"
          className="flex w-fit items-center gap-2 text-sm text-muted-foreground transition-colors hover:text-foreground"
        >
          <AppIcon icon="arrow-left" className="h-4 w-4" />
          Agents
        </NextLink>
        <div className="mt-4 flex items-start justify-between gap-4">
          <div>
            <h1 className="text-lg font-semibold text-foreground">{heading}</h1>
            <p className="mt-1 text-sm text-muted-foreground">{subheading}</p>
          </div>
          {headerAction ? (
            <div className="shrink-0">{headerAction}</div>
          ) : null}
        </div>
      </div>

      <Section title="Basics" description="Name and describe the agent.">
        <LabeledField label="Name" required>
          <Input
            value={form.name}
            onChange={(event) => update({ name: event.target.value })}
            placeholder="Research Lead"
            className="w-full"
            disabled={saving}
            aria-label="Agent name"
            data-testid="agent-name"
          />
        </LabeledField>
        <LabeledField label="Description">
          <Input
            value={form.description}
            onChange={(event) => update({ description: event.target.value })}
            placeholder="What this agent is for"
            className="w-full"
            disabled={saving}
            aria-label="Agent description"
            data-testid="agent-description"
          />
        </LabeledField>
      </Section>

      <Section title="Model" description="The default model this agent runs on.">
        <ModelSelect
          models={models}
          value={effectiveModel}
          onModelChange={(model) => update({ model })}
          disabled={saving || modelsQuery.isLoading}
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
          disabled={saving}
          aria-label="Agent instructions"
          data-testid="agent-instructions"
          className="w-full resize-y rounded-md border border-border bg-card px-3 py-2 text-sm text-foreground outline-none transition-colors placeholder:text-muted-foreground focus:border-primary/40"
        />
      </Section>

      <Section
        title="Tools"
        description="Which runtime tools this agent may use. Defaults to all."
      >
        <ToolsField
          groups={AGENT_TOOL_GROUPS}
          selection={parentTools}
          onToolsChange={(tools) => update({ tools })}
          lockedOn={lockedTools}
          disabled={saving}
        />
      </Section>

      <Section title="Plugins" description="Workspace plugins this agent can access.">
        <PluginsField
          plugins={plugins}
          selectedSlugs={form.pluginSlugs}
          isLoading={pluginsQuery.isLoading}
          disabled={saving}
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
        title="Team"
        description="Optionally scope this agent to a team. Leave unset to keep it workspace-wide."
      >
        <TeamSelect
          value={form.teamId}
          onValueChange={(teamId) => update({ teamId })}
          disabled={saving}
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
          disabled={saving}
        />
      </Section>

      <Section title="Advanced" description="Sandbox environment.">
        <div className="flex flex-col gap-4">
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
              disabled={saving}
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
              disabled={saving}
            />
          </LabeledField>
        </div>
      </Section>

      <div className="flex items-center justify-end gap-2 border-t border-border pt-4">
        <Button
          type="button"
          variant="tertiary"
          size="sm"
          isDisabled={saving}
          onPress={() => router.push("/w/settings/agents")}
        >
          Cancel
        </Button>
        <Button
          type="submit"
          variant="primary"
          size="sm"
          isDisabled={!canSubmit}
          data-testid="agent-submit"
        >
          {saving ? <Spinner color="current" size="sm" /> : null}
          {submitLabel}
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
      <Select.Trigger className="h-9 w-full justify-between px-3 text-sm transition-colors">
        <span className="truncate">{selected?.label}</span>
        <Select.Indicator />
      </Select.Trigger>
      <Select.Popover className="p-1.5">
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
