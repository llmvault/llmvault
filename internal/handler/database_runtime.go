package handler

import (
	"context"

	dbi "github.com/usehivy/hivy/internal/databaseintegration"
)

func executeDatabaseQuery(ctx context.Context, provider, dsn string, body []byte, policy dbi.Policy) (any, error) {
	if provider == dbi.ProviderMongoDB {
		return dbi.ExecuteMongo(ctx, dsn, body, policy)
	}
	return dbi.ExecuteSQL(ctx, provider, dsn, string(body), policy)
}

func testDatabaseConnection(ctx context.Context, provider, dsn string) error {
	if provider == dbi.ProviderMongoDB {
		return dbi.TestMongo(ctx, dsn)
	}
	return dbi.TestSQL(ctx, provider, dsn)
}

func introspectDatabase(ctx context.Context, provider, dsn string) (any, error) {
	if provider == dbi.ProviderMongoDB {
		return dbi.IntrospectMongo(ctx, dsn)
	}
	return dbi.IntrospectSQL(ctx, provider, dsn)
}
