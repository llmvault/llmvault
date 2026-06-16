// Package credentials is the single place that decides which Credential an
// LLM calls should use.
//
// There are two kinds of credentials in the system:
//
//   - Org credentials: owned by a customer org, created via the user-facing API.
//   - System credentials (org_id = NULL): owned by the platform, created via
//     admin-only endpoints.
//
// Agent runtime credential-resolution routes through ResolveForModel. The user
// picks a canonical model; the backend finds the first active org credential
// that supports it, then falls back to the first active system credential.
package credentials
