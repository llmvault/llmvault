export type PluginCategory =
  | "All"
  | "Featured"
  | "Business & Operations"
  | "Communication"
  | "Creativity"
  | "Data & Analytics"
  | "Developer Tools"
  | "Education & Research"
  | "Finance"
  | "Other"
  | "Productivity"
  | "Research"
  | "Security"
  | "Travel"

export interface Plugin {
  id: string
  name: string
  description: string
  category: PluginCategory
  detailCategory?: PluginCategory
  icon: string
  iconColor: string
  official?: boolean
  developer?: string
  capabilities?: string[]
  examples?: string[]
  skills?: Array<{ name: string; description: string }>
  links?: {
    website?: string
    privacy?: string
    terms?: string
  }
  longDescription?: string
}

export const CATEGORIES: PluginCategory[] = [
  "All",
  "Featured",
  "Business & Operations",
  "Communication",
  "Creativity",
  "Data & Analytics",
  "Developer Tools",
  "Education & Research",
  "Finance",
  "Other",
  "Productivity",
  "Research",
  "Security",
  "Travel",
]

export const SOURCES = [
  { id: "curated", label: "Curated by Hivy" },
  { id: "official", label: "hivy-official" },
]

export const CONNECTED_APPS = [
  { id: "notion", name: "Notion", color: "#000000", icon: "simple-icons:notion" },
  { id: "linear", name: "Linear", color: "#5E6AD2", icon: "simple-icons:linear" },
  { id: "slack", name: "Slack", color: "#4A154B", icon: "simple-icons:slack" },
  { id: "github", name: "GitHub", color: "#181717", icon: "simple-icons:github" },
  { id: "hubspot", name: "HubSpot", color: "#FF7A59", icon: "simple-icons:hubspot" },
  { id: "salesforce", name: "Salesforce", color: "#00A1E0", icon: "simple-icons:salesforce" },
  { id: "attio", name: "Attio", color: "#111111", icon: "lucide:database" },
  { id: "pipedrive", name: "Pipedrive", color: "#008542", icon: "lucide:briefcase" },
]

export const FEATURED_PLUGINS: Plugin[] = [
  {
    id: "product-design",
    name: "Product Design",
    description: "Explore and prototype ideas",
    category: "Featured",
    detailCategory: "Creativity",
    icon: "lucide:component",
    iconColor: "#A855F7",
    official: true,
    developer: "OpenAI",
    capabilities: ["Interactive", "Read", "Write"],
    examples: [
      "Help me get started",
      "Turn this product idea into three visual directions",
      "Clone this URL into an editable prototype",
    ],
    skills: [
      { name: "audit", description: "Audit or critique product UX and design from captured screenshots" },
      { name: "design-qa", description: "Internal prototype QA comparison against a visual source" },
      { name: "get-context", description: "Confirm the Product Design brief before design or build work" },
      { name: "ideate", description: "Generate visual directions after brief confirmation" },
      { name: "image-to-code", description: "Build a responsive frontend from a selected visual target after brief confirmation" },
    ],
    links: {
      website: "#",
      privacy: "#",
      terms: "#",
    },
    longDescription:
      "Turn early ideas into prototypes teams can review. Explore product directions, audit user flows, research user friction, prototype from a live URL, and make static screenshots interactive. Start with a written brief, URL, screenshot, or existing design, then compare visual directions before building. Use available browser, Figma, Canva, image generation, and hosting tools to gather references, create concepts, review the result, and carry the work forward with your team.",
  },
  {
    id: "creative-production",
    name: "Creative Production",
    description: "Create marketing visuals from a brief or product image.",
    category: "Featured",
    icon: "lucide:sparkles",
    iconColor: "#8B5CF6",
    official: true,
    developer: "Hivy",
    capabilities: ["Interactive", "Read", "Write"],
    examples: [
      "Generate a campaign hero image from this brief",
      "Resize these creatives for Instagram Stories",
      "Create a product shot on a clean background",
    ],
    skills: [
      { name: "generate", description: "Create images from text prompts or reference assets" },
      { name: "edit", description: "Apply style, background, or composition changes to visuals" },
      { name: "resize", description: "Adapt creatives to common platform formats" },
    ],
    links: {
      website: "#",
      privacy: "#",
      terms: "#",
    },
    longDescription:
      "Creative Production helps marketing and design teams generate and refine visuals directly from a brief, product image, or reference. Produce campaign assets, social posts, and product photography without leaving Hivy.",
  },
  {
    id: "sales",
    name: "Sales",
    description: "Prepare sales work faster",
    category: "Featured",
    icon: "lucide:trending-up",
    iconColor: "#F97316",
    official: true,
    developer: "Hivy",
    capabilities: ["Read", "Write"],
    examples: [
      "Draft a follow-up email for this opportunity",
      "Summarize the latest account activity",
      "Prepare talking points for tomorrow's demo",
    ],
    skills: [
      { name: "draft", description: "Generate outreach, follow-ups, and proposals" },
      { name: "summarize", description: "Condense account activity and call notes" },
      { name: "prep", description: "Build agendas and talking points for meetings" },
    ],
    links: {
      website: "#",
      privacy: "#",
      terms: "#",
    },
    longDescription:
      "The Sales plugin accelerates deal workflows by drafting outreach, summarizing account activity, and preparing talking points from your CRM and conversation context.",
  },
  {
    id: "investment-banking",
    name: "Investment Banking",
    description: "M&A, capital markets, LevFin, valuation, diligence, and pitch workflows",
    category: "Featured",
    icon: "lucide:landmark",
    iconColor: "#10B981",
    official: true,
    developer: "Hivy",
    capabilities: ["Read", "Write"],
    examples: [
      "Build a comparable companies analysis",
      "Draft the executive summary for this CIM",
      "Check this term sheet for missing provisions",
    ],
    skills: [
      { name: "valuation", description: "Run comps, precedent transactions, and DCF analysis" },
      { name: "diligence", description: "Review documents and flag key risks" },
      { name: "pitch", description: "Draft and format pitch materials" },
    ],
    links: {
      website: "#",
      privacy: "#",
      terms: "#",
    },
    longDescription:
      "Investment Banking supports M&A, capital markets, leveraged finance, valuation, diligence, and pitch workflows. Pull data from documents and models to build analyses and draft materials faster.",
  },
  {
    id: "public-equity",
    name: "Public Equity Investing",
    description: "Public equity PM research, long/short, earnings, ETF/index diligence, and memos",
    category: "Featured",
    icon: "lucide:bar-chart-3",
    iconColor: "#22C55E",
    official: true,
    developer: "Hivy",
    capabilities: ["Read", "Write"],
    examples: [
      "Summarize the latest earnings transcript",
      "Compare this stock to its ETF peers",
      "Draft an investment memo outline",
    ],
    skills: [
      { name: "earnings", description: "Extract and compare earnings call takeaways" },
      { name: "diligence", description: "Run long/short and ETF/index comparisons" },
      { name: "memo", description: "Structure and draft investment memos" },
    ],
    links: {
      website: "#",
      privacy: "#",
      terms: "#",
    },
    longDescription:
      "Public Equity Investing supports portfolio manager research, long/short idea generation, earnings analysis, ETF and index diligence, and investment memo writing.",
  },
]

export const CATALOG_PLUGINS: Plugin[] = [
  {
    id: "attio",
    name: "Attio",
    description: "Attio connects Hivy directly to your CRM workspace, letting you manage customer relationships...",
    category: "Business & Operations",
    icon: "lucide:database",
    iconColor: "#111111",
    developer: "Attio",
    capabilities: ["Read", "Write"],
    examples: [
      "Show me my open deals",
      "Create a new contact from this email",
      "Summarize my pipeline this quarter",
    ],
    skills: [
      { name: "query", description: "Search and read CRM records" },
      { name: "create", description: "Add contacts, companies, and deals" },
      { name: "update", description: "Modify record fields and statuses" },
    ],
    links: {
      website: "#",
      privacy: "#",
      terms: "#",
    },
    longDescription:
      "Attio connects Hivy directly to your CRM workspace, letting you manage customer relationships, query records, and update deal data from chat.",
  },
  {
    id: "carta-crm",
    name: "Carta CRM",
    description: "Carta CRM helps investment teams stay on top of deal flow by keeping deals, companies, and...",
    category: "Business & Operations",
    icon: "lucide:briefcase",
    iconColor: "#4B5563",
    developer: "Carta",
    capabilities: ["Read", "Write"],
    examples: [
      "List my active deals",
      "Add a note to this company",
      "Prepare a pipeline summary",
    ],
    skills: [
      { name: "pipeline", description: "View and summarize deal flow" },
      { name: "notes", description: "Attach notes to companies and deals" },
      { name: "report", description: "Generate pipeline and activity reports" },
    ],
    links: {
      website: "#",
      privacy: "#",
      terms: "#",
    },
    longDescription:
      "Carta CRM helps investment teams stay on top of deal flow by keeping deals, companies, and relationships organized and accessible from Hivy.",
  },
  {
    id: "demandbase",
    name: "Demandbase",
    description: "Demandbase integration with Hivy gives sales, marketing, and GTM teams seamless access to...",
    category: "Business & Operations",
    icon: "lucide:target",
    iconColor: "#1D4ED8",
    developer: "Demandbase",
    capabilities: ["Read"],
    examples: [
      "Show intent data for this account",
      "Find contacts at my target accounts",
      "Summarize this account's engagement",
    ],
    skills: [
      { name: "intent", description: "Retrieve account intent signals" },
      { name: "contacts", description: "Find decision-makers at target accounts" },
      { name: "engagement", description: "Summarize account engagement history" },
    ],
    links: {
      website: "#",
      privacy: "#",
      terms: "#",
    },
    longDescription:
      "Demandbase integration with Hivy gives sales, marketing, and GTM teams seamless access to account intelligence, intent data, and contact insights.",
  },
  {
    id: "hubspot",
    name: "HubSpot",
    description: "Work with your HubSpot data to analyze patterns, create and update records, and manage your...",
    category: "Business & Operations",
    icon: "simple-icons:hubspot",
    iconColor: "#FF7A59",
    developer: "HubSpot",
    capabilities: ["Read", "Write"],
    examples: [
      "What happened with this contact last week?",
      "Create a task to follow up next Tuesday",
      "Summarize my open tickets",
    ],
    skills: [
      { name: "records", description: "Read and update HubSpot records" },
      { name: "tasks", description: "Create and manage follow-up tasks" },
      { name: "report", description: "Summarize CRM activity and pipeline" },
    ],
    links: {
      website: "#",
      privacy: "#",
      terms: "#",
    },
    longDescription:
      "Work with your HubSpot data to analyze patterns, create and update records, and manage your pipeline without switching contexts.",
  },
  {
    id: "pipedrive",
    name: "Pipedrive",
    description: "Connect to sync Pipedrive deals and contacts for use in Hivy.",
    category: "Business & Operations",
    icon: "lucide:chart-pie",
    iconColor: "#008542",
    developer: "Pipedrive",
    capabilities: ["Read", "Write"],
    examples: [
      "Show deals closing this month",
      "Add a new lead from this conversation",
      "Update deal stage to negotiation",
    ],
    skills: [
      { name: "deals", description: "Query and update Pipedrive deals" },
      { name: "contacts", description: "Manage contacts and organizations" },
      { name: "activities", description: "Schedule and complete activities" },
    ],
    links: {
      website: "#",
      privacy: "#",
      terms: "#",
    },
    longDescription:
      "Connect to sync Pipedrive deals and contacts for use in Hivy, so you can manage your sales workflow from chat.",
  },
  {
    id: "atlassian-jira",
    name: "Atlassian Jira",
    description: "Manage Jira issues, sprints, and projects without leaving Hivy.",
    category: "Productivity",
    icon: "simple-icons:jira",
    iconColor: "#0052CC",
    developer: "Atlassian",
    capabilities: ["Read", "Write"],
    examples: [
      "Create a bug ticket for this issue",
      "What tickets are in the current sprint?",
      "Summarize recent updates on PROJ-123",
    ],
    skills: [
      { name: "issues", description: "Create, read, and update Jira issues" },
      { name: "sprints", description: "Inspect sprint progress and backlog" },
      { name: "projects", description: "Summarize project status and recent changes" },
    ],
    links: {
      website: "#",
      privacy: "#",
      terms: "#",
    },
    longDescription:
      "Manage Jira issues, sprints, and projects without leaving Hivy. Create tickets, check sprint status, and summarize project updates from chat.",
  },
  {
    id: "box",
    name: "Box",
    description: "Search and summarize files stored in your Box account.",
    category: "Productivity",
    icon: "simple-icons:box",
    iconColor: "#0061D5",
    developer: "Box",
    capabilities: ["Read"],
    examples: [
      "Find the Q3 budget spreadsheet",
      "Summarize the latest contract draft",
      "List files shared with me this week",
    ],
    skills: [
      { name: "search", description: "Search files and folders in Box" },
      { name: "summarize", description: "Summarize document contents" },
      { name: "list", description: "List recent and shared files" },
    ],
    links: {
      website: "#",
      privacy: "#",
      terms: "#",
    },
    longDescription:
      "Search and summarize files stored in your Box account. Ask questions across documents and locate the right file without switching apps.",
  },
  {
    id: "brand24",
    name: "Brand24",
    description: "The Brand24 integration lets you monitor brand mentions and sentiment in real time.",
    category: "Productivity",
    icon: "lucide:radio",
    iconColor: "#10B981",
    developer: "Brand24",
    capabilities: ["Read"],
    examples: [
      "What are people saying about our brand today?",
      "Show mentions with negative sentiment",
      "Alert me about a spike in mentions",
    ],
    skills: [
      { name: "mentions", description: "Track brand mentions across the web" },
      { name: "sentiment", description: "Analyze sentiment trends" },
      { name: "alerts", description: "Surface spikes and notable mentions" },
    ],
    links: {
      website: "#",
      privacy: "#",
      terms: "#",
    },
    longDescription:
      "The Brand24 integration lets you monitor brand mentions and sentiment in real time, so you can respond quickly to feedback and trends.",
  },
  {
    id: "clickup",
    name: "ClickUp",
    description: "Turn Hivy chats into ClickUp tasks and keep projects moving.",
    category: "Productivity",
    icon: "simple-icons:clickup",
    iconColor: "#7B68EE",
    developer: "ClickUp",
    capabilities: ["Read", "Write"],
    examples: [
      "Create a task from this message",
      "List overdue tasks in Marketing",
      "Update the status of this task",
    ],
    skills: [
      { name: "tasks", description: "Create and update ClickUp tasks" },
      { name: "lists", description: "Query tasks by list, status, or assignee" },
      { name: "status", description: "Move tasks through workflow statuses" },
    ],
    links: {
      website: "#",
      privacy: "#",
      terms: "#",
    },
    longDescription:
      "Turn Hivy chats into ClickUp tasks and keep projects moving. Create, update, and query tasks without leaving the conversation.",
  },
  {
    id: "latex",
    name: "LaTeX",
    description: "Compile LaTeX documents and render equations from your chats.",
    category: "Research",
    icon: "lucide:sigma",
    iconColor: "#374151",
    developer: "Hivy",
    capabilities: ["Interactive", "Write"],
    examples: [
      "Render this equation in LaTeX",
      "Compile this document to PDF",
      "Fix the syntax in this proof",
    ],
    skills: [
      { name: "render", description: "Render LaTeX equations and documents" },
      { name: "compile", description: "Compile LaTeX sources to PDF" },
      { name: "syntax", description: "Check and fix LaTeX syntax" },
    ],
    links: {
      website: "#",
      privacy: "#",
      terms: "#",
    },
    longDescription:
      "Compile LaTeX documents and render equations from your chats. Perfect for research, academic writing, and technical documentation.",
  },
  {
    id: "codex-security",
    name: "Codex Security",
    description: "Security scanning for your codebase",
    category: "Security",
    icon: "lucide:shield-check",
    iconColor: "#2563EB",
    developer: "OpenAI",
    capabilities: ["Read", "Write"],
    examples: [
      "Scan this repo for vulnerabilities",
      "Explain this security warning",
      "Generate a fix for this issue",
    ],
    skills: [
      { name: "scan", description: "Run security scans on code" },
      { name: "explain", description: "Explain findings and severity" },
      { name: "fix", description: "Suggest and apply security fixes" },
    ],
    links: {
      website: "#",
      privacy: "#",
      terms: "#",
    },
    longDescription:
      "Security scanning for your codebase. Detect vulnerabilities, understand findings, and generate fixes directly from chat.",
  },
  {
    id: "finn",
    name: "FINN",
    description: "A FINN car subscription is a flexible way to stay mobile anytime - without long-term commitments...",
    category: "Travel",
    icon: "lucide:car",
    iconColor: "#F59E0B",
    developer: "FINN",
    capabilities: ["Read"],
    examples: [
      "Find available cars in Berlin",
      "Compare subscription prices",
      "Show me the terms for this model",
    ],
    skills: [
      { name: "search", description: "Search available vehicles and locations" },
      { name: "compare", description: "Compare subscription plans and pricing" },
      { name: "terms", description: "Retrieve terms for selected models" },
    ],
    links: {
      website: "#",
      privacy: "#",
      terms: "#",
    },
    longDescription:
      "A FINN car subscription is a flexible way to stay mobile anytime — without long-term commitments. Search vehicles, compare plans, and review terms from Hivy.",
  },
  {
    id: "weather-promise",
    name: "WeatherPromise",
    description: "Protect your trip with WeatherPromise and get back the full cost if it rains more than promised...",
    category: "Travel",
    icon: "lucide:cloud-rain",
    iconColor: "#F97316",
    developer: "WeatherPromise",
    capabilities: ["Read"],
    examples: [
      "Is my trip eligible for rain protection?",
      "Show coverage for these dates",
      "File a claim for last week's trip",
    ],
    skills: [
      { name: "eligibility", description: "Check trip eligibility for protection" },
      { name: "coverage", description: "Show coverage details by date and location" },
      { name: "claims", description: "Start and track protection claims" },
    ],
    links: {
      website: "#",
      privacy: "#",
      terms: "#",
    },
    longDescription:
      "Protect your trip with WeatherPromise and get back the full cost if it rains more than promised. Check eligibility, view coverage, and manage claims from chat.",
  },
]

export const ALL_PLUGINS = [...FEATURED_PLUGINS, ...CATALOG_PLUGINS]

export const SECTION_ORDER: PluginCategory[] = [
  "Featured",
  "Business & Operations",
  "Productivity",
  "Research",
  "Security",
  "Travel",
]

export function findPluginBySlug(slug: string): Plugin | undefined {
  return ALL_PLUGINS.find((plugin) => plugin.id === slug)
}
