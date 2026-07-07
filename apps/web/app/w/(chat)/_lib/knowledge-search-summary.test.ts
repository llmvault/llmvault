import { describe, expect, it } from "vitest"
import {
  isKnowledgeSearchTool,
  summarizeKnowledgeSearchOutput,
} from "@/app/w/(chat)/_lib/knowledge-search-summary"

function toolOutput(results: unknown[], totalResults?: number): string {
  return JSON.stringify({
    success: true,
    query: "pricing rollout",
    total_results: totalResults ?? results.length,
    result_groups: results.length,
    results,
  })
}

function group(
  source: { kind: string; provider?: string; name?: string },
  docIDs: string[]
) {
  return {
    source_id: "src-1",
    source: { id: "src-1", name: source.name ?? "Source", ...source },
    result_count: docIDs.length,
    chunks: docIDs.map((docID, index) => ({
      id: `${docID}:${index}`,
      score: 0.5,
      doc_id: docID,
      excerpt: "...",
    })),
  }
}

/**
 * Mirrors what the runtime stores durably: the Go tool marshals maps with
 * alphabetical keys, wraps the JSON in an MCP content envelope, and
 * summarize_json cuts the serialized envelope at 2,000 chars + "..."
 * (sandboxes/runtime/crates/runtime/src/handler.rs).
 */
function goOrderedInnerJSON(): string {
  const chunk = (docID: string, part: number) => ({
    // Alphabetical key order, as Go's encoding/json emits maps.
    doc_id: docID,
    excerpt: "lorem ipsum dolor sit amet ".repeat(30),
    id: `${docID}#${part}`,
    link: "https://example.com/x",
    part_index: part,
    score: 0.42,
    semantic_id: "Some document title",
    title: "Some document title",
  })
  const goGroup = (
    source: { kind: string; provider: string },
    docIDs: string[]
  ) => ({
    chunks: docIDs.map((docID, index) => chunk(docID, index)),
    doc_id: docIDs[0],
    link: "https://example.com/x",
    rag_source_id: "src",
    result_count: docIDs.length,
    source: { id: "src", kind: source.kind, name: "Src", provider: source.provider },
    source_id: "src",
    title: "Src",
  })
  return JSON.stringify({
    query: "pricing rollout",
    result_groups: 3,
    results: [
      goGroup({ kind: "WEBSITE", provider: "WEBSITE" }, [
        "https://example.com/a",
        "https://example.com/b",
      ]),
      goGroup({ kind: "INTEGRATION", provider: "slack" }, [
        "C0123ABCDE__1699999999.000100",
        "C0123ABCDE__1699999999.000200",
      ]),
      goGroup({ kind: "INTEGRATION", provider: "linear" }, [
        "linear_issue_uuid-1",
      ]),
    ],
    success: true,
    total_results: 5,
  })
}

function envelopeJSON(): string {
  return JSON.stringify({
    content: [{ type: "text", text: goOrderedInnerJSON() }],
  })
}

function truncatedResultSummary(maxChars: number): string {
  return `${envelopeJSON().slice(0, maxChars)}...`
}

describe("isKnowledgeSearchTool", () => {
  it("matches the tool name and its hivy-prefixed variant", () => {
    expect(isKnowledgeSearchTool("search_knowledge_base")).toBe(true)
    expect(isKnowledgeSearchTool("hivy_search_knowledge_base")).toBe(true)
    expect(isKnowledgeSearchTool("web_search")).toBe(false)
    expect(isKnowledgeSearchTool(undefined)).toBe(false)
  })
})

describe("summarizeKnowledgeSearchOutput", () => {
  it("summarizes website pages with the chrome mark", () => {
    const lines = summarizeKnowledgeSearchOutput(
      toolOutput([
        group({ kind: "WEBSITE", provider: "WEBSITE" }, [
          "https://example.com/a",
          "https://example.com/b",
          "https://example.com/c",
        ]),
      ])
    )
    expect(lines).toEqual([
      {
        key: "website:page",
        provider: "chrome",
        icon: undefined,
        text: "Found 3 website pages",
      },
    ])
  })

  it("renders one aggregate line per linear doc type", () => {
    const lines = summarizeKnowledgeSearchOutput(
      toolOutput([
        group({ kind: "INTEGRATION", provider: "linear" }, [
          "linear_issue_uuid-1",
          "linear_issue_uuid-2",
          "linear_issue_uuid-3",
          "linear_issue_uuid-4",
          "linear_project_uuid-5",
          "linear_initiative_uuid-6",
          "linear_initiative_uuid-7",
        ]),
      ])
    )
    expect(lines?.map((line) => line.text)).toEqual([
      "Found 4 Linear issues",
      "Found 1 Linear project",
      "Found 2 Linear initiatives",
    ])
    expect(lines?.every((line) => line.provider === "linear")).toBe(true)
  })

  it("uses singular forms for a single hit", () => {
    const lines = summarizeKnowledgeSearchOutput(
      toolOutput([
        group({ kind: "INTEGRATION", provider: "github" }, [
          "github_pr_org/repo_42",
        ]),
      ])
    )
    expect(lines?.[0]?.text).toBe("Found 1 GitHub pull request")
  })

  it("keeps the canonical type order regardless of hit order", () => {
    const lines = summarizeKnowledgeSearchOutput(
      toolOutput([
        group({ kind: "INTEGRATION", provider: "github" }, [
          "github_issue_org/repo_7",
          "github_pr_org/repo_42",
        ]),
      ])
    )
    expect(lines?.map((line) => line.text)).toEqual([
      "Found 1 GitHub pull request",
      "Found 1 GitHub issue",
    ])
  })

  it("summarizes slack messages and notion pages", () => {
    const lines = summarizeKnowledgeSearchOutput(
      toolOutput([
        group({ kind: "INTEGRATION", provider: "slack" }, [
          "C123__1699.001",
          "C123__1699.002",
        ]),
        group({ kind: "INTEGRATION", provider: "notion" }, [
          "notion_page_uuid-1",
        ]),
      ])
    )
    expect(lines?.map((line) => line.text)).toEqual([
      "Found 2 Slack messages",
      "Found 1 Notion page",
    ])
  })

  it("maps github-app integration providers to github", () => {
    const lines = summarizeKnowledgeSearchOutput(
      toolOutput([
        group({ kind: "INTEGRATION", provider: "github-app" }, [
          "github_issue_org/repo_7",
        ]),
      ])
    )
    expect(lines?.[0]).toMatchObject({
      key: "github:issue",
      provider: "github",
      text: "Found 1 GitHub issue",
    })
  })

  it("merges multiple sources of the same kind into one line", () => {
    const lines = summarizeKnowledgeSearchOutput(
      toolOutput([
        group({ kind: "WEBSITE", provider: "WEBSITE", name: "Docs" }, [
          "https://docs.example.com/a",
        ]),
        group({ kind: "WEBSITE", provider: "WEBSITE", name: "Blog" }, [
          "https://blog.example.com/b",
        ]),
      ])
    )
    expect(lines).toHaveLength(1)
    expect(lines?.[0]?.text).toBe("Found 2 website pages")
  })

  it("counts documents, not chunks", () => {
    const lines = summarizeKnowledgeSearchOutput(
      toolOutput([
        {
          ...group({ kind: "WEBSITE", provider: "WEBSITE" }, []),
          chunks: [
            { id: "p1", doc_id: "https://example.com/a", part_index: 0 },
            { id: "p2", doc_id: "https://example.com/a", part_index: 1 },
            { id: "p3", doc_id: "https://example.com/b", part_index: 0 },
          ],
        },
      ])
    )
    expect(lines?.[0]?.text).toBe("Found 2 website pages")
  })

  it("uses a file icon for uploads", () => {
    const lines = summarizeKnowledgeSearchOutput(
      toolOutput([
        group({ kind: "FILE_UPLOAD", provider: "FILE_UPLOAD" }, ["upload-1"]),
      ])
    )
    expect(lines?.[0]).toMatchObject({
      key: "file_upload:document",
      provider: undefined,
      icon: "file-text",
      text: "Found 1 uploaded document",
    })
  })

  it("infers the family from doc_id prefixes when source is missing", () => {
    const lines = summarizeKnowledgeSearchOutput(
      toolOutput([
        {
          source_id: "src-1",
          result_count: 1,
          chunks: [{ id: "p1", doc_id: "linear_issue_uuid-1" }],
        },
      ])
    )
    expect(lines?.[0]?.text).toBe("Found 1 Linear issue")
  })

  it("returns an empty list for a successful empty search", () => {
    expect(summarizeKnowledgeSearchOutput(toolOutput([]))).toEqual([])
  })

  it("recovers a grouped summary from a truncated result_summary", () => {
    const lines = summarizeKnowledgeSearchOutput(truncatedResultSummary(2000))
    expect(lines).not.toBeNull()
    expect(lines!.length).toBeGreaterThan(0)
    // The first group (website) fits within the 2,000-char budget intact.
    expect(lines![0]).toMatchObject({ key: "website:page", provider: "chrome" })
    expect(lines![0]!.text).toMatch(/^Found \d+\+? website pages?$/)
    // The cut lands mid-payload: the aggregate it landed in is a lower bound
    // ("N+"). No vague "more sources" rows — every line names its source.
    const texts = lines!.map((line) => line.text)
    expect(texts.some((text) => /\d+\+/.test(text))).toBe(true)
    expect(texts.every((text) => /^Found \d+\+? /.test(text))).toBe(true)
  })

  it("parses the full untruncated envelope exactly", () => {
    const lines = summarizeKnowledgeSearchOutput(envelopeJSON())
    expect(lines?.map((line) => line.text)).toEqual([
      "Found 2 website pages",
      "Found 2 Slack messages",
      "Found 1 Linear issue",
    ])
  })

  it("never throws on any truncation point of a real payload", () => {
    const full = envelopeJSON()
    for (let cut = 10; cut < full.length; cut += 7) {
      const summary = `${full.slice(0, cut)}...`
      expect(() => summarizeKnowledgeSearchOutput(summary)).not.toThrow()
    }
  })

  it("returns null for unparseable or unexpected payloads", () => {
    expect(summarizeKnowledgeSearchOutput(undefined)).toBeNull()
    expect(summarizeKnowledgeSearchOutput("")).toBeNull()
    expect(summarizeKnowledgeSearchOutput("Error: query is required")).toBeNull()
    expect(summarizeKnowledgeSearchOutput('{"success":false}')).toBeNull()
    expect(
      summarizeKnowledgeSearchOutput('{"success":true,"results":"nope"}')
    ).toBeNull()
    expect(summarizeKnowledgeSearchOutput('["not","an","object"]')).toBeNull()
  })
})
