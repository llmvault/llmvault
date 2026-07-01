import type { components } from "@/lib/api/schema"
import type { AgentSandboxImage, AgentSandboxSize } from "../_lib"

export type ModelSummary = components["schemas"]["modelSummary"]
export type AgentCreateBody = components["schemas"]["agentMutationRequest"]

// Runtime built-in tools (mirrors model.RuntimeBuiltInToolIDs on the backend),
// grouped for display in the tool picker.
export type ToolDef = { id: string; label: string }
export type ToolGroup = { title: string; tools: ToolDef[] }

export const TOOL_GROUPS: ToolGroup[] = [
  {
    title: "Filesystem & code",
    tools: [
      { id: "read_file", label: "Read file" },
      { id: "write_file", label: "Write file" },
      { id: "apply_patch", label: "Apply patch" },
      { id: "file_search", label: "File search" },
      { id: "glob", label: "Glob" },
      { id: "grep", label: "Grep" },
      { id: "multi_grep", label: "Multi-grep" },
      { id: "lsp", label: "LSP" },
    ],
  },
  {
    title: "Shell",
    tools: [
      { id: "bash", label: "Bash" },
      { id: "check_bash_status", label: "Bash status" },
    ],
  },
  {
    title: "Skills",
    tools: [
      { id: "skills_list", label: "List skills" },
      { id: "skill_view", label: "View skill" },
      { id: "skill_manage", label: "Manage skills" },
    ],
  },
  {
    title: "Orchestration",
    tools: [
      { id: "subagent_task", label: "Delegate to sub-agent" },
      { id: "update_plan", label: "Update plan" },
      { id: "search_sessions", label: "Search sessions" },
      { id: "request_user_input", label: "Ask user" },
    ],
  },
]

export const ALL_TOOL_IDS: string[] = TOOL_GROUPS.flatMap((group) =>
  group.tools.map((tool) => tool.id)
)

// The tool a parent must have to dispatch to its sub-agents. Kept locked-on when
// the agent has sub-agents (mirrors the backend, which force-enables it).
export const SUBAGENT_TASK_TOOL = "subagent_task"

export type ToolSelection = Record<string, boolean>

export function allToolsSelected(): ToolSelection {
  return Object.fromEntries(ALL_TOOL_IDS.map((id) => [id, true]))
}

export function selectedToolCount(selection: ToolSelection): number {
  return ALL_TOOL_IDS.filter((id) => selection[id]).length
}

export function selectedToolLabels(selection: ToolSelection): string[] {
  return TOOL_GROUPS.flatMap((group) => group.tools).filter(
    (tool) => selection[tool.id]
  ).map((tool) => tool.label)
}

// Only enabled tools are sent, as a { id: true } map (model.JSON on the backend).
function toSelectionMap(selection: ToolSelection): Record<string, boolean> {
  const out: Record<string, boolean> = {}
  for (const id of ALL_TOOL_IDS) {
    if (selection[id]) out[id] = true
  }
  return out
}

export type SubAgentForm = {
  key: string
  name: string
  description: string
  model: string // "" = inherit the parent's model
  instructions: string
  tools: ToolSelection
}

export type AgentForm = {
  name: string
  description: string
  icon: string
  model: string
  availableModels: string[]
  instructions: string
  tools: ToolSelection
  sandboxImage: AgentSandboxImage
  sandboxSize: AgentSandboxSize
  pluginSlugs: string[]
  subAgents: SubAgentForm[]
}

let subAgentSeq = 0

export function emptySubAgent(): SubAgentForm {
  subAgentSeq += 1
  return {
    key: `sub-${subAgentSeq}`,
    name: "",
    description: "",
    model: "",
    instructions: "",
    tools: {},
  }
}

export function emptyAgentForm(): AgentForm {
  return {
    name: "",
    description: "",
    icon: "",
    model: "",
    availableModels: [],
    instructions: "",
    tools: allToolsSelected(),
    sandboxImage: "default",
    sandboxSize: "small",
    pluginSlugs: [],
    subAgents: [],
  }
}

export function subAgentNameError(subAgents: SubAgentForm[]): string | null {
  const seen = new Set<string>()
  for (const sub of subAgents) {
    const name = sub.name.trim()
    if (!name) return "Every sub-agent needs a name."
    if (seen.has(name)) return `Duplicate sub-agent name "${name}".`
    seen.add(name)
  }
  return null
}

export function canSubmitAgent(form: AgentForm): boolean {
  return (
    form.name.trim().length > 0 &&
    form.model.trim().length > 0 &&
    subAgentNameError(form.subAgents) === null
  )
}

export function buildCreateBody(form: AgentForm): AgentCreateBody {
  const availableModels = form.availableModels.includes(form.model)
    ? form.availableModels
    : [form.model, ...form.availableModels]

  return {
    name: form.name.trim(),
    description: form.description.trim() || undefined,
    icon: form.icon.trim() || undefined,
    model: form.model,
    available_models: availableModels,
    instructions: form.instructions.trim() || undefined,
    tools: toSelectionMap(form.tools),
    sandbox_image: form.sandboxImage,
    sandbox_size: form.sandboxSize,
    sub_agents: form.subAgents.map((sub) => {
      const tools = toSelectionMap(sub.tools)
      return {
        name: sub.name.trim(),
        description: sub.description.trim() || undefined,
        instructions: sub.instructions.trim() || undefined,
        model: sub.model.trim() || undefined,
        tools: Object.keys(tools).length > 0 ? tools : undefined,
      }
    }),
  }
}
