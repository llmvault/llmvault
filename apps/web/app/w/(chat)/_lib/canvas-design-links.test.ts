import { describe, expect, it } from "vitest"
import {
  canvasDesignTargetKey,
  canvasDesignTargets,
  isFreshCanvasSessionURL,
} from "@/app/w/(chat)/_lib/canvas-design-links"

const canvasUrl =
  "https://canvas.usehivy.com/#/workspace?file-id=9e54f9e0-35c6-48b4-ac84-e95bac316011&team-id=630567c0-663f-41ca-81a0-6c49ce5aea03"

describe("canvas design links", () => {
  it("extracts unique Canvas workspace targets from assistant text", () => {
    const targets = canvasDesignTargets(
      `Open [the design](${canvasUrl}) and again ${canvasUrl}.`
    )

    expect(targets).toEqual([
      {
        key: "9e54f9e0-35c6-48b4-ac84-e95bac316011",
        fileId: "9e54f9e0-35c6-48b4-ac84-e95bac316011",
        sourceUrl: canvasUrl,
      },
    ])
  })

  it("keeps page-specific targets separate", () => {
    expect(
      canvasDesignTargetKey(
        "9e54f9e0-35c6-48b4-ac84-e95bac316011",
        "11111111-1111-4111-8111-111111111111"
      )
    ).toBe(
      "9e54f9e0-35c6-48b4-ac84-e95bac316011:11111111-1111-4111-8111-111111111111"
    )
  })

  it("rejects non-Canvas and incomplete targets", () => {
    expect(
      canvasDesignTargets(
        "https://example.com/#/workspace?file-id=9e54f9e0-35c6-48b4-ac84-e95bac316011"
      )
    ).toEqual([])
    expect(
      canvasDesignTargets("https://canvas.usehivy.com/#/workspace?file-id=9e")
    ).toEqual([])
  })

  it("treats cached session URLs as fresh until near expiry", () => {
    expect(
      isFreshCanvasSessionURL(
        {
          url: "https://canvas.usehivy.com/api/hivy/session?token=test",
          cachedAt: 1_000,
          expiresAt: 121_000,
        },
        60_000
      )
    ).toBe(true)
    expect(
      isFreshCanvasSessionURL(
        {
          url: "https://canvas.usehivy.com/api/hivy/session?token=test",
          cachedAt: 1_000,
          expiresAt: 61_000,
        },
        60_000
      )
    ).toBe(false)
  })
})
