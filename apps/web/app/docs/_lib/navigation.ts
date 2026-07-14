export type DocPage = {
  slug: string
  title: string
  description: string
}

type DocSection = {
  title: string
  pages: DocPage[]
}

export const DOC_SECTIONS: DocSection[] = [
  {
    title: "Get started",
    pages: [
      {
        slug: "welcome-to-hivy",
        title: "Welcome to Hivy",
        description:
          "Hivy gives each team its own agents. They can use approved tools and knowledge, then run again on a schedule or external event.",
      },
      {
        slug: "run-your-first-agent",
        title: "Run your first agent",
        description:
          "Pick the agent that fits the job and a model that won't waste money. Then send your first task.",
      },
      {
        slug: "how-hivy-organizes-work",
        title: "How Hivy organizes work",
        description:
          "See where a session lives and who can reach it. The same boundaries stop agents from roaming across the workspace.",
      },
    ],
  },
  {
    title: "Workspace and access",
    pages: [
      {
        slug: "workspace-and-access/teams",
        title: "Teams",
        description:
          "A team keeps its members and agents on the same work while limiting which tools and knowledge they can reach.",
      },
      {
        slug: "workspace-and-access/channels",
        title: "Channels",
        description:
          "Keep related agent sessions in one place, with the team and default agent already set.",
      },
      {
        slug: "workspace-and-access/roles-and-invitations",
        title: "Access control",
        description:
          "Set a workspace role when you invite someone, then add the teams they need. Private channels keep their own member lists.",
      },
      {
        slug: "workspace-and-access/slack-channels",
        title: "Connect a Slack channel",
        description:
          "Link a public Slack channel to a Hivy team. Each @Hivy thread becomes a session, and the team's access limits still apply.",
      },
    ],
  },
  {
    title: "Agents",
    pages: [
      {
        slug: "agents/agent-catalog",
        title: "Agents and the agent catalog",
        description:
          "Find a specialist that already knows the job. Check what it requires before installing a copy for one team.",
      },
      {
        slug: "agents/configure-an-agent",
        title: "Create and configure an agent",
        description:
          "Define the agent's job and owning team, then choose the model and tools it may use.",
      },
      {
        slug: "agents/tools-and-sub-agents",
        title: "Tools, sub-agents, and sandboxes",
        description:
          "Control what an agent can run and which helpers receive delegated work. Sandbox settings decide what survives between sessions.",
      },
      {
        slug: "agents/sessions",
        title: "Agent sessions",
        description:
          "Send a task inside a channel and watch the agent work. Follow-up requests keep the original context.",
      },
      {
        slug: "agents/generated-work",
        title: "Generated files and agent work",
        description:
          "Track the files and artifacts an agent creates while its tool calls and delegated work remain visible in the session.",
      },
    ],
  },
  {
    title: "Plugins and connections",
    pages: [
      {
        slug: "plugins-and-connections/how-access-works",
        title: "Connections and team access",
        description:
          "A connection belongs to a person; teams receive plugin access, and agents inherit only what their team allows.",
      },
      {
        slug: "plugins-and-connections/connect-tools",
        title: "Connect and configure your tools",
        description:
          "Connect the account a plugin needs and restrict the resources it may touch. Team switches control where the plugin appears.",
      },
    ],
  },
  {
    title: "Knowledge and memory",
    pages: [
      {
        slug: "knowledge-and-memory/knowledge-sources",
        title: "Knowledge sources",
        description:
          "Index uploaded files and selected content from connected services. Grant the finished source to specific teams.",
      },
      {
        slug: "knowledge-and-memory/indexing-and-access",
        title: "Knowledge access control",
        description:
          "Limit each source to chosen resources and teams. Check sync health or indexed documents when results look wrong.",
      },
      {
        slug: "knowledge-and-memory/memories-and-rules",
        title: "Memories and rules",
        description:
          "See what a channel's sessions taught Hivy. Confirm or fix what it learned; pinned rules apply to every future session.",
      },
    ],
  },
  {
    title: "Automations",
    pages: [
      {
        slug: "automations/overview",
        title: "Automations overview",
        description:
          "Start an agent from a connected-app event, a recurring schedule, or an incoming HTTP request.",
      },
      {
        slug: "automations/event-triggers",
        title: "Connected-app event triggers",
        description:
          "Use events from a connected service to start an agent session and include the event payload.",
      },
      {
        slug: "automations/schedules",
        title: "Schedules",
        description:
          "Give an agent a recurring task and choose when it runs. Hivy creates each result as a session in the selected channel.",
      },
      {
        slug: "automations/http-webhooks",
        title: "HTTP webhooks",
        description:
          "Give an outside system a private URL that starts an agent session when it receives an authenticated POST request.",
      },
      {
        slug: "automations/runs-and-troubleshooting",
        title: "Runs and troubleshooting",
        description:
          "Each run links to the session it created and shows its status. Failed deliveries keep the error needed to trace the problem.",
      },
    ],
  },
  {
    title: "Artifacts",
    pages: [
      {
        slug: "sheets-and-apps/sheets",
        title: "Sheets",
        description:
          "Sheets are channel databases that agents can update across sessions. People can inspect and edit the same records.",
      },
      {
        slug: "sheets-and-apps/agent-built-apps",
        title: "Agent-built apps",
        description:
          "Ask Ricky to build an app around any data source you choose, including Hivy Sheets and external databases. It can also call outside services.",
      },
    ],
  },
  {
    title: "Administration",
    pages: [
      {
        slug: "administration/usage-and-billing",
        title: "Usage, plans, and billing",
        description:
          "See how much credit the workspace has and what each session spent. Compare plan changes before checkout.",
      },
    ],
  },
]

export const DOC_PAGES = DOC_SECTIONS.flatMap((section) =>
  section.pages.map((page) => ({ ...page, section: section.title }))
)

export function getDocPage(slug: string) {
  return DOC_PAGES.find((page) => page.slug === slug)
}

export function getAdjacentDocPages(slug: string) {
  const index = DOC_PAGES.findIndex((page) => page.slug === slug)
  if (index < 0) return { previous: undefined, next: undefined }
  return {
    previous: DOC_PAGES[index - 1],
    next: DOC_PAGES[index + 1],
  }
}

export function searchDocPages(query: string) {
  const normalized = query.trim().toLowerCase()
  if (!normalized) return []
  return DOC_PAGES.filter((page) =>
    `${page.title} ${page.description} ${page.section}`
      .toLowerCase()
      .includes(normalized)
  )
}
