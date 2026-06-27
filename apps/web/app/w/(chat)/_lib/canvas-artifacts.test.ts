import { afterEach, describe, expect, it, vi } from "vitest"
import {
  canvasArtifactCommentPayload,
  fetchCanvasArtifacts,
  normalizeCanvasArtifactList,
  normalizeCanvasProjectList,
  sendCanvasArtifactComment,
} from "@/app/w/(chat)/_lib/canvas-artifacts"

describe("canvas artifacts", () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it("normalizes project and artifact API responses", () => {
    expect(
      normalizeCanvasProjectList({
        projects: [
          {
            project_id: "project-1",
            slug: "launch",
            name: "Launch",
            artifact_count: 2,
          },
        ],
      })
    ).toEqual([
      {
        id: "project-1",
        slug: "launch",
        name: "Launch",
        description: undefined,
        artifactCount: 2,
        updatedAt: undefined,
      },
    ])

    expect(
      normalizeCanvasArtifactList({
        artifacts: [
          {
            artifact_id: "artifact-1",
            project_id: "project-1",
            artifact_type: "web_page",
            name: "Homepage",
            manifest: { entry_file: "index.html" },
          },
        ],
      })
    ).toEqual([
      {
        id: "artifact-1",
        projectId: "project-1",
        slug: undefined,
        name: "Homepage",
        type: "web_page",
        entryFile: "index.html",
        updatedAt: undefined,
      },
    ])
  })

  it("loads artifacts through the authenticated API proxy", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          artifacts: [{ id: "artifact-1", project_id: "project-1" }],
        }),
        { status: 200, headers: { "Content-Type": "application/json" } }
      )
    )
    vi.stubGlobal("fetch", fetchMock)

    await expect(
      fetchCanvasArtifacts({ projectId: "project-1", sessionId: "session-1" })
    ).resolves.toEqual([
      expect.objectContaining({
        id: "artifact-1",
        projectId: "project-1",
      }),
    ])

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/proxy/v1/canvas/artifacts?project_id=project-1&session_id=session-1",
      expect.objectContaining({ method: "GET" })
    )
  })

  it("sends artifact comments as structured session message payloads", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ ok: true }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      })
    )
    vi.stubGlobal("fetch", fetchMock)
    const comment = canvasArtifactCommentPayload({
      artifact: {
        id: "artifact-1",
        projectId: "project-1",
        name: "Homepage",
        type: "web_page",
      },
      project: { id: "project-1", name: "Launch" },
      viewport: "desktop",
      body: "Tighten the top nav spacing.",
      selector: '[data-canvas-id="top-nav"]',
      now: new Date("2026-06-26T12:00:00.000Z"),
    })

    await sendCanvasArtifactComment("session-1", comment)

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/proxy/v1/sessions/session-1/messages",
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({
          text: "Tighten the top nav spacing.",
          artifact_comments: [comment],
        }),
      })
    )
  })
})
