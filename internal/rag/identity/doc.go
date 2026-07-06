// Package identity resolves external source identities (e.g. a GitHub
// user login) to Hivy User records via OAuthAccount lookups.
//
// It maps source principals (by email or login) to Hivy users by
// extending the existing OAuthAccount table (ProviderUserEmail,
// ProviderUserLogin, VerifiedEmails, LastSyncedAt columns) rather than
// introducing a new mapping table.
package identity
