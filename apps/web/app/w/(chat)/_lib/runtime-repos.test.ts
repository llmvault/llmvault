import { afterEach, describe, expect, it, vi } from "vitest"
import {
  RuntimeRepoHTTPError,
  fetchRuntimeRepos,
  fetchRuntimeRepoDiff,
  fetchRuntimeRepoFileContent,
} from "@/app/w/(chat)/_lib/runtime-repos"

describe("runtime repository helpers", () => {
  const originalFetch = global.fetch

  afterEach(() => {
    global.fetch = originalFetch
    delete (globalThis as unknown as Record<string, unknown>).window
    vi.restoreAllMocks()
  })

  it("reads desktop repositories through the Tauri bridge", async () => {
    const invoke = vi.fn().mockResolvedValue({
      status: 200,
      body: {
        repos: [
          {
            id: "repo-1",
            name: "repo",
            relative_path: "repos/repo",
            head_sha: "abc",
            base_sha: "def",
          },
        ],
      },
    })
    ;(globalThis as unknown as Record<string, unknown>).window = globalThis
    ;(window as unknown as Record<string, unknown>).__TAURI__ = {
      core: { invoke },
    }

    const repos = await fetchRuntimeRepos({
      sandbox_base_url: "hivy-desktop://runtime",
      token: "desktop-bridge",
    })

    expect(repos).toHaveLength(1)
    expect(invoke).toHaveBeenCalledWith("runtime_request", {
      request: { method: "GET", path: "/repos", body: undefined },
    })
  })

  it("fetches repository file content with path and paging query params", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          repo_id: "repo/one",
          path: "src/app file.tsx",
          content: "export const value = 1\n",
          encoding: "utf-8",
          truncated: false,
          total_lines: 1,
          total_bytes: 23,
          shown_lines: 1,
          offset: 5,
          limit: 20,
        }),
        {
          status: 200,
          headers: { "content-type": "application/json" },
        }
      )
    )
    global.fetch = fetchMock as unknown as typeof fetch

    const content = await fetchRuntimeRepoFileContent(
      {
        sandbox_base_url: "http://sandbox.test/",
        token: "runtime-token",
      },
      "repo/one",
      "src/app file.tsx",
      undefined,
      { offset: 5, limit: 20 }
    )

    expect(content.path).toBe("src/app file.tsx")
    expect(content.content).toBe("export const value = 1\n")
    expect(fetchMock).toHaveBeenCalledTimes(1)
    const [url, init] = fetchMock.mock.calls[0]
    const parsed = new URL(String(url))
    expect(parsed.origin).toBe("http://sandbox.test")
    expect(parsed.pathname).toBe("/repos/repo%2Fone/content")
    expect(parsed.searchParams.get("path")).toBe("src/app file.tsx")
    expect(parsed.searchParams.get("offset")).toBe("5")
    expect(parsed.searchParams.get("limit")).toBe("20")
    expect((init as RequestInit).headers).toMatchObject({
      Accept: "application/json",
      Authorization: "Bearer runtime-token",
      "X-Daytona-Skip-Preview-Warning": "true",
    })
  })

  it("fetches repository diff with optional path and context query params", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          repo_id: "repo/one",
          path: "src/app.tsx",
          diff: "diff --git a/src/app.tsx b/src/app.tsx\n",
        }),
        {
          status: 200,
          headers: { "content-type": "application/json" },
        }
      )
    )
    global.fetch = fetchMock as unknown as typeof fetch

    const diff = await fetchRuntimeRepoDiff(
      {
        sandbox_base_url: "http://sandbox.test/",
        token: "runtime-token",
      },
      "repo/one",
      undefined,
      { path: "src/app.tsx", context: 12 }
    )

    expect(diff.diff).toBe("diff --git a/src/app.tsx b/src/app.tsx\n")
    expect(fetchMock).toHaveBeenCalledTimes(1)
    const [url, init] = fetchMock.mock.calls[0]
    const parsed = new URL(String(url))
    expect(parsed.origin).toBe("http://sandbox.test")
    expect(parsed.pathname).toBe("/repos/repo%2Fone/diff")
    expect(parsed.searchParams.get("path")).toBe("src/app.tsx")
    expect(parsed.searchParams.get("context")).toBe("12")
    expect((init as RequestInit).headers).toMatchObject({
      Accept: "application/json",
      Authorization: "Bearer runtime-token",
      "X-Daytona-Skip-Preview-Warning": "true",
    })
  })

  it("includes runtime response text in HTTP errors", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response("file too large; use offset and limit", {
        status: 400,
      })
    )
    global.fetch = fetchMock as unknown as typeof fetch

    let thrown: unknown
    try {
      await fetchRuntimeRepoFileContent(
        {
          sandbox_base_url: "http://sandbox.test",
          token: "runtime-token",
        },
        "repo-one",
        "large.log"
      )
    } catch (error) {
      thrown = error
    }

    expect(thrown).toBeInstanceOf(RuntimeRepoHTTPError)
    expect(thrown).toMatchObject({
      status: 400,
      message:
        "Sandbox repository request failed with HTTP 400: file too large; use offset and limit",
    })
  })
})
