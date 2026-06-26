import { afterEach, describe, expect, it, vi, type Mock } from "vitest"
import {
  canvasCatalogFileTarget,
  fetchCanvasProjectCatalog,
  parseCanvasProjectCatalog,
} from "@/app/w/(chat)/_lib/canvas-projects"

describe("canvas projects", () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it("parses projects with nested files", () => {
    const catalog = parseCanvasProjectCatalog({
      projects: [
        {
          project_id: "project-1",
          name: "Launch",
          files: [
            {
              file_id: "file-1",
              project_id: "project-1",
              page_id: "page-1",
              name: "Homepage",
              workspace_url:
                "https://canvas.test/#/workspace?file-id=file-1&page-id=page-1",
            },
          ],
        },
        {
          project_id: "project-2",
          files: [{ name: "Missing id" }],
        },
      ],
    })

    expect(catalog.projects).toEqual([
      {
        projectId: "project-1",
        name: "Launch",
        files: [
          {
            fileId: "file-1",
            projectId: "project-1",
            pageId: "page-1",
            name: "Homepage",
            workspaceURL:
              "https://canvas.test/#/workspace?file-id=file-1&page-id=page-1",
          },
        ],
      },
      {
        projectId: "project-2",
        name: "Untitled project",
        files: [],
      },
    ])
  })

  it("builds a Canvas design target for the existing iframe flow", () => {
    const catalog = parseCanvasProjectCatalog({
      projects: [
        {
          project_id: "project-1",
          name: "Launch",
          files: [
            {
              file_id: "file-1",
              page_id: "page-1",
              name: "Homepage",
              workspace_url:
                "https://canvas.test/#/workspace?file-id=file-1&page-id=page-1",
            },
          ],
        },
      ],
    })

    expect(
      canvasCatalogFileTarget(catalog.projects[0].files[0], catalog.projects[0])
    ).toEqual({
      key: "file-1:page-1",
      fileId: "file-1",
      pageId: "page-1",
      sourceUrl:
        "https://canvas.test/#/workspace?file-id=file-1&page-id=page-1",
      fileName: "Homepage",
      projectName: "Launch",
    })
  })

  it("fetches the Canvas project catalog through the proxy", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(
        JSON.stringify({
          projects: [{ project_id: "project-1", name: "Launch", files: [] }],
        }),
        { status: 200, headers: { "Content-Type": "application/json" } }
      )
    )

    await expect(fetchCanvasProjectCatalog()).resolves.toEqual({
      projects: [{ projectId: "project-1", name: "Launch", files: [] }],
    })
    expect(globalThis.fetch as Mock).toHaveBeenCalledWith(
      "/api/proxy/v1/canvas/projects",
      expect.objectContaining({ method: "GET" })
    )
  })
})
