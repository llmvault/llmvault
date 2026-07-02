"use client"

import { useState } from "react"
import { Button, Input } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { ModelSelect } from "./_model-select"
import { ToolsField } from "./_tools-field"
import {
  emptySubAgent,
  toolGroupsFor,
  type ModelSummary,
  type SubAgentForm,
  type ToolSelection,
} from "./_lib"

const SUBAGENT_TOOL_GROUPS = toolGroupsFor("subagent")

export function SubAgentsField({
  subAgents,
  onSubAgentsChange,
  models,
  disabled = false,
}: {
  subAgents: SubAgentForm[]
  onSubAgentsChange: (next: SubAgentForm[]) => void
  models: ModelSummary[]
  disabled?: boolean
}) {
  const [openKeys, setOpenKeys] = useState<Set<string>>(new Set())

  function toggleOpen(key: string) {
    setOpenKeys((prev) => {
      const next = new Set(prev)
      if (next.has(key)) next.delete(key)
      else next.add(key)
      return next
    })
  }

  function addSubAgent() {
    const sub = emptySubAgent()
    onSubAgentsChange([...subAgents, sub])
    setOpenKeys((prev) => new Set(prev).add(sub.key))
  }

  function removeSubAgent(key: string) {
    onSubAgentsChange(subAgents.filter((sub) => sub.key !== key))
  }

  function patchSubAgent(key: string, patch: Partial<SubAgentForm>) {
    onSubAgentsChange(
      subAgents.map((sub) => (sub.key === key ? { ...sub, ...patch } : sub))
    )
  }

  return (
    <div className="flex flex-col gap-3">
      {subAgents.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          No sub-agents. Add one to let this agent delegate scoped work.
        </p>
      ) : null}

      <div className="flex flex-col gap-2">
        {subAgents.map((sub) => {
          const open = openKeys.has(sub.key)
          return (
            <div
              key={sub.key}
              data-testid="sub-agent-card"
              className="overflow-hidden rounded-xl border border-border bg-card"
            >
              <div className="flex items-center gap-3 px-3 py-2.5">
                <button
                  type="button"
                  onClick={() => toggleOpen(sub.key)}
                  data-testid="sub-agent-toggle"
                  className="flex min-w-0 flex-1 items-center gap-2 text-left"
                >
                  <AppIcon
                    icon={open ? "chevron-down" : "chevron-right"}
                    className="h-4 w-4 shrink-0 text-muted-foreground"
                  />
                  <span className="min-w-0 truncate text-sm font-medium text-foreground">
                    {sub.name.trim() || "Untitled sub-agent"}
                  </span>
                </button>
                <button
                  type="button"
                  onClick={() => removeSubAgent(sub.key)}
                  disabled={disabled}
                  aria-label="Remove sub-agent"
                  className="shrink-0 rounded-md p-1 text-muted-foreground transition-colors hover:text-danger"
                >
                  <AppIcon icon="x" className="h-4 w-4" />
                </button>
              </div>

              {open ? (
                <div className="flex flex-col gap-4 border-t border-border px-3 py-3">
                  <Field label="Name" required>
                    <Input
                      value={sub.name}
                      onChange={(event) =>
                        patchSubAgent(sub.key, { name: event.target.value })
                      }
                      placeholder="Researcher"
                      className="w-full"
                      disabled={disabled}
                      aria-label="Sub-agent name"
                      data-testid="sub-agent-name"
                    />
                  </Field>
                  <Field label="Description">
                    <Input
                      value={sub.description}
                      onChange={(event) =>
                        patchSubAgent(sub.key, {
                          description: event.target.value,
                        })
                      }
                      placeholder="When the parent should delegate to this sub-agent"
                      className="w-full"
                      disabled={disabled}
                      aria-label="Sub-agent description"
                      data-testid="sub-agent-description"
                    />
                  </Field>
                  <Field label="Model">
                    <ModelSelect
                      models={models}
                      value={sub.model}
                      onModelChange={(model) =>
                        patchSubAgent(sub.key, { model })
                      }
                      includeInherit
                      disabled={disabled}
                      ariaLabel="Sub-agent model"
                    />
                  </Field>
                  <Field label="Instructions">
                    <textarea
                      value={sub.instructions}
                      onChange={(event) =>
                        patchSubAgent(sub.key, {
                          instructions: event.target.value,
                        })
                      }
                      placeholder="What this sub-agent should do when delegated to…"
                      rows={4}
                      disabled={disabled}
                      aria-label="Sub-agent instructions"
                      data-testid="sub-agent-instructions"
                      className="w-full resize-y rounded-md border border-border bg-card px-3 py-2 text-sm text-foreground outline-none transition-colors placeholder:text-muted-foreground focus:border-primary/40"
                    />
                  </Field>
                  <Field label="Tools">
                    <ToolsField
                      groups={SUBAGENT_TOOL_GROUPS}
                      selection={sub.tools}
                      onToolsChange={(tools: ToolSelection) =>
                        patchSubAgent(sub.key, { tools })
                      }
                      disabled={disabled}
                    />
                  </Field>
                </div>
              ) : null}
            </div>
          )
        })}
      </div>

      <div>
        <Button
          type="button"
          variant="secondary"
          size="sm"
          onPress={addSubAgent}
          isDisabled={disabled}
          data-testid="add-sub-agent"
        >
          <AppIcon icon="plus" className="h-4 w-4" />
          Add sub-agent
        </Button>
      </div>
    </div>
  )
}

function Field({
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
