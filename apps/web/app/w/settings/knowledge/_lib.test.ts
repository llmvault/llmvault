import { describe, expect, it } from "vitest"
import { deriveStatus, ingestionActionForStatus, type RagSource } from "./_lib"

function source(overrides: Partial<RagSource> = {}): RagSource {
  return {
    id: "source-1",
    org_id: "org-1",
    kind: "WEBSITE",
    name: "Product docs",
    status: "ACTIVE",
    enabled: true,
    config: [],
    total_docs_indexed: 0,
    in_repeated_error_state: false,
    created_at: "2026-08-14T00:00:00Z",
    updated_at: "2026-08-14T00:00:00Z",
    ...overrides,
  }
}

describe("knowledge-source ingestion actions", () => {
  it("offers retry when the latest sync failed, including disabled error sources", () => {
    const status = deriveStatus(
      source({
        enabled: false,
        latest_attempt: { status: "failed" } as NonNullable<
          RagSource["latest_attempt"]
        >,
      })
    )

    expect(status).toBe("error")
    expect(ingestionActionForStatus(status)).toBe("retry")
  })

  it("offers resume for a paused source even when its previous attempt is still active", () => {
    const status = deriveStatus(
      source({
        status: "PAUSED",
        latest_attempt: {
          status: "in_progress",
        } as NonNullable<RagSource["latest_attempt"]>,
      })
    )

    expect(status).toBe("paused")
    expect(ingestionActionForStatus(status)).toBe("resume")
  })

  it("offers pause for an active source", () => {
    expect(ingestionActionForStatus(deriveStatus(source()))).toBe("pause")
  })
})
