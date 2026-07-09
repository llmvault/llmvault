import type { components } from "@/lib/api/schema"
import type { AgentSandboxImage, AgentSandboxSize } from "../_lib"

export type ModelSummary = components["schemas"]["modelSummary"]
type AgentCreateBody = components["schemas"]["agentMutationRequest"]
export type AgentDetail = components["schemas"]["agentListItem"]
type SubAgentDetail = components["schemas"]["subAgentResponse"]
type ToolFilter = components["schemas"]["ToolFilter"]

// Default model for a new agent.
export const DEFAULT_AGENT_MODEL = "deepseek-v4-flash"

// A tool the agent can be granted. "runtime" tools are the built-in runtime
// tools (model.RuntimeBuiltInToolIDs); "mcp" tools are gated via mcp_tool_filter.
// hideForSubAgent tools are only offered on the top-level agent.
type ToolKind = "runtime" | "mcp"
type ToolDef = {
  id: string
  label: string
  kind: ToolKind
  hideForSubAgent?: boolean
}
export type ToolGroup = { title: string; tools: ToolDef[] }

const TOOL_GROUPS: ToolGroup[] = [
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

const RUNTIME_TOOL_IDS: string[] = ALL_TOOLS.filter(
  (tool) => tool.kind === "runtime"
).map((tool) => tool.id)

const MCP_TOOL_IDS: string[] = ALL_TOOLS.filter(
  (tool) => tool.kind === "mcp"
).map((tool) => tool.id)

// The tool a parent must have to dispatch to its sub-agents. Kept locked-on when
// the agent has sub-agents (mirrors the backend, which force-enables it).
export const SUBAGENT_TASK_TOOL = "subagent_task"

export type ToolSelection = Record<string, boolean>

// Everything defaults on (matching the backend: full runtime toolset and all MCP
// tools allowed unless denied).
function allToolsSelected(): ToolSelection {
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
// so only unchecked tools are denied). Always sent so an edit can also clear a
// previously-set filter; an empty deny list normalizes to "no filter" server-side.
function mcpToolFilterFor(selection: ToolSelection): { deny: string[] } {
  return { deny: MCP_TOOL_IDS.filter((id) => !selection[id]) }
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
  instructions: string
  tools: ToolSelection
  sandboxImage: AgentSandboxImage
  sandboxSize: AgentSandboxSize
  subAgents: SubAgentForm[]
  teamId: string
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
    instructions: "",
    tools: allToolsSelected(),
    sandboxImage: "default",
    sandboxSize: "small",
    subAgents: [],
    teamId: "",
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

function canSubmitAgent(form: AgentForm): boolean {
  return (
    form.name.trim().length > 0 &&
    form.model.trim().length > 0 &&
    subAgentNameError(form.subAgents) === null
  )
}

// Rebuilds the tool selection from a saved agent: runtime tools come from the
// stored tools map, MCP tools are on unless present in the mcp_tool_filter deny.
function toolSelectionFromAgent(
  tools: Record<string, unknown> | undefined,
  mcpFilter: ToolFilter | undefined
): ToolSelection {
  const selection: ToolSelection = {}
  for (const id of RUNTIME_TOOL_IDS) selection[id] = Boolean(tools?.[id])
  const deny = new Set(mcpFilter?.deny ?? [])
  for (const id of MCP_TOOL_IDS) selection[id] = !deny.has(id)
  return selection
}

function subAgentFormFromDetail(
  sub: SubAgentDetail,
  parentModel: string,
  index: number
): SubAgentForm {
  // A sub-agent model equal to the parent's is shown as "inherit".
  const subModel = (sub.model ?? "").trim()
  return {
    key: sub.id || `sub-${index}`,
    name: sub.name ?? "",
    description: sub.description ?? "",
    model: subModel && subModel !== parentModel ? subModel : "",
    instructions: sub.instructions ?? "",
    tools: toolSelectionFromAgent(
      sub.tools as Record<string, unknown> | undefined,
      sub.mcp_tool_filter
    ),
  }
}

// Maps a saved agent into editable form state. The agent read type carries
// `team_id`, so the team picker preselects the agent's current team.
export function agentFormFromDetail(agent: AgentDetail): AgentForm {
  const model = agent.model ?? ""
  return {
    name: agent.name ?? "",
    description: agent.description ?? "",
    icon: agent.icon ?? "",
    model,
    instructions: agent.instructions ?? "",
    tools: toolSelectionFromAgent(
      agent.tools as Record<string, unknown> | undefined,
      agent.mcp_tool_filter
    ),
    sandboxImage: (agent.sandbox_image as AgentSandboxImage) || "default",
    sandboxSize: (agent.sandbox_size as AgentSandboxSize) || "small",
    subAgents: (agent.sub_agents ?? []).map((sub, index) =>
      subAgentFormFromDetail(sub, model, index)
    ),
    teamId: agent.team_id ?? "",
  }
}

export function buildCreateBody(form: AgentForm): AgentCreateBody {
  return {
    name: form.name.trim(),
    description: form.description.trim() || undefined,
    icon: form.icon.trim() || undefined,
    model: form.model,
    instructions: form.instructions.trim() || undefined,
    tools: runtimeToolsMap(form.tools),
    mcp_tool_filter: mcpToolFilterFor(form.tools),
    sandbox_image: form.sandboxImage,
    sandbox_size: form.sandboxSize,
    team_id: form.teamId || undefined,
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
