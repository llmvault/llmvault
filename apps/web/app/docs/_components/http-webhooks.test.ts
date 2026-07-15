import React from "react"
import { renderToString } from "react-dom/server"
import { describe, expect, it } from "vitest"
import { HttpWebhooks } from "./http-webhooks"

describe("HttpWebhooks", () => {
  it("documents secure asynchronous webhook delivery", () => {
    const html = renderToString(React.createElement(HttpWebhooks))

    expect(html).toContain("Create the webhook")
    expect(html).toContain("Create and store a shared secret")
    expect(html).toContain("Authorization: Bearer")
    expect(html).toContain("below 256 KB")
    expect(html).toContain("Treat the response as acceptance")
    expect(html).toContain("Hivy redacts JSON fields")
    expect(html).toContain("Video placeholder")
    expect(html).toContain("/docs/captures/")
    expect(html).not.toContain("conversation")
    expect(html).not.toContain("—")
  })
})
