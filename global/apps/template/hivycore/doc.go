// DO NOT EDIT — hivycore is the Hivy platform core, vendored into every app.
// App builder agents must never modify any file in this package. Platform
// security fixes ship as new template versions; local edits will be lost and
// may break the app's authentication or data access.
//
// Package hivycore wires the platform side of a Hivy app: configuration from
// the injected HIVY_* environment, the launch-JWT auth callback, the
// encrypted-cookie session, the auth middleware, static SPA serving, and the
// typed sheets client over the internal app API. Agent code interacts with it
// only through the exported surface: MustNew/New, (*App).HandleAPI,
// (*App).Sheets, UserFrom, WriteJSON, and (*App).WriteError.
package hivycore

// TemplateVersion identifies the template revision this hivycore was built
// from. It is stamped into app.json, recorded on the app's versions, and
// reported by /healthz — it is the fleet migration handle for platform
// patches.
const TemplateVersion = "1"
