import { createElement } from "react"
import { renderToStaticMarkup } from "react-dom/server"
import { describe, expect, it, vi } from "vitest"
import { CredentialPanel } from "./credentials-admin"
import { emptyCredentialForm } from "./types"

describe("CredentialPanel", () => {
  it("offers an inference connection test before saving", () => {
    const html = renderToStaticMarkup(
      createElement(CredentialPanel, {
        credentials: [],
        providers: [
          {
            id: "deepseek",
            name: "DeepSeek",
            base_url: "https://api.deepseek.com",
            default_auth_scheme: "bearer",
            model_ids: ["deepseek-v4-flash", "deepseek-v4-pro"],
            test_model_id: "deepseek-v4-flash",
          },
        ],
        form: {
          ...emptyCredentialForm,
          provider_id: "deepseek",
          base_url: "https://api.deepseek.com",
          api_key: "test-key",
        },
        saving: false,
        testing: false,
        tested: false,
        revokingID: null,
        onFormChange: vi.fn(),
        onSubmit: vi.fn(),
        onTest: vi.fn(),
        onRevoke: vi.fn(),
      })
    )

    expect(html).toContain("Test connection")
    expect(html).toContain("Save credential")
    expect(html).toContain("DeepSeek")
  })
})
