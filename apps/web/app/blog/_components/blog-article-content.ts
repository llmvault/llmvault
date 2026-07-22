import type { BlogPost } from "./blog-content"

export type BlogArticleSection = {
  id: string
  heading: string
  paragraphs: readonly string[]
  points?: readonly string[]
}

export type BlogArticleContent = {
  summary: readonly string[]
  sections: readonly BlogArticleSection[]
}

const rightModelArticle: BlogArticleContent = {
  summary: [
    "Use the least expensive model that clears the quality bar for the job.",
    "Judge models on the work your agent performs, not a public leaderboard.",
    "Track corrections beside token cost so cheap mistakes don't look efficient.",
    "Keep the agent role stable when you test or replace its model.",
  ],
  sections: [
    {
      id: "bigger-models-hide-bad-job-design",
      heading: "Bigger models hide bad job design",
      paragraphs: [
        "A model upgrade can rescue a vague instruction, but it also makes the job harder to understand. If a support agent receives every ticket, every document, and an open-ended request to help, a larger model may produce a better answer. It will also charge you for reading context that should never have reached the session.",
        "Start by narrowing the work. Give the agent one role, a clear finish condition, and access to the few sources it needs. Once the request is bounded, model choice becomes a practical decision instead of a guess about which name sits highest on a benchmark chart.",
      ],
    },
    {
      id: "test-the-job-you-have",
      heading: "Test the job you actually have",
      paragraphs: [
        "Public evaluations measure useful things, but they don't measure your approval rules, your customer vocabulary, or the odd shape of your internal data. Build a small evaluation set from real work. Remove secrets and personal data, then keep the requests that exposed meaningful differences between models.",
        "Run each candidate with the same agent instructions and tools. Record the answer, token cost, latency, tool failures, and whether a person had to repair the result. Ten representative requests will tell you more than a broad benchmark that never touches your workflow.",
      ],
      points: [
        "Routine classification and extraction usually reward speed and low cost.",
        "Long account reviews need enough context without paying for unused reasoning.",
        "Code changes need models that can inspect files and follow tool feedback.",
        "Sensitive decisions need a stricter quality bar and human review.",
      ],
    },
    {
      id: "give-each-agent-a-budget",
      heading: "Give each agent a budget",
      paragraphs: [
        "Cost limits work best at the agent level because jobs consume tokens differently. A ticket triage agent may run hundreds of short sessions. A research agent might run twice a day and read far more material each time. One workspace-wide model policy treats those jobs as though they have the same economics.",
        "Set a target cost for a successful run, then watch the distribution rather than one average. A sudden tail of expensive sessions often points to oversized context, repeated tool calls, or retries that the agent cannot resolve. Changing the model can help, but first check what the session was asked to carry.",
      ],
    },
    {
      id: "change-model-not-role",
      heading: "Change the model, not the role",
      paragraphs: [
        "The agent should remain the stable unit. Its instructions, connected tools, knowledge, and team access describe the job; the model is one replaceable part. That separation lets you test a faster model on support work without rebuilding the support agent or moving its history somewhere else.",
        "When a provider lowers prices or a smaller model improves, rerun the same evaluation set. If quality holds, switch the assignment. Your teammates still talk to the same agent, completed sessions stay together, and the savings appear on the next run.",
      ],
    },
  ],
}

const categoryFallbacks: Record<BlogPost["category"], BlogArticleContent> = {
  Agents: {
    summary: [
      "Give the agent one named job and a finish condition.",
      "Keep the request, work, and answer in the same session.",
      "Attach access through the team that owns the agent.",
      "Review completed runs before changing the role.",
    ],
    sections: [
      {
        id: "start-with-ownership",
        heading: "Start with ownership",
        paragraphs: [
          "An agent becomes useful when a team knows which requests belong to it and what a finished answer looks like. Name the job plainly, assign it to the team that owns the outcome, and keep unrelated tools out of reach.",
          "That boundary makes failures easier to read. A missed request points to routing. A weak answer points to instructions, context, or model choice. Without ownership, every problem looks like an agent problem.",
        ],
      },
      {
        id: "keep-the-work-together",
        heading: "Keep the work together",
        paragraphs: [
          "A request should open one durable session. Tool calls, files, costs, replies, and follow-up work belong there, so a teammate can inspect what happened without reconstructing the run from several systems.",
        ],
      },
    ],
  },
  Engineering: {
    summary: [
      "Treat every automated run as an inspectable session.",
      "Pass secrets to programs without printing their values.",
      "End idle compute quickly and preserve the useful output.",
      "Measure retries and tool failures beside model cost.",
    ],
    sections: [
      {
        id: "make-runtime-visible",
        heading: "Make runtime behaviour visible",
        paragraphs: [
          "Agent infrastructure gets expensive in the gaps: idle sandboxes, repeated tool calls, oversized context, and retries that never change the plan. A completed answer hides those details unless the run keeps them attached.",
          "Store the request, tool activity, duration, model cost, and final state as one session. Engineers can then fix the part that failed instead of replacing the whole agent.",
        ],
      },
      {
        id: "protect-the-boundaries",
        heading: "Protect the runtime boundaries",
        paragraphs: [
          "Programs may need credentials, but the agent doesn't need to read their values. Provide variable names, pass values through the sandbox environment, and block commands that expose the environment in logs or messages.",
        ],
      },
    ],
  },
  Knowledge: {
    summary: [
      "Index only the sources a team has approved.",
      "Search indexed knowledge before reading a provider directly.",
      "Use live provider tools when freshness or a write action requires them.",
      "Keep structured records in sheets when they change between sessions.",
    ],
    sections: [
      {
        id: "search-approved-context-first",
        heading: "Search approved context first",
        paragraphs: [
          "An agent should begin with the knowledge its team has already selected. Indexed sources are faster to search, easier to scope, and less likely to pull unrelated company material into a session.",
          "Provider tools still matter when the request needs current state or an authorized write. The order is what changes: use the team's known context first, then reach outward when the indexed answer is missing or stale.",
        ],
      },
      {
        id: "choose-the-right-memory",
        heading: "Choose the right kind of memory",
        paragraphs: [
          "Documents work for policies, notes, and reference material. Records that change often belong in a sheet where agents and teammates can read the same rows, update fields, and see what changed after a run.",
        ],
      },
    ],
  },
}

export function getBlogArticleContent(post: BlogPost): BlogArticleContent {
  if (post.slug === "right-model-right-job") return rightModelArticle
  return categoryFallbacks[post.category]
}
