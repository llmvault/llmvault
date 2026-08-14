type PricingComparison = {
  id: "claude" | "chatgpt" | "gumloop" | "notion"
  tabLabel: string
  name: string
  plan: string
  summary: string
  rows: readonly {
    label: string
    hivy: string
    hivyDetail: string
    competitor: string
    competitorDetail: string
  }[]
  sources: readonly {
    label: string
    href: string
  }[]
}

const hivyUsage = {
  hivy: "Pay for the work",
  hivyDetail:
    "Models are charged at cost. Sandbox compute is 1 credit per active vCPU-minute.",
} as const

export const pricingComparisons: readonly PricingComparison[] = [
  {
    id: "claude",
    tabLabel: "Claude",
    name: "Claude Team",
    plan: "Standard seats",
    summary:
      "A seat subscription with included limits and optional usage credits after those limits.",
    rows: [
      {
        label: "Monthly floor",
        hivy: "$0 recurring",
        hivyDetail: "Start free. The minimum credit deposit is $5.",
        competitor: "$50 / month",
        competitorDetail: "Two-seat minimum at $25 per standard seat.",
      },
      {
        label: "Five-person team",
        hivy: "$0 / month base",
        hivyDetail: "Adding people does not add a seat charge.",
        competitor: "$125 / month",
        competitorDetail: "Five standard seats on monthly billing.",
      },
      {
        label: "Agent usage",
        ...hivyUsage,
        competitor: "Limits included",
        competitorDetail:
          "Optional usage credits can extend work after the included seat limits.",
      },
      {
        label: "When agents are idle",
        hivy: "$0 sandbox compute",
        hivyDetail: "Sleeping sandboxes do not spend compute credits.",
        competitor: "Seat fee continues",
        competitorDetail: "The subscription is billed for every active seat.",
      },
      {
        label: "Seat model",
        hivy: "Unlimited users",
        hivyDetail: "No per-user plan or seat minimum.",
        competitor: "Two-seat minimum",
        competitorDetail: "$20 per seat on annual billing; $25 monthly.",
      },
    ],
    sources: [
      {
        label: "Claude Team pricing",
        href: "https://support.claude.com/en/articles/9266767-what-is-the-team-plan",
      },
    ],
  },
  {
    id: "chatgpt",
    tabLabel: "ChatGPT",
    name: "ChatGPT Business",
    plan: "Standard ChatGPT seats",
    summary:
      "A shared ChatGPT workspace billed per standard seat, with workspace credits available beyond included limits.",
    rows: [
      {
        label: "Monthly floor",
        hivy: "$0 recurring",
        hivyDetail: "Start free. The minimum credit deposit is $5.",
        competitor: "$50 / month",
        competitorDetail: "Two-seat minimum at $25 per standard seat.",
      },
      {
        label: "Five-person team",
        hivy: "$0 / month base",
        hivyDetail: "Adding people does not add a seat charge.",
        competitor: "$125 / month",
        competitorDetail: "Five standard seats on monthly billing.",
      },
      {
        label: "Agent usage",
        ...hivyUsage,
        competitor: "Limits included",
        competitorDetail:
          "Workspace credits can cover usage beyond the included rate limits.",
      },
      {
        label: "When agents are idle",
        hivy: "$0 sandbox compute",
        hivyDetail: "Sleeping sandboxes do not spend compute credits.",
        competitor: "Seat fee continues",
        competitorDetail: "The subscription is billed for every active seat.",
      },
      {
        label: "Seat model",
        hivy: "Unlimited users",
        hivyDetail: "No per-user plan or seat minimum.",
        competitor: "Two-seat minimum",
        competitorDetail: "$20 per seat on annual billing; $25 monthly.",
      },
    ],
    sources: [
      {
        label: "ChatGPT Business pricing",
        href: "https://help.openai.com/en/articles/8792828-what-is-chatgpt-team/",
      },
    ],
  },
  {
    id: "gumloop",
    tabLabel: "Gumloop",
    name: "Gumloop",
    plan: "Pro",
    summary:
      "A fixed platform subscription with a shared monthly credit allocation and paid overages.",
    rows: [
      {
        label: "Monthly floor",
        hivy: "$0 recurring",
        hivyDetail: "Start free. The minimum credit deposit is $5.",
        competitor: "$37 / month",
        competitorDetail: "Pro starts with 20,000 monthly credits.",
      },
      {
        label: "Five-person team",
        hivy: "$0 / month base",
        hivyDetail: "Adding people does not add a seat charge.",
        competitor: "$37 / month",
        competitorDetail:
          "Pro lists unlimited seats and one shared credit pool.",
      },
      {
        label: "Agent usage",
        ...hivyUsage,
        competitor: "20,000 credits included",
        competitorDetail:
          "Gumloop lists overage at $0.007 per credit, capped at twice the monthly allocation.",
      },
      {
        label: "When agents are idle",
        hivy: "$0 sandbox compute",
        hivyDetail: "Sleeping sandboxes do not spend compute credits.",
        competitor: "$37 base continues",
        competitorDetail: "The Pro subscription renews monthly.",
      },
      {
        label: "Seat model",
        hivy: "Unlimited users",
        hivyDetail: "No per-user plan or seat minimum.",
        competitor: "Unlimited seats",
        competitorDetail: "Collaboration is included in Pro.",
      },
    ],
    sources: [
      {
        label: "Gumloop pricing",
        href: "https://www.gumloop.com/pricing",
      },
      {
        label: "Gumloop credit guide",
        href: "https://docs.gumloop.com/core-concepts/credits",
      },
    ],
  },
  {
    id: "notion",
    tabLabel: "Notion agents",
    name: "Notion Custom Agents",
    plan: "Business + Notion credits",
    summary:
      "An agent add-on for Business or Enterprise workspaces, charged on top of the per-member workspace plan.",
    rows: [
      {
        label: "Monthly floor",
        hivy: "$0 recurring",
        hivyDetail: "Start free. The minimum credit deposit is $5.",
        competitor: "$20 / member + credits",
        competitorDetail:
          "Custom Agents require Business or Enterprise and purchased Notion credits.",
      },
      {
        label: "Five-person team",
        hivy: "$0 / month base",
        hivyDetail: "Adding people does not add a seat charge.",
        competitor: "$100 / month + credits",
        competitorDetail: "Five Business members on monthly billing.",
      },
      {
        label: "Agent usage",
        ...hivyUsage,
        competitor: "$10 / 1,000 credits",
        competitorDetail:
          "Credits are shared across the workspace and more complex runs use more.",
      },
      {
        label: "When agents are idle",
        hivy: "$0 sandbox compute",
        hivyDetail: "Sleeping sandboxes do not spend compute credits.",
        competitor: "Workspace fee continues",
        competitorDetail: "Business remains billed per member.",
      },
      {
        label: "Seat model",
        hivy: "Unlimited users",
        hivyDetail: "No per-user plan or seat minimum.",
        competitor: "Per workspace member",
        competitorDetail: "Paid Notion plans charge for each member.",
      },
    ],
    sources: [
      {
        label: "Notion plan pricing",
        href: "https://www.notion.com/pricing",
      },
      {
        label: "Custom Agent credits",
        href: "https://www.notion.com/help/buy-and-track-notion-credits-for-custom-agents",
      },
    ],
  },
] as const
