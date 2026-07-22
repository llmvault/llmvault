export type BlogPost = {
  slug: string
  title: string
  excerpt: string
  category: "Agents" | "Engineering" | "Knowledge"
  date: string
  readTime: string
  author: string
  icon: string
  issue: string
}

export const blogPosts: readonly BlogPost[] = [
  {
    slug: "right-model-right-job",
    title: "The right model for every agent is rarely the biggest one",
    excerpt:
      "A practical way to match model cost, context, speed, and reasoning depth to the work an agent actually does.",
    category: "Agents",
    date: "July 18, 2026",
    readTime: "7 min read",
    author: "Ada Nwosu",
    icon: "brain",
    issue: "01",
  },
  {
    slug: "slack-to-finished-work",
    title: "From a Slack mention to finished work, without losing the thread",
    excerpt:
      "How channel routing, shared context, and durable sessions turn everyday messages into reliable agent work.",
    category: "Agents",
    date: "July 14, 2026",
    readTime: "6 min read",
    author: "Hivy team",
    icon: "message-square",
    issue: "02",
  },
  {
    slug: "agent-access-boundaries",
    title: "Give an agent enough access to finish the job, and nothing more",
    excerpt:
      "A team-first approach to connections, knowledge, skills, and the permissions agents inherit at runtime.",
    category: "Agents",
    date: "July 9, 2026",
    readTime: "8 min read",
    author: "Simi Adebayo",
    icon: "shield-check",
    issue: "03",
  },
  {
    slug: "knowledge-before-api",
    title: "Search company knowledge before reaching for another API call",
    excerpt:
      "Why agents should begin with indexed company context, then use live provider tools only when the work needs them.",
    category: "Knowledge",
    date: "July 3, 2026",
    readTime: "5 min read",
    author: "Hivy team",
    icon: "database",
    issue: "04",
  },
  {
    slug: "automation-as-session",
    title: "Every automation should leave behind a session you can inspect",
    excerpt:
      "Triggers are only the start. The useful record is the request, reasoning, tool calls, result, cost, and follow-up in one place.",
    category: "Engineering",
    date: "June 26, 2026",
    readTime: "9 min read",
    author: "Hakaree Jackson",
    icon: "workflow",
    issue: "05",
  },
  {
    slug: "sandbox-secrets",
    title: "Let agents use secrets without teaching them to reveal secrets",
    excerpt:
      "Environment variables can stay opaque while programs still receive the credentials they need inside a sandbox.",
    category: "Engineering",
    date: "June 19, 2026",
    readTime: "6 min read",
    author: "Hivy engineering",
    icon: "key-round",
    issue: "06",
  },
  {
    slug: "structured-agent-memory",
    title: "Agent memory works better when part of it looks like a database",
    excerpt:
      "Sheets give teams a shared, inspectable place for records that need to survive a chat and change over time.",
    category: "Knowledge",
    date: "June 12, 2026",
    readTime: "7 min read",
    author: "Ada Nwosu",
    icon: "table",
    issue: "07",
  },
  {
    slug: "idle-sandbox-economics",
    title: "The quiet economics of ending idle sandboxes quickly",
    excerpt:
      "Small runtime decisions compound across thousands of agent sessions. Here is how we think about the idle tail.",
    category: "Engineering",
    date: "June 5, 2026",
    readTime: "10 min read",
    author: "Hivy engineering",
    icon: "terminal",
    issue: "08",
  },
  {
    slug: "designing-agent-handoffs",
    title: "A good agent handoff answers three questions before work starts",
    excerpt:
      "Who owns the request, what context travels with it, and where the answer returns determine whether the workflow holds together.",
    category: "Agents",
    date: "May 28, 2026",
    readTime: "5 min read",
    author: "Simi Adebayo",
    icon: "route",
    issue: "09",
  },
  {
    slug: "operating-ai-team",
    title: "What changes when you operate AI agents as a team resource",
    excerpt:
      "The unit of management shifts from isolated chats to shared agents, capabilities, knowledge, and observable runs.",
    category: "Agents",
    date: "May 21, 2026",
    readTime: "11 min read",
    author: "Hivy team",
    icon: "users",
    issue: "10",
  },
]

export const blogCategories = [
  "All",
  "Agents",
  "Engineering",
  "Knowledge",
] as const

export type BlogCategory = (typeof blogCategories)[number]

export function getBlogPost(slug: string) {
  return blogPosts.find((post) => post.slug === slug)
}
