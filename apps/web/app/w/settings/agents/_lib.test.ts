import { describe, expect, it } from "vitest"
import {
  agentCanInstall,
  agentAvatarURL,
  agentAvailableModels,
  agentCategories,
  agentMatchesCategory,
  agentMatchesQuery,
  agentMissingPlugins,
  groupAgents,
  normalizeAgentSandboxSize,
  pluginForRequirement,
  pluginsBySlug,
  type CatalogAgent,
  type InstalledAgent,
} from "./_lib"
import { pluginLogoProvider, type ApiPlugin } from "@/app/w/(chat)/plugins/_lib"

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

  it("uses catalog available models with the catalog default first when needed", () => {
    const item = agent({
      model: "deepseek-v4-pro",
      available_models: [" qwen3.7-plus ", "qwen3.7-plus"],
    })

    expect(agentAvailableModels(item)).toEqual([
      "deepseek-v4-pro",
      "qwen3.7-plus",
    ])
  })

  it("normalizes agent sandbox size values", () => {
    expect(normalizeAgentSandboxSize("xlarge")).toBe("xlarge")
    expect(normalizeAgentSandboxSize("jumbo")).toBe("small")
    expect(normalizeAgentSandboxSize(undefined)).toBe("small")
  })

  it("resolves required plugin logo data from the plugin catalog", () => {
    const plugin: ApiPlugin = {
      id: "plugin-1",
      slug: "github",
      name: "GitHub",
      icon: "simple-icons:github",
      icon_color: "#181717",
      required_connections: [
        { provider: "github-app", kind: "integration", required: true },
      ],
    }
    const lookup = pluginsBySlug([plugin])
    const matched = pluginForRequirement({ slug: "github" }, lookup)

    expect(matched).toBe(plugin)
    expect(pluginLogoProvider(matched!)).toBe("github-app")
  })
})
