import { describe, expect, it } from "vitest"
import { pluginLogoProvider, type ApiPlugin } from "@/app/w/(chat)/plugins/_lib"
import {
  agentCanInstall,
  agentInstalledTeamIDs,
  agentIsInstalled,
  agentRequiredPluginSlugs,
  agentAvatarURL,
  agentCategories,
  agentMatchesCategory,
  agentMatchesQuery,
  agentMissingPlugins,
  availableTeamsFor,
  formatMissingPluginsMessage,
  withPluginMCPToolDenied,
  groupAgents,
  installErrorMessage,
  installedTeamsFor,
  missingPluginSlugsFromError,
  normalizeAgentSandboxImage,
  normalizeAgentSandboxSize,
  pluginEnabledForAgent,
  pluginForRequirement,
  pluginsBySlug,
  teamHasAgent,
  teamNameByID,
  type CatalogAgent,
  type InstalledAgent,
  type Team,
} from "./_lib"

describe("withPluginMCPToolDenied", () => {
  it("adds, removes, sorts, and clears per-plugin tool denies", () => {
    const pluginID = "11111111-1111-1111-1111-111111111111"
    const added = withPluginMCPToolDenied(
      { [pluginID]: ["users_list"] },
      pluginID,
      "chat_delete",
      true
    )
    expect(added[pluginID]).toEqual(["chat_delete", "users_list"])
    expect(
      withPluginMCPToolDenied(added, pluginID, "chat_delete", false)
    ).toEqual({ [pluginID]: ["users_list"] })
    expect(
      withPluginMCPToolDenied(
        { [pluginID]: ["users_list"] },
        pluginID,
        "users_list",
        false
      )
    ).toEqual({})
  })
})

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

  it("normalizes agent sandbox size values", () => {
    expect(normalizeAgentSandboxSize("nano")).toBe("nano")
    expect(normalizeAgentSandboxSize("xlarge")).toBe("xlarge")
    expect(normalizeAgentSandboxSize("jumbo")).toBe("small")
    expect(normalizeAgentSandboxSize(undefined)).toBe("small")
  })

  it("normalizes agent sandbox image values", () => {
    expect(normalizeAgentSandboxImage("developer")).toBe("developer")
    expect(normalizeAgentSandboxImage("gpu")).toBe("default")
    expect(normalizeAgentSandboxImage(undefined)).toBe("default")
  })

  it("resolves required plugin logo data from the plugin catalog", () => {
    const plugin: ApiPlugin = {
      id: "plugin-1",
      slug: "github",
      name: "GitHub",
      icon: "github",
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

  it("reads installed team ids and treats team-scoped clones as installed", () => {
    const item = agent({ installed_team_ids: ["team-a", " ", "team-b"] })

    expect(agentInstalledTeamIDs(item)).toEqual(["team-a", "team-b"])
    expect(teamHasAgent(item, "team-a")).toBe(true)
    expect(teamHasAgent(item, "team-c")).toBe(false)
    expect(agentIsInstalled(item)).toBe(true)
    expect(agentIsInstalled(agent({}))).toBe(false)
  })

  it("splits teams into installed and installable for a catalog agent", () => {
    const teams: Team[] = [
      { id: "team-a", name: "Growth" },
      { id: "team-b", name: "Support" },
      { id: "team-c", name: "Design" },
    ]
    const item = agent({ installed_team_ids: ["team-b"] })

    expect(installedTeamsFor(teams, item).map((t) => t.id)).toEqual(["team-b"])
    expect(availableTeamsFor(teams, item).map((t) => t.id)).toEqual([
      "team-a",
      "team-c",
    ])
    expect(teamNameByID(teams, "team-c")).toBe("Design")
    expect(teamNameByID(teams, "missing")).toBe("missing")
  })

  it("parses missing plugin slugs from a 422 install error body", () => {
    expect(
      missingPluginSlugsFromError({
        error: "your team is missing required plugins",
        missing_plugins: ["github", " ", "slack"],
      })
    ).toEqual(["github", "slack"])
    expect(missingPluginSlugsFromError({ error: "nope" })).toEqual([])
    expect(missingPluginSlugsFromError(new Error("boom"))).toEqual([])
  })

  it("formats a non-generic missing-plugins message with friendly names", () => {
    const item = agent({
      required_plugins: [{ slug: "github", name: "GitHub", installed: false }],
      recommended_plugins: [{ slug: "slack", name: "Slack", installed: false }],
    })

    expect(formatMissingPluginsMessage("Growth", ["github"], item)).toBe(
      "Growth can't use this agent yet — it needs the GitHub plugin. Ask an org admin to enable it for Growth."
    )
    expect(
      formatMissingPluginsMessage("Growth", ["github", "slack"], item)
    ).toBe(
      "Growth can't use this agent yet — it needs the GitHub and Slack plugins. Ask an org admin to enable it for Growth."
    )
    // Falls back to the raw slug when the catalog has no friendly name.
    expect(formatMissingPluginsMessage("Growth", ["linear"], item)).toBe(
      "Growth can't use this agent yet — it needs the linear plugin. Ask an org admin to enable it for Growth."
    )
  })

  it("classifies install errors into missing-plugins, membership, and generic copy", () => {
    const item = agent({
      required_plugins: [{ slug: "github", name: "GitHub", installed: false }],
    })

    expect(
      installErrorMessage(
        { error: "missing", missing_plugins: ["github"] },
        "Growth",
        item
      )
    ).toContain("it needs the GitHub plugin")
    expect(
      installErrorMessage(
        { error: "you are not a member of this team" },
        "Growth",
        item
      )
    ).toBe("You're not a member of that team.")
    expect(installErrorMessage({ error: "boom" }, "Growth", item)).toBe("boom")
    expect(installErrorMessage({}, "Growth", item)).toBe(
      "Could not install agent"
    )
  })

  it("tracks required plugin slugs and agent plugin enablement", () => {
    const item = agent({
      required_plugins: [
        { slug: "design", name: "Design", installed: true },
        { slug: " ", name: "Blank", installed: true },
      ],
    })
    const plugin: ApiPlugin = {
      slug: "design",
      enabled_agent_ids: ["agent-1"],
    }

    expect(Array.from(agentRequiredPluginSlugs(item))).toEqual(["design"])
    expect(pluginEnabledForAgent(plugin, "agent-1")).toBe(true)
    expect(pluginEnabledForAgent(plugin, "agent-2")).toBe(false)
  })
})
