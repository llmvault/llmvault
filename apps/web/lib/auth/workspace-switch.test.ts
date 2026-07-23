import type { QueryClient } from "@tanstack/react-query"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

vi.mock("@/app/w/(chat)/_lib/session-sandbox-access", () => ({
  clearSessionSandboxAccess: vi.fn(),
}))

vi.mock("@/app/w/(chat)/_stores/session-stream-manager", () => ({
  resumeSessionConnectionsForOrg: vi.fn(),
  suspendSessionConnectionsForOrg: vi.fn(),
}))

import { clearSessionSandboxAccess } from "@/app/w/(chat)/_lib/session-sandbox-access"
import {
  resumeSessionConnectionsForOrg,
  suspendSessionConnectionsForOrg,
} from "@/app/w/(chat)/_stores/session-stream-manager"
import { isAuthQueryKey, switchActiveOrg } from "@/lib/auth/workspace-switch"

describe("workspace switching", () => {
  const originalDocument = globalThis.document

  beforeEach(() => {
    vi.clearAllMocks()
    Object.defineProperty(globalThis, "document", {
      configurable: true,
      value: { cookie: "" },
    })
  })

  afterEach(() => {
    if (originalDocument) {
      Object.defineProperty(globalThis, "document", {
        configurable: true,
        value: originalDocument,
      })
      return
    }
    Reflect.deleteProperty(globalThis, "document")
  })

  it("isolates outgoing data and resumes suspended work in the selected workspace", async () => {
    const calls: string[] = []
    const queryClient = {
      cancelQueries: vi.fn(async () => {
        calls.push("cancel")
      }),
      removeQueries: vi.fn(() => {
        calls.push("remove")
      }),
      invalidateQueries: vi.fn(async () => {
        calls.push("invalidate")
      }),
    } as unknown as QueryClient

    await switchActiveOrg(queryClient, "org-b", {
      previousOrgId: "org-a",
      activate: () => calls.push("activate"),
    })

    expect(calls).toEqual(["cancel", "activate", "remove", "invalidate"])
    expect(suspendSessionConnectionsForOrg).toHaveBeenCalledWith("org-a")
    expect(clearSessionSandboxAccess).toHaveBeenCalledOnce()
    expect(document.cookie).toContain("hivy_active_org=org-b")
    expect(resumeSessionConnectionsForOrg).toHaveBeenCalledWith(
      "org-b",
      queryClient
    )
  })

  it("preserves only the workspace-independent auth query", () => {
    expect(isAuthQueryKey(["get", "/auth/me"])).toBe(true)
    expect(isAuthQueryKey(["get", "/v1/sessions"])).toBe(false)
  })
})
