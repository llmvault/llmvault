package sandbox

import "context"

// AliasProvider is an optional capability for providers that support stable
// alias hostnames mapping to a (sandbox, port), surviving sandbox recreation.
// Only the microsandbox provider implements it; callers type-assert against
// this interface and skip the operation when a provider does not satisfy it.
type AliasProvider interface {
	// ClaimAlias claims a new alias or repoints an existing one to the given
	// sandbox and guest port, returning the public URL. Repoint is
	// last-write-wins by design (an app redeployed into a fresh sandbox keeps
	// its alias by moving the mapping).
	ClaimAlias(ctx context.Context, alias string, sandboxExternalID string, port int) (string, error)
	// ReleaseAlias removes an alias mapping. Releasing an unknown alias is a
	// no-op success.
	ReleaseAlias(ctx context.Context, alias string) error
}
