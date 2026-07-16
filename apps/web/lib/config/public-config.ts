// Runtime public configuration.
//
// These values are read from PLAIN server-side env vars (NOT NEXT_PUBLIC_*, which
// Next bakes into the JS bundle at build time and cannot be changed in a prebuilt
// image). The root layout injects them into the initial HTML as
// `window.__HIVY_ENV__` at request time; the client reads them via clientConfig().
// This lets a single prebuilt web image be repointed at any domain purely by
// setting env in docker-compose — the same model as the Go backend.

type PublicConfig = {
  /** Public base URL of the API as reached by the BROWSER (OAuth redirects, SSE). */
  apiUrl: string
  /** Host serving integration connect flows + template logos. */
  connectionsHost: string
  /** Wildcard preview host suffix ({port}-{id}.<domain>) for sandbox previews. */
  previewDomain: string
  /** Documentation root used by contextual in-product help. */
  docsUrl: string
  /** Optional YouTube URLs for onboarding and contextual tutorials. */
  tutorialVideos: Record<string, string>
}

/** Global key under which the server injects the config for the client to read. */
export const PUBLIC_CONFIG_KEY = "__HIVY_ENV__"

/**
 * Reads and validates the public config from server-side env. Throws if a
 * required var is missing so a misconfigured container fails fast at the first
 * request (mirrors the backend's required-env philosophy). Server-only path —
 * on the client, clientConfig() reads the injected window value instead.
 */
function readServerPublicConfig(): PublicConfig {
  // apiUrl: browser-facing public API URL. HIVY_PUBLIC_API_URL lets split-network
  // deploys (browser vs. server-internal) differ; falls back to HIVY_API_URL.
  const apiUrl = (
    process.env.HIVY_PUBLIC_API_URL ||
    process.env.HIVY_API_URL ||
    ""
  ).trim()
  const connectionsHost = (process.env.HIVY_CONNECTIONS_HOST || "").trim()
  const previewDomain = (process.env.HIVY_PREVIEW_BASE_DOMAIN || "").trim()
  const docsUrl = (process.env.HIVY_DOCS_URL || "").trim().replace(/\/$/, "")
  const tutorialVideos = {
    welcome: (process.env.HIVY_WELCOME_VIDEO_URL || "").trim(),
    automations: (process.env.HIVY_TUTORIAL_AUTOMATIONS_VIDEO_URL || "").trim(),
    schedules: (process.env.HIVY_TUTORIAL_SCHEDULES_VIDEO_URL || "").trim(),
    webhooks: (process.env.HIVY_TUTORIAL_WEBHOOKS_VIDEO_URL || "").trim(),
    connections: (process.env.HIVY_TUTORIAL_CONNECTIONS_VIDEO_URL || "").trim(),
    knowledge: (process.env.HIVY_TUTORIAL_KNOWLEDGE_VIDEO_URL || "").trim(),
    teams: (process.env.HIVY_TUTORIAL_TEAMS_VIDEO_URL || "").trim(),
  }

  const missing: string[] = []
  if (!apiUrl) missing.push("HIVY_PUBLIC_API_URL (or HIVY_API_URL)")
  if (!connectionsHost) missing.push("HIVY_CONNECTIONS_HOST")
  if (!previewDomain) missing.push("HIVY_PREVIEW_BASE_DOMAIN")
  if (missing.length > 0) {
    throw new Error(
      `Missing required public config env: ${missing.join(", ")}. ` +
        `Set these on the web service (e.g. in docker-compose) so the frontend can be served under your own domain.`
    )
  }

  return {
    apiUrl,
    connectionsHost,
    previewDomain,
    docsUrl,
    tutorialVideos,
  }
}

/**
 * Returns the public config. On the client, reads the value injected by the
 * server into window.__HIVY_ENV__. During SSR, reads server env directly.
 */
export function clientConfig(): PublicConfig {
  if (typeof window !== "undefined") {
    const injected = (
      window as unknown as Record<string, PublicConfig | undefined>
    )[PUBLIC_CONFIG_KEY]
    if (!injected) {
      throw new Error(
        `window.${PUBLIC_CONFIG_KEY} is not set — the runtime public config was not injected. ` +
          `Ensure <PublicConfigScript /> renders in the root layout.`
      )
    }
    return injected
  }
  return readServerPublicConfig()
}

/**
 * Serializes the validated server config for inline <script> injection.
 * Escapes `<` to avoid breaking out of the script tag.
 */
export function serializePublicConfig(): string {
  return JSON.stringify(readServerPublicConfig()).replace(/</g, "\\u003c")
}

/**
 * Test helper: injects a public config into window so isomorphic code that calls
 * clientConfig() works under vitest/jsdom. Defaults mirror the legacy hosted
 * values so existing assertions need minimal changes; override per test as needed.
 */
export function setPublicConfigForTests(
  overrides: Partial<PublicConfig> = {}
): void {
  const base: PublicConfig = {
    apiUrl: "https://api.usehivy.test",
    connectionsHost: "connections.usehivy.test",
    previewDomain: "preview.usehivy.test",
    docsUrl: "https://docs.usehivy.test",
    tutorialVideos: {},
  }
  ;(globalThis as unknown as Record<string, unknown>).window ??= {}
  ;(window as unknown as Record<string, PublicConfig>)[PUBLIC_CONFIG_KEY] = {
    ...base,
    ...overrides,
  }
}
