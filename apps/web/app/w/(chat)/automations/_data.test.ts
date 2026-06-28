import { describe, expect, it } from "vitest"
import {
  automationEventLabel,
  automationFromCatalog,
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
})
