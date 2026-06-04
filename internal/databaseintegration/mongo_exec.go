package databaseintegration

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const mongoIntrospectionSampleLimit = 25

type MongoCollectionInfo struct {
	Collection string           `json:"collection"`
	Fields     []MongoFieldInfo `json:"fields,omitempty"`
}

type MongoFieldInfo struct {
	Path string `json:"path"`
	Type string `json:"type,omitempty"`
}

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

func IntrospectMongo(ctx context.Context, dsn string) ([]MongoCollectionInfo, error) {
	client, dbName, err := openMongo(ctx, dsn)
	if err != nil {
		return nil, err
	}
	defer client.Disconnect(context.WithoutCancel(ctx))
	db := client.Database(dbName)
	names, err := db.ListCollectionNames(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	sort.Strings(names)
	out := make([]MongoCollectionInfo, 0, len(names))
	for _, name := range names {
		fields, err := introspectMongoFields(ctx, db.Collection(name))
		if err != nil {
			return nil, err
		}
		out = append(out, MongoCollectionInfo{Collection: name, Fields: fields})
	}
	return out, nil
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

func introspectMongoFields(ctx context.Context, collection *mongo.Collection) ([]MongoFieldInfo, error) {
	cursor, err := collection.Find(ctx, bson.M{}, options.Find().SetLimit(mongoIntrospectionSampleLimit))
	if err != nil {
		return nil, fmt.Errorf("sample collection %s: %w", collection.Name(), err)
	}
	defer cursor.Close(ctx)
	fields := map[string]string{}
	for cursor.Next(ctx) {
		var doc bson.M
		if err := cursor.Decode(&doc); err != nil {
			return nil, err
		}
		collectMongoFieldPaths("", doc, fields, 0)
	}
	if err := cursor.Err(); err != nil {
		return nil, err
	}
	return mongoFieldMapToList(fields), nil
}

func collectMongoFieldPaths(prefix string, value any, fields map[string]string, depth int) {
	if depth > 6 {
		return
	}
	switch typed := value.(type) {
	case bson.M:
		for key, child := range typed {
			path := joinMongoPath(prefix, key)
			fields[path] = mongoValueType(child)
			collectMongoFieldPaths(path, child, fields, depth+1)
		}
	case map[string]any:
		for key, child := range typed {
			path := joinMongoPath(prefix, key)
			fields[path] = mongoValueType(child)
			collectMongoFieldPaths(path, child, fields, depth+1)
		}
	case bson.A:
		for _, child := range typed {
			collectMongoFieldPaths(prefix, child, fields, depth+1)
		}
	case []any:
		for _, child := range typed {
			collectMongoFieldPaths(prefix, child, fields, depth+1)
		}
	}
}

func joinMongoPath(prefix, key string) string {
	if prefix == "" {
		return key
	}
	return prefix + "." + key
}

func mongoValueType(value any) string {
	switch value.(type) {
	case bson.M, map[string]any:
		return "object"
	case bson.A, []any:
		return "array"
	case string:
		return "string"
	case bool:
		return "boolean"
	case int32, int64, int, float32, float64:
		return "number"
	case nil:
		return "null"
	default:
		return "mixed"
	}
}

func mongoFieldMapToList(fields map[string]string) []MongoFieldInfo {
	out := make([]MongoFieldInfo, 0, len(fields))
	for path, valueType := range fields {
		out = append(out, MongoFieldInfo{Path: path, Type: valueType})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Path < out[j].Path
	})
	return out
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
