import { describe, expect, it } from "vitest"
import {
  automationEventLabel,
  automationFromCatalog,
  automationFromInstalledTrigger,
  automationInstructions,
  automationMatchesQuery,
  automationSourceLabel,
  automationTriggerDefaultValue,
  automationTriggerKey,
  type CatalogAutomation,
} from "./_data"

describe("automation catalog data", () => {
  it("maps the Slack reaction trigger template shape", () => {
    const catalog: CatalogAutomation = {
      slug: "slack-reaction",
      name: "React with emoji",
      description: "Run an agent from a Slack reaction.",
      category: "Communication",
      integration: { provider: "slack", required: true },
      plugins: { required: ["slack"], recommended: [] },
      trigger: {
        key: "reaction_added",
        defaults: {
          value: "eyes",
          instructions: "Reply in the reacted Slack thread.",
        },
      },
    }

    const automation = automationFromCatalog(catalog, "Triggers")

    expect(automationSourceLabel(automation)).toBe("Slack")
    expect(automationTriggerKey(automation)).toBe("reaction_added")
    expect(automationEventLabel(automation)).toBe("Reaction Added")
    expect(automationTriggerDefaultValue(automation)).toBe("eyes")
    expect(automationInstructions(automation)).toBe(
      "Reply in the reacted Slack thread."
    )
    expect(automationMatchesQuery(automation, "reaction")).toBe(true)
  })

  it("maps an installed Slack reaction trigger row", () => {
    const automation = automationFromInstalledTrigger({
      id: "trigger-1",
      provider: "slack",
      trigger_key: "reaction_added",
      trigger_value: "eyes",
      external_resource_name: "general",
      agent_id: "agent-1",
      agent_name: "Hivy",
    })

    expect(automation.name).toBe("React with emoji")
    expect(automation.description).toBe(
      "Hivy runs when :eyes: is added in general."
    )
    expect(automation.href).toBe("/w/automations/triggers/trigger-1")
    expect(automationMatchesQuery(automation, "general")).toBe(true)
  })

  it("maps the GitHub code-reviews trigger template source label", () => {
    const catalog: CatalogAutomation = {
      slug: "github-code-reviews-pr-mention",
      name: "Code review requested on a pull request",
      description:
        "Run a code-review agent when someone @mentions @usehivy-reviews on a GitHub pull request.",
      category: "Development",
      integration: { provider: "github-app-code-reviews", required: true },
      plugins: { required: ["github-code-reviews"], recommended: [] },
      trigger: {
        key: "pr_mention",
        defaults: { value: "", instructions: "Review the pull request." },
      },
    }

    const automation = automationFromCatalog(catalog, "Triggers")

    expect(automationSourceLabel(automation)).toBe("GitHub Code Reviews")
    expect(automation.icon).toBe("github")
    expect(automation.iconColor).toBe("#181717")
    expect(automationTriggerKey(automation)).toBe("pr_mention")
  })

  it("maps an installed GitHub mention trigger row", () => {
    const automation = automationFromInstalledTrigger({
      id: "trigger-2",
      provider: "github-app",
      trigger_key: "pr_mention",
      external_resource_key: "usehivy/hivy",
      agent_id: "agent-1",
      agent_name: "Zuko",
    })

    expect(automation.name).toBe("Mentioned on GitHub")
    expect(automation.description).toBe(
      "Zuko runs when Hivy is @mentioned in usehivy/hivy."
    )
    expect(automation.category).toBe("Development")
  })

  it("maps an installed GitHub code-review trigger row", () => {
    const automation = automationFromInstalledTrigger({
      id: "trigger-3",
      provider: "github-app-code-reviews",
      trigger_key: "pr_mention",
      external_resource_key: "usehivy/hivy",
      agent_id: "agent-1",
      agent_name: "Zuko",
    })

    expect(automation.name).toBe("Code review requested")
    expect(automation.description).toBe(
      "Zuko runs when @usehivy-reviews is @mentioned in usehivy/hivy."
    )
    expect(automation.category).toBe("Development")
    expect(automation.iconColor).toBe("#181717")
  })

  it("maps the GitHub auto-review (pr_opened) trigger template", () => {
    const catalog: CatalogAutomation = {
      slug: "github-code-reviews-pr-opened",
      name: "Review every new pull request",
      description:
        "Run a code-review agent automatically when a pull request is opened.",
      category: "Development",
      integration: { provider: "github-app-code-reviews", required: true },
      plugins: { required: ["github-code-reviews"], recommended: [] },
      trigger: {
        key: "pr_opened",
        defaults: { value: "", instructions: "Review the pull request." },
      },
    }

    const automation = automationFromCatalog(catalog, "Triggers")

    expect(automationSourceLabel(automation)).toBe("GitHub Code Reviews")
    expect(automationTriggerKey(automation)).toBe("pr_opened")
  })

  it("maps an installed GitHub auto-review (pr_opened) trigger row", () => {
    const automation = automationFromInstalledTrigger({
      id: "trigger-4",
      provider: "github-app-code-reviews",
      trigger_key: "pr_opened",
      external_resource_key: "usehivy/hivy",
      agent_id: "agent-1",
      agent_name: "Zuko",
    })

    expect(automation.name).toBe("Reviews new pull requests")
    expect(automation.description).toBe(
      "Zuko reviews new pull requests in usehivy/hivy."
    )
    expect(automation.category).toBe("Development")
    expect(automation.iconColor).toBe("#181717")
  })

  it("marks disabled installed triggers", () => {
    const automation = automationFromInstalledTrigger({
      id: "trigger-1",
      provider: "slack",
      trigger_key: "reaction_added",
      trigger_value: "eyes",
      external_resource_name: "general",
      agent_id: "agent-1",
      agent_name: "Hivy",
      enabled: false,
    })

    expect(automation.description).toBe(
      "Disabled. Hivy runs when :eyes: is added in general."
    )
  })
})
