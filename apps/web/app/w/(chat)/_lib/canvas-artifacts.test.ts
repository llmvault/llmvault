import React from "react"
import { renderToString } from "react-dom/server"
import { beforeEach, describe, expect, it, vi } from "vitest"

const mocks = vi.hoisted(() => ({
  useQuery: vi.fn(),
  useMutation: vi.fn(),
}))

vi.mock("@/lib/api/hooks", () => ({
  $api: {
    useQuery: mocks.useQuery,
    useMutation: mocks.useMutation,
  },
}))

import {
  canvasArtifactCommentPayload,
  normalizeCanvasArtifactList,
  normalizeCanvasProjectList,
  useCanvasArtifactPreviewURL,
  useCanvasProjects,
  useSendCanvasArtifactComment,
} from "@/app/w/(chat)/_lib/canvas-artifacts"

function probe<T>(render: () => T): T {
  const captured: T[] = []
  function Probe() {
    captured.push(render())
    return null
  }
  renderToString(React.createElement(Probe))
  const result = captured[0]
  if (result === undefined) throw new Error("hook did not run")
  return result
}

beforeEach(() => {
  mocks.useQuery.mockReset()
  mocks.useMutation.mockReset()
})

describe("canvas normalizers", () => {
  it("normalizes project and artifact API responses", () => {
    expect(
      normalizeCanvasProjectList({
        projects: [
          {
            project_id: "project-1",
            slug: "launch",
            name: "Launch",
            artifact_count: 2,
            artifacts: [
              { id: "artifact-1", artifact_type: "web_page", name: "Homepage" },
            ],
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
        artifacts: [
          {
            id: "artifact-1",
            projectId: "project-1",
            slug: undefined,
            name: "Homepage",
            type: "web_page",
            entryFile: undefined,
            updatedAt: undefined,
          },
        ],
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
})

describe("useCanvasProjects", () => {
  it("maps the snake_case project list to the domain shape", () => {
    mocks.useQuery.mockReturnValue({
      data: {
        projects: [
          {
            id: "project-1",
            name: "Launch",
            artifacts: [{ id: "artifact-1", project_id: "project-1" }],
          },
        ],
      },
      isPending: false,
      isError: false,
    })

    const result = probe(() => useCanvasProjects("session-1"))

    // The GET query is keyed on the session id.
    expect(mocks.useQuery.mock.calls[0]?.slice(0, 3)).toEqual([
      "get",
      "/v1/canvas/projects",
      { params: { query: { session_id: "session-1" } } },
    ])
    expect(result.data).toEqual([
      expect.objectContaining({
        id: "project-1",
        artifacts: [
          expect.objectContaining({ id: "artifact-1", projectId: "project-1" }),
        ],
      }),
    ])
  })

  it("omits the session filter when there is no active session", () => {
    mocks.useQuery.mockReturnValue({ data: undefined })
    probe(() => useCanvasProjects(null))
    expect(mocks.useQuery.mock.calls[0]?.[2]).toEqual({})
  })
})

describe("useCanvasArtifactPreviewURL", () => {
  it("posts the preview request and stays disabled until artifact + session", () => {
    mocks.useQuery.mockReturnValue({ data: { url: "https://sandbox/preview" } })

    const result = probe(() =>
      useCanvasArtifactPreviewURL({
        artifactId: "artifact-1",
        sessionId: "session-1",
      })
    )

    const [method, path, init, options] = mocks.useQuery.mock.calls[0] ?? []
    expect(method).toBe("post")
    expect(path).toBe("/v1/canvas/artifacts/{artifactID}/preview-url")
    expect(init).toEqual({
      params: { path: { artifactID: "artifact-1" } },
      body: { session_id: "session-1" },
    })
    expect(options).toMatchObject({ enabled: true })
    expect(result.data).toEqual({
      url: "https://sandbox/preview",
      expiresIn: undefined,
      expiresAt: undefined,
    })

    // Missing session disables the query.
    probe(() =>
      useCanvasArtifactPreviewURL({ artifactId: "artifact-1", sessionId: null })
    )
    expect(mocks.useQuery.mock.calls[1]?.[3]).toMatchObject({ enabled: false })
  })
})

describe("useSendCanvasArtifactComment", () => {
  it("wraps the comment into a session message body", async () => {
    const mutateAsync = vi.fn().mockResolvedValue({})
    mocks.useMutation.mockReturnValue({ mutateAsync })

    const controller = probe(() => useSendCanvasArtifactComment("session-1"))
    expect(mocks.useMutation.mock.calls[0]?.slice(0, 2)).toEqual([
      "post",
      "/v1/sessions/{id}/messages",
    ])

    const comment = canvasArtifactCommentPayload({
      artifact: {
        id: "artifact-1",
        projectId: "project-1",
        name: "Homepage",
        type: "web_page",
      },
      project: { id: "project-1", name: "Launch", artifacts: [] },
      viewport: "desktop",
      body: "Tighten the top nav spacing.",
      selector: '[data-canvas-id="top-nav"]',
      now: new Date("2026-06-26T12:00:00.000Z"),
    })

    await controller.sendComment(comment)
    expect(mutateAsync).toHaveBeenCalledWith({
      params: { path: { id: "session-1" } },
      body: {
        text: "Tighten the top nav spacing.",
        artifact_comments: [comment],
      },
    })
  })

  it("throws when there is no active session", () => {
    mocks.useMutation.mockReturnValue({ mutateAsync: vi.fn() })
    const controller = probe(() => useSendCanvasArtifactComment(null))
    expect(() =>
      controller.sendComment(
        canvasArtifactCommentPayload({
          artifact: { id: "a", name: "A", type: "web_page" },
          viewport: "desktop",
          body: "hi",
        })
      )
    ).toThrow("No active session.")
  })
})
