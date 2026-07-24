import { afterEach, describe, expect, it } from "vitest"
import {
  clientConfig,
  PUBLIC_CONFIG_ELEMENT_ID,
  PUBLIC_CONFIG_KEY,
  type PublicConfig,
} from "./public-config"

const originalWindow = globalThis.window
const originalDocument = globalThis.document

afterEach(() => {
  Object.defineProperty(globalThis, "window", {
    configurable: true,
    writable: true,
    value: originalWindow,
  })
  Object.defineProperty(globalThis, "document", {
    configurable: true,
    writable: true,
    value: originalDocument,
  })
})

describe("runtime public config", () => {
  it("reads streamed runtime config before application hydration", () => {
    Object.defineProperty(globalThis, "window", {
      configurable: true,
      writable: true,
      value: {},
    })
    const config: PublicConfig = {
      apiUrl: "https://api.usehivy.test",
      connectionsHost: "https://connections.usehivy.test",
      previewDomain: "preview.usehivy.test",
      docsUrl: "https://usehivy.test/docs",
      tutorialVideos: {},
    }
    Object.defineProperty(globalThis, "document", {
      configurable: true,
      writable: true,
      value: {
        getElementById: (id: string) =>
          id === PUBLIC_CONFIG_ELEMENT_ID
            ? { textContent: JSON.stringify(config) }
            : null,
      },
    })

    expect(clientConfig()).toEqual(config)
    expect(
      (window as unknown as Record<string, PublicConfig>)[PUBLIC_CONFIG_KEY]
    ).toEqual(config)
  })
})
