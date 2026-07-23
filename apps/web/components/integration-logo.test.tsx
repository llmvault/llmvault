import React from "react"
import { renderToStaticMarkup } from "react-dom/server"
import { describe, expect, it } from "vitest"

import { IntegrationLogo } from "./integration-logo"

describe("IntegrationLogo", () => {
  it("renders Grafana from the local brand catalog", () => {
    const html = renderToStaticMarkup(
      React.createElement(IntegrationLogo, { provider: "grafana" })
    )

    expect(html).toContain("<svg")
    expect(html).toContain('aria-label="grafana"')
    expect(html).not.toContain("/images/template-logos/grafana.svg")
  })
})
