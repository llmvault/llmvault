package handler

import (
	"context"

	dbi "github.com/usehivy/hivy/internal/databaseintegration"
)

func executeDatabaseQuery(ctx context.Context, provider, dsn string, body []byte, policy dbi.Policy) (any, error) {
	return dbi.Execute(ctx, provider, dsn, body, policy)
}

func testDatabaseConnection(ctx context.Context, provider, dsn string) error {
	if provider == dbi.ProviderMongoDB {
		return dbi.TestMongo(ctx, dsn)
	}
	if provider == dbi.ProviderRedis {
		return dbi.TestRedis(ctx, dsn)
	}
	return dbi.TestSQL(ctx, provider, dsn)
}

func introspectDatabase(ctx context.Context, provider, dsn string) (any, error) {
	if provider == dbi.ProviderMongoDB {
		return dbi.IntrospectMongo(ctx, dsn)
	}
	if provider == dbi.ProviderRedis {
		return dbi.IntrospectRedis(ctx, dsn)
	}
	return dbi.IntrospectSQL(ctx, provider, dsn)
}
