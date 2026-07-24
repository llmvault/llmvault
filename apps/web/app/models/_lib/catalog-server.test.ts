import { afterEach, describe, expect, it, vi } from "vitest"

vi.mock("server-only", () => ({}))

import { fetchModelCatalog } from "./catalog-server"

afterEach(() => {
  vi.unstubAllGlobals()
})

describe("server model catalog loader", () => {
  it("loads the complete catalog from the typed public endpoint", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          total: 1,
          models: [
            {
              id: "ling-3.0-flash",
              name: "Ling 3.0 Flash",
              providers: [
                {
                  id: "novita",
                  name: "Novita AI",
                  default: true,
                  priority: 1,
                  cost: { input: 0, output: 0 },
                },
              ],
            },
          ],
        }),
        {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }
      )
    )
    vi.stubGlobal("fetch", fetchMock)

    const models = await fetchModelCatalog("https://api.usehivy.test")

    expect(models).toHaveLength(1)
    expect(models[0]?.id).toBe("ling-3.0-flash")
    const request = fetchMock.mock.calls[0]?.[0] as Request
    expect(request.url).toBe("https://api.usehivy.test/v1/catalog/models")
    expect(request.cache).toBe("force-cache")
  })
})
