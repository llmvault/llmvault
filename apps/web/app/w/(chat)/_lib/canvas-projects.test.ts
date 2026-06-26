import { describe, expect, it } from "vitest"
import { canvasCatalogFileTarget } from "@/app/w/(chat)/_lib/canvas-projects"

describe("canvas projects", () => {
  it("builds a Canvas design target for the existing iframe flow", () => {
    expect(
      canvasCatalogFileTarget(
        {
          file_id: "file-1",
          page_id: "page-1",
          name: "Homepage",
          workspace_url:
            "https://canvas.test/#/workspace?file-id=file-1&page-id=page-1",
        },
        { project_id: "project-1", name: "Launch", files: [] }
      )
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
})
