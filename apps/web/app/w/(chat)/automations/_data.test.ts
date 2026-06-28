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
