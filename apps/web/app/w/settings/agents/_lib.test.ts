import { describe, expect, it } from "vitest"
import {
  agentCanInstall,
  agentAvatarURL,
  agentCategories,
  agentMatchesCategory,
  agentMatchesQuery,
  agentMissingPlugins,
  groupAgents,
  type CatalogAgent,
  type InstalledAgent,
} from "./_lib"

function agent(overrides: Partial<CatalogAgent>): CatalogAgent {
  return {
    id: "agent-1",
    slug: "agent-one",
    name: "Agent One",
    description: "Handles workspace requests",
    category: "Workspace",
    required_plugins: [],
    ...overrides,
  }
}

describe("agent catalog helpers", () => {
  it("detects missing required plugins and blocks install", () => {
    const item = agent({
      required_plugins: [
        { slug: "github", name: "GitHub", installed: true },
        { slug: "slack", name: "Slack", installed: false },
      ],
    })

    expect(agentMissingPlugins(item).map((plugin) => plugin.slug)).toEqual([
      "slack",
    ])
    expect(agentCanInstall(item)).toBe(false)
  })

  it("allows install when required plugins are installed and agent is not installed", () => {
    const item = agent({
      required_plugins: [{ slug: "github", name: "GitHub", installed: true }],
    })

    expect(agentCanInstall(item)).toBe(true)
  })

  it("does not allow install when the catalog agent is already installed", () => {
    const item = agent({ installed_agent_id: "installed-agent" })

    expect(agentCanInstall(item)).toBe(false)
  })

  it("builds filter categories and groups agents by catalog category", () => {
    const agents = [
      agent({ slug: "hivy", category: "Workspace", official: true }),
      agent({ slug: "hakaree", category: "Developer Tools" }),
    ]
    const categories = agentCategories(agents)

    expect(categories).toEqual([
      "All",
      "Featured",
      "Developer Tools",
      "Workspace",
    ])
    expect(Object.keys(groupAgents(agents, categories, "All"))).toEqual([
      "Developer Tools",
      "Workspace",
    ])
  })

  it("matches featured agents and text queries", () => {
    const item = agent({
      name: "Hakaree",
      description: "Coding and infrastructure specialist",
      category: "Developer Tools",
      official: true,
      slug: "hakaree",
    })

    expect(agentMatchesCategory(item, "Featured")).toBe(true)
    expect(agentMatchesQuery(item, "infra")).toBe(true)
    expect(agentMatchesQuery(item, "sales")).toBe(false)
  })

  it("falls back to the catalog avatar for installed agents", () => {
    const item: InstalledAgent = {
      id: "installed-agent",
      name: "Hivy",
      avatar_url: "",
      catalog: {
        avatar_url: "/assets/hivy.png",
      },
    }

    expect(agentAvatarURL(item)).toBe("/assets/hivy.png")
  })
})
