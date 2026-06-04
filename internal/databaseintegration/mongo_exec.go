package databaseintegration

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func ExecuteMongo(ctx context.Context, dsn string, body []byte, policy Policy) (map[string]any, error) {
	cmd, err := ValidateMongoCommand(body, policy)
	if err != nil {
		return nil, err
	}
	client, dbName, err := openMongo(ctx, dsn)
	if err != nil {
		return nil, err
	}
	defer client.Disconnect(context.WithoutCancel(ctx))
	var result bson.M
	if err := client.Database(dbName).RunCommand(ctx, bson.M(cmd)).Decode(&result); err != nil {
		return nil, fmt.Errorf("execute MongoDB command: %w", err)
	}
	return maskMongoResult(result, policy), nil
}

func TestMongo(ctx context.Context, dsn string) error {
	client, _, err := openMongo(ctx, dsn)
	if err != nil {
		return err
	}
	defer client.Disconnect(context.WithoutCancel(ctx))
	return client.Ping(ctx, nil)
}

func IntrospectMongo(ctx context.Context, dsn string) ([]string, error) {
	client, dbName, err := openMongo(ctx, dsn)
	if err != nil {
		return nil, err
	}
	defer client.Disconnect(context.WithoutCancel(ctx))
	return client.Database(dbName).ListCollectionNames(ctx, bson.M{})
}

func openMongo(ctx context.Context, dsn string) (*mongo.Client, string, error) {
	client, err := mongo.Connect(options.Client().ApplyURI(dsn).SetConnectTimeout(5 * time.Second))
	if err != nil {
		return nil, "", err
	}
	dbName := mongoDatabaseName(dsn)
	if dbName == "" {
		dbName = "admin"
	}
	return client, dbName, nil
}

func mongoDatabaseName(dsn string) string {
	parsed, err := url.Parse(dsn)
	if err != nil {
		return ""
	}
	return strings.Trim(parsed.Path, "/")
}

func maskMongoResult(result map[string]any, policy Policy) map[string]any {
	masks := toSet(policy.MaskedFields)
	if len(masks) == 0 {
		return result
	}
	maskMongoValue(result, masks)
	return result
}

func maskMongoValue(value any, masks map[string]bool) {
	switch v := value.(type) {
	case map[string]any:
		for key, child := range v {
			if masks[strings.ToLower(key)] {
				v[key] = "[masked]"
			} else {
				maskMongoValue(child, masks)
			}
		}
	case bson.M:
		for key, child := range v {
			if masks[strings.ToLower(key)] {
				v[key] = "[masked]"
			} else {
				maskMongoValue(child, masks)
			}
		}
	case []any:
		for _, child := range v {
			maskMongoValue(child, masks)
		}
	}
}
