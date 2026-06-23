// Static agent (specialist) catalog for the /w design exploration. Mirrors
// the backend specialist definitions: each agent owns its tools and the set
// of models it can run on. The agent is fixed for the lifetime of a session;
// the model can be switched mid-session, but only within the agent's list.

export interface AgentModel {
  id: string
  label: string
  provider: string
}

export const MODELS: AgentModel[] = [
  {
    id: "claude-opus",
    label: "Claude Opus 4.8",
    provider: "Anthropic",
  },
  {
    id: "claude-sonnet",
    label: "Claude Sonnet 4.6",
    provider: "Anthropic",
  },
  {
    id: "gemini-pro",
    label: "Gemini 2.5 Pro",
    provider: "Google",
  },
  {
    id: "deepseek-v3",
    label: "DeepSeek V3.2",
    provider: "DeepSeek",
  },
  { id: "grok-4", label: "Grok 4", provider: "xAI" },
  { id: "qwen-max", label: "Qwen3 Max", provider: "Qwen" },
  { id: "kimi-k2", label: "Kimi K2", provider: "MoonshotAI" },
]

export function modelById(id: string): AgentModel {
  const model = MODELS.find((entry) => entry.id === id)
  if (!model) throw new Error(`Unknown model: ${id}`)
  return model
}

export interface Agent {
  id: string
  name: string
  description: string
  icon: string
  tools: string[]
  modelIds: string[]
  defaultModelId: string
  placeholder: string
}

export const AGENTS: Agent[] = [
  {
    id: "software-engineer",
    name: "Software Engineer",
    description:
      "Codes in a cloud sandbox with full repository access. Runs commands, browses the web, and opens pull requests.",
    icon: "lucide:code-xml",
    tools: ["Terminal", "Browser", "Git", "Code editor"],
    modelIds: ["claude-opus", "claude-sonnet", "gemini-pro", "grok-4"],
    defaultModelId: "claude-opus",
    placeholder: "Describe a bug to fix or a feature to build…",
  },
  {
    id: "research-analyst",
    name: "Research Analyst",
    description:
      "Searches the web, reads sources end to end, and writes cited reports and briefs into your workspace drive.",
    icon: "lucide:telescope",
    tools: ["Web search", "Browser", "Drive"],
    modelIds: ["claude-opus", "gemini-pro", "deepseek-v3"],
    defaultModelId: "claude-opus",
    placeholder: "What should I research for you?",
  },
  {
    id: "data-analyst",
    name: "Data Analyst",
    description:
      "Queries your connected databases and warehouses, explores datasets, and produces charts and summaries.",
    icon: "lucide:chart-spline",
    tools: ["SQL", "Notebooks", "Charts", "Drive"],
    modelIds: ["claude-sonnet", "gemini-pro", "qwen-max"],
    defaultModelId: "claude-sonnet",
    placeholder: "Ask a question about your data…",
  },
  {
    id: "content-writer",
    name: "Content Writer",
    description:
      "Drafts docs, posts, and announcements in your voice. Works from briefs, research notes, and past writing.",
    icon: "lucide:pen-line",
    tools: ["Drive", "Web search"],
    modelIds: ["claude-sonnet", "kimi-k2", "deepseek-v3"],
    defaultModelId: "claude-sonnet",
    placeholder: "What are we writing today?",
  },
  {
    id: "support-engineer",
    name: "Support Engineer",
    description:
      "Investigates customer issues across your tools — reads tickets, checks logs and dashboards, and drafts replies.",
    icon: "lucide:headset",
    tools: ["Integrations", "Logs", "Browser"],
    modelIds: ["claude-sonnet", "qwen-max", "gemini-pro"],
    defaultModelId: "claude-sonnet",
    placeholder: "Paste a ticket or describe the issue…",
  },
]

export const DEFAULT_AGENT_ID = "software-engineer"

export function agentById(id: string): Agent {
  const agent = AGENTS.find((entry) => entry.id === id)
  if (!agent) throw new Error(`Unknown agent: ${id}`)
  return agent
}
