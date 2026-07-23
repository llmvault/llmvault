import { createElement } from "react"
import { renderToStaticMarkup } from "react-dom/server"
import { describe, expect, it, vi } from "vitest"

const mocks = vi.hoisted(() => ({
  replace: vi.fn(),
  useQuery: vi.fn((..._args: unknown[]) => ({
    data: undefined,
    isError: false,
    isLoading: false,
  })),
}))

vi.mock("next/navigation", () => ({
  useParams: () => ({ id: "source-123" }),
  useRouter: () => ({ replace: mocks.replace }),
}))

vi.mock("@/lib/auth/auth-context", () => ({
  useAuth: () => ({ isLoading: false }),
}))

vi.mock("@/lib/auth/use-role", () => ({
  useIsAdmin: () => false,
}))

vi.mock("@/lib/api/hooks", () => ({
  $api: { useQuery: mocks.useQuery },
}))

const { default: KnowledgeDocumentsPageContent } = await import(
  "./_knowledge-documents-page"
)

describe("KnowledgeDocumentsPageContent", () => {
  it("does not render or load knowledge data for a regular member", () => {
    const html = renderToStaticMarkup(
      createElement(KnowledgeDocumentsPageContent)
    )

    expect(html).toBe("")
    expect(mocks.useQuery).toHaveBeenCalledTimes(3)
    for (const call of mocks.useQuery.mock.calls) {
      expect(call[3]).toEqual({ enabled: false })
    }
  })
})
