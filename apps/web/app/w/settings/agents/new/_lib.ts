import type { components } from "@/lib/api/schema"
import type { AgentSandboxImage, AgentSandboxSize } from "../_lib"

export type ModelSummary = components["schemas"]["modelSummary"]
export type AgentCreateBody = components["schemas"]["agentMutationRequest"]

// Default model for a new agent.
export const DEFAULT_AGENT_MODEL = "deepseek-v4-flash"

// A tool the agent can be granted. "runtime" tools are the built-in runtime
// tools (model.RuntimeBuiltInToolIDs); "mcp" tools are gated via mcp_tool_filter.
// hideForSubAgent tools are only offered on the top-level agent.
export type ToolKind = "runtime" | "mcp"
export type ToolDef = {
  id: string
  label: string
  kind: ToolKind
  hideForSubAgent?: boolean
}
export type ToolGroup = { title: string; tools: ToolDef[] }

export const TOOL_GROUPS: ToolGroup[] = [
  {
    title: "Filesystem & code",
    tools: [
      { id: "read_file", label: "Read file", kind: "runtime" },
      { id: "write_file", label: "Write file", kind: "runtime" },
      { id: "apply_patch", label: "Apply patch", kind: "runtime" },
      { id: "file_search", label: "File search", kind: "runtime" },
      { id: "glob", label: "Glob", kind: "runtime" },
      { id: "grep", label: "Grep", kind: "runtime" },
      { id: "multi_grep", label: "Multi-grep", kind: "runtime" },
      { id: "lsp", label: "LSP", kind: "runtime" },
    ],
  },
  {
    title: "Shell",
    tools: [
      { id: "bash", label: "Bash", kind: "runtime" },
      { id: "check_bash_status", label: "Bash status", kind: "runtime" },
    ],
  },
  {
    title: "Skills",
    tools: [
      { id: "skills_list", label: "List skills", kind: "runtime" },
      { id: "skill_view", label: "View skill", kind: "runtime" },
    ],
  },
  {
    title: "Image & web",
    tools: [
      { id: "generate_image", label: "Generate image", kind: "mcp" },
      {
        id: "generate_vector_image",
        label: "Generate vector image",
        kind: "mcp",
      },
      { id: "web_search", label: "Web search", kind: "mcp" },
      { id: "web_fetch", label: "Fetch webpage", kind: "mcp" },
    ],
  },
  {
    title: "Orchestration",
    tools: [
      {
        id: "subagent_task",
        label: "Delegate to sub-agent",
        kind: "runtime",
        hideForSubAgent: true,
      },
      {
        id: "update_plan",
        label: "Update plan",
        kind: "runtime",
        hideForSubAgent: true,
      },
      {
        id: "search_sessions",
        label: "Search sessions",
        kind: "runtime",
        hideForSubAgent: true,
      },
      {
        id: "request_user_input",
        label: "Ask user",
        kind: "runtime",
        hideForSubAgent: true,
      },
    ],
  },
]

const ALL_TOOLS: ToolDef[] = TOOL_GROUPS.flatMap((group) => group.tools)

// Tool groups shown for a given context. Sub-agents hide orchestration tools.
export function toolGroupsFor(context: "agent" | "subagent"): ToolGroup[] {
  if (context === "agent") return TOOL_GROUPS
  return TOOL_GROUPS.map((group) => ({
    ...group,
    tools: group.tools.filter((tool) => !tool.hideForSubAgent),
  })).filter((group) => group.tools.length > 0)
}

export const RUNTIME_TOOL_IDS: string[] = ALL_TOOLS.filter(
  (tool) => tool.kind === "runtime"
).map((tool) => tool.id)

export const MCP_TOOL_IDS: string[] = ALL_TOOLS.filter(
  (tool) => tool.kind === "mcp"
).map((tool) => tool.id)

// The tool a parent must have to dispatch to its sub-agents. Kept locked-on when
// the agent has sub-agents (mirrors the backend, which force-enables it).
export const SUBAGENT_TASK_TOOL = "subagent_task"

export type ToolSelection = Record<string, boolean>

// Everything defaults on (matching the backend: full runtime toolset and all MCP
// tools allowed unless denied).
export function allToolsSelected(): ToolSelection {
  return Object.fromEntries(
    [...RUNTIME_TOOL_IDS, ...MCP_TOOL_IDS].map((id) => [id, true])
  )
}

// Enabled runtime tools, as a { id: true } map (agents.tools on the backend).
// MCP tools are excluded — they are controlled via mcp_tool_filter instead.
function runtimeToolsMap(selection: ToolSelection): Record<string, boolean> {
  const out: Record<string, boolean> = {}
  for (const id of RUNTIME_TOOL_IDS) {
    if (selection[id]) out[id] = true
  }
  return out
}

// MCP tools the user turned off, expressed as a deny list (default = all allowed,
// so only unchecked tools are denied). Returns undefined when nothing is denied.
export function mcpToolFilterFor(
  selection: ToolSelection
): { deny: string[] } | undefined {
  const deny = MCP_TOOL_IDS.filter((id) => !selection[id])
  return deny.length > 0 ? { deny } : undefined
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
    tools: runtimeToolsMap(form.tools),
    mcp_tool_filter: mcpToolFilterFor(form.tools),
    sandbox_image: form.sandboxImage,
    sandbox_size: form.sandboxSize,
    sub_agents: form.subAgents.map((sub) => {
      const tools = runtimeToolsMap(sub.tools)
      return {
        name: sub.name.trim(),
        description: sub.description.trim() || undefined,
        instructions: sub.instructions.trim() || undefined,
        model: sub.model.trim() || undefined,
        tools: Object.keys(tools).length > 0 ? tools : undefined,
        mcp_tool_filter: mcpToolFilterFor(sub.tools),
      }
    }),
  }
}
