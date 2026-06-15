"use client"

import { useMemo, useState } from "react"
import { useRouter } from "next/navigation"
import {
  Avatar,
  Button,
  Input,
  ListBox,
  Select,
  Spinner,
  TextArea,
  toast,
} from "@heroui/react"
import { Icon } from "@iconify/react"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"
import { cn } from "@/lib/utils"
import type { components } from "@/lib/api/schema"

type ModelSummary = components["schemas"]["modelSummary"]
type SandboxTemplate = components["schemas"]["sandboxTemplateResponse"]

const DEFAULT_MODEL = "deepseek-v4-flash"
const TEMPLATE_NONE = "none"

function initials(name: string) {
  const parts = name.trim().split(/\s+/).filter(Boolean)
  if (parts.length >= 2) return (parts[0][0] + parts[1][0]).toUpperCase()
  return (name.trim().slice(0, 2) || "AG").toUpperCase()
}

export default function NewAgentPage() {
  const router = useRouter()

  const modelsQuery = $api.useQuery("get", "/v1/agents/models")
  const templatesQuery = $api.useQuery("get", "/v1/sandbox-templates", {
    params: { query: { limit: 100 } },
  })
  const createAgent = $api.useMutation("post", "/v1/agents")

  const models = useMemo(
    () => (modelsQuery.data ?? []) as ModelSummary[],
    [modelsQuery.data]
  )
  const templates = ((
    templatesQuery.data as { data?: SandboxTemplate[] } | undefined
  )?.data ?? []) as SandboxTemplate[]

  const [name, setName] = useState("")
  const [description, setDescription] = useState("")
  const [model, setModel] = useState("")
  const [instructions, setInstructions] = useState("")
  const [sandboxStrategy, setSandboxStrategy] = useState<
    "per_session" | "always_on"
  >("per_session")
  const [templateId, setTemplateId] = useState(TEMPLATE_NONE)
  const [submitting, setSubmitting] = useState(false)

  const defaultModelID = useMemo(() => {
    if (models.length === 0) return ""
    return models.find((m) => m.id === DEFAULT_MODEL)?.id ?? models[0]?.id ?? ""
  }, [models])
  const selectedModel = model || defaultModelID

  async function handleCreate() {
    if (!name.trim()) {
      toast.danger("Give your agent a name")
      return
    }
    setSubmitting(true)
    try {
      await createAgent.mutateAsync({
        body: {
          name: name.trim(),
          description: description.trim() || undefined,
          model: selectedModel || undefined,
          instructions: instructions.trim() || undefined,
          sandbox_strategy: sandboxStrategy,
          sandbox_template_id:
            templateId === TEMPLATE_NONE ? undefined : templateId,
        },
      })
      toast.success(`Created ${name.trim()}`)
      router.push("/w/settings/agents")
    } catch (err) {
      toast.danger(extractErrorMessage(err, "Could not create the agent"))
      setSubmitting(false)
    }
  }

  const modelOptions = models.map((m) => ({
    value: m.id ?? "",
    label: m.name ?? m.id ?? "Unknown model",
  }))
  const templateOptions = [
    { value: TEMPLATE_NONE, label: "Default (created on first run)" },
    ...templates.map((t) => ({
      value: t.id ?? "",
      label: t.name ?? t.slug ?? "Template",
    })),
  ]

  return (
    <div className="flex flex-col gap-10">
      <header className="sticky top-0 z-10 flex items-center justify-between gap-4 bg-background/95 py-4 backdrop-blur">
        <div className="flex items-center gap-2">
          <button
            type="button"
            aria-label="Back to agents"
            onClick={() => router.push("/w/settings/agents")}
            className="hover:bg-default flex h-8 w-8 items-center justify-center rounded-lg text-muted transition-colors hover:text-foreground"
          >
            <Icon icon="lucide:arrow-left" className="h-4.5 w-4.5" />
          </button>
          <h1 className="text-lg font-semibold">Create agent</h1>
        </div>
        <div className="flex items-center gap-2">
          <Button
            variant="tertiary"
            size="sm"
            isDisabled={submitting}
            onPress={() => router.push("/w/settings/agents")}
          >
            Cancel
          </Button>
          <Button
            variant="primary"
            size="sm"
            isDisabled={submitting || !name.trim()}
            onPress={() => void handleCreate()}
          >
            {submitting ? <Spinner color="current" size="sm" /> : null}
            Create agent
          </Button>
        </div>
      </header>

      {/* Identity */}
      <Segment
        title="Identity"
        description="How your agent appears across the workspace."
      >
        <div className="flex items-end gap-4">
          <Avatar size="lg" className="shrink-0">
            <Avatar.Fallback>{initials(name)}</Avatar.Fallback>
          </Avatar>
          <Field label="Name" required className="flex-1">
            <Input
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="Support Agent"
              className="w-full"
              autoFocus
            />
          </Field>
        </div>
        <Field label="Description">
          <Input
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder="What this agent is for"
            className="w-full"
          />
        </Field>
      </Segment>

      {/* Instructions & model */}
      <Segment
        title="Instructions & model"
        description="The system prompt and the model that powers this agent."
      >
        <Field label="Model">
          <FieldSelect
            ariaLabel="Model"
            value={selectedModel}
            onChange={setModel}
            options={modelOptions}
            loading={modelsQuery.isLoading}
          />
        </Field>
        <Field
          label="System instructions"
          hint="Describe the agent's role, tone, and how it should behave."
        >
          <TextArea
            value={instructions}
            onChange={(e: React.ChangeEvent<HTMLTextAreaElement>) =>
              setInstructions(e.target.value)
            }
            placeholder="You are a helpful support agent for…"
            rows={6}
            className="w-full"
          />
        </Field>
      </Segment>

      {/* Runtime */}
      <Segment
        title="Runtime"
        description="How the agent's sandbox is provisioned."
      >
        <Field label="Sandbox strategy">
          <Segmented
            options={[
              { value: "per_session", label: "Per session" },
              { value: "always_on", label: "Always on" },
            ]}
            value={sandboxStrategy}
            onChange={(v) =>
              setSandboxStrategy(v as "per_session" | "always_on")
            }
          />
        </Field>
        <Field label="Sandbox template">
          <FieldSelect
            ariaLabel="Sandbox template"
            value={templateId}
            onChange={setTemplateId}
            options={templateOptions}
            loading={templatesQuery.isLoading}
          />
        </Field>
      </Segment>
    </div>
  )
}

function Segment({
  title,
  description,
  children,
}: {
  title: string
  description: string
  children: React.ReactNode
}) {
  return (
    <section className="flex flex-col gap-4">
      <div className="flex flex-col gap-0.5">
        <h2 className="text-sm font-medium">{title}</h2>
        <p className="text-sm text-muted">{description}</p>
      </div>
      <div className="flex flex-col gap-4">{children}</div>
    </section>
  )
}

function Field({
  label,
  hint,
  required,
  className,
  children,
}: {
  label: string
  hint?: string
  required?: boolean
  className?: string
  children: React.ReactNode
}) {
  return (
    <label className={cn("flex flex-col gap-1.5", className)}>
      <span className="text-sm font-medium">
        {label}
        {required ? <span className="text-danger"> *</span> : null}
      </span>
      {children}
      {hint ? <span className="text-xs text-muted">{hint}</span> : null}
    </label>
  )
}

function FieldSelect({
  ariaLabel,
  value,
  onChange,
  options,
  loading,
}: {
  ariaLabel: string
  value: string
  onChange: (value: string) => void
  options: { value: string; label: string }[]
  loading?: boolean
}) {
  return (
    <Select
      aria-label={ariaLabel}
      selectedKey={value || null}
      onSelectionChange={(key) => onChange(String(key))}
      isDisabled={loading}
      className="w-full"
    >
      <Select.Trigger className="w-full justify-between">
        <Select.Value />
        <Select.Indicator />
      </Select.Trigger>
      <Select.Popover>
        <ListBox>
          {options.map((option) => (
            <ListBox.Item key={option.value} id={option.value}>
              {option.label}
            </ListBox.Item>
          ))}
        </ListBox>
      </Select.Popover>
    </Select>
  )
}

function Segmented({
  options,
  value,
  onChange,
}: {
  options: { value: string; label: string }[]
  value: string
  onChange: (value: string) => void
}) {
  return (
    <div className="bg-surface inline-flex items-center gap-1 rounded-xl border border-border p-1">
      {options.map((option) => (
        <button
          key={option.value}
          type="button"
          onClick={() => onChange(option.value)}
          className={cn(
            "rounded-lg px-3 py-1.5 text-sm transition-colors",
            option.value === value
              ? "bg-default font-medium text-foreground"
              : "text-muted hover:text-foreground"
          )}
        >
          {option.label}
        </button>
      ))}
    </div>
  )
}
