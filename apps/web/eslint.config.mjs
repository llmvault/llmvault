import { defineConfig, globalIgnores } from "eslint/config"
import nextVitals from "eslint-config-next/core-web-vitals"
import nextTs from "eslint-config-next/typescript"

const eslintConfig = defineConfig([
  ...nextVitals,
  ...nextTs,
  // Override default ignores of eslint-config-next.
  globalIgnores([
    // Default ignores of eslint-config-next:
    ".next/**",
    ".source/**",
    "out/**",
    "build/**",
    "next-env.d.ts",
  ]),
  {
    // Logging hygiene. Use the pino logger from @/lib/logger instead of
    // raw console for anything operational. console.warn/console.error are
    // tolerated as last-resort escape hatches; everything else is a build
    // failure. Sentry's @sentry/nextjs client auto-captures unhandled
    // errors, so most "log this somewhere" needs are already covered.
    rules: {
      "@typescript-eslint/no-unused-vars": [
        "warn",
        {
          argsIgnorePattern: "^_",
          caughtErrorsIgnorePattern: "^_",
          varsIgnorePattern: "^_",
        },
      ],
      "no-console": ["error", { allow: ["warn", "error"] }],
      // Hivy API access goes through the generated `$api` hooks
      // (openapi-react-query), never a hand-written `fetch`. This keeps every
      // request typed and keyed off the OpenAPI contract. Raw `fetch` is
      // permitted for exactly three things, each of which either lives in the
      // sanctioned server-side proxy (app/api/proxy/**, overridden below) or
      // carries an `eslint-disable-next-line no-restricted-globals` with a
      // reason: (1) signed-storage uploads (PUT to a pre-signed object-store
      // URL), (2) direct-to-sandbox calls (browser → sandbox base URL), and
      // (3) SSE — which uses `fetchEventSource`/`EventSource`, not the global
      // `fetch`, and so is unaffected by this rule.
      "no-restricted-globals": [
        "error",
        {
          name: "fetch",
          message:
            "Don't call the global fetch() against the Hivy API — use the generated $api hooks (openapi-react-query). fetch is allowed only for signed-storage uploads, direct-sandbox calls, and SSE (fetchEventSource); add `// eslint-disable-next-line no-restricted-globals` with a reason for those.",
        },
      ],
    },
  },
  {
    // The API proxy IS the sanctioned browser→backend boundary; it is where raw
    // fetch to the Hivy API legitimately lives (token refresh + request
    // forwarding). Exempt it from the no-raw-fetch rule.
    files: ["app/api/proxy/**/*.{ts,tsx}"],
    rules: {
      "no-restricted-globals": "off",
    },
  },
  {
    files: ["app/w/agents/_components/manage-integrations-dialog.tsx"],
    rules: {
      // TanStack Virtual intentionally returns imperative functions. This
      // warning is noisy and not actionable for the virtualized action list.
      "react-hooks/incompatible-library": "off",
    },
  },
  {
    // Build/codegen scripts are CLI-shaped — console output is the point.
    files: ["scripts/**/*.{ts,mts,tsx,js,mjs}"],
    rules: {
      "no-console": "off",
    },
  },
])

export default eslintConfig
