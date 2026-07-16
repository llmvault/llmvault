package databaseintegration

import "context"

// Execute runs one policy-checked read-only operation against a supported
// database provider.
func Execute(ctx context.Context, provider, dsn string, body []byte, policy Policy) (any, error) {
	if provider == ProviderMongoDB {
		return ExecuteMongo(ctx, dsn, body, policy)
	}
	if provider == ProviderRedis {
		return ExecuteRedis(ctx, dsn, body, policy)
	}
	return ExecuteSQL(ctx, provider, dsn, string(body), policy)
}
