package databaseintegration

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/usehivy/hivy/internal/model"
)

// ErrSchemaQualificationRequired is returned when an unqualified table name is
// ambiguous under a policy that permits more than one schema.
var ErrSchemaQualificationRequired = errors.New("schema qualification is required")

const (
	ProviderPostgres = "postgres"
	ProviderMySQL    = "mysql"
	ProviderMongoDB  = "mongodb"
	ProviderRedis    = "redis"
	DefaultMaxRows   = 100
)

type Policy struct {
	AllowedSchemas     []string `json:"allowed_schemas,omitempty"`
	AllowedTables      []string `json:"allowed_tables,omitempty"`
	AllowedCollections []string `json:"allowed_collections,omitempty"`
	AllowedKeys        []string `json:"allowed_keys,omitempty"`
	MaskedFields       []string `json:"masked_fields,omitempty"`
	MaxRows            int      `json:"max_rows,omitempty"`
}

func NormalizeProvider(provider string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case ProviderPostgres, "postgresql":
		return ProviderPostgres, nil
	case ProviderMySQL:
		return ProviderMySQL, nil
	case ProviderMongoDB, "mongo":
		return ProviderMongoDB, nil
	case ProviderRedis:
		return ProviderRedis, nil
	default:
		return "", fmt.Errorf("unsupported database provider")
	}
}

func PolicyFromJSON(raw model.JSON) Policy {
	var policy Policy
	if raw != nil {
		if b, err := json.Marshal(raw); err == nil {
			_ = json.Unmarshal(b, &policy)
		}
	}
	if policy.MaxRows <= 0 || policy.MaxRows > 1000 {
		policy.MaxRows = DefaultMaxRows
	}
	return policy
}

func PolicyToJSON(policy Policy) model.JSON {
	if policy.MaxRows <= 0 || policy.MaxRows > 1000 {
		policy.MaxRows = DefaultMaxRows
	}
	b, _ := json.Marshal(policy)
	var out model.JSON
	_ = json.Unmarshal(b, &out)
	return out
}

func MaskRow(row map[string]any, policy Policy) {
	masks := toSet(policy.MaskedFields)
	for key := range row {
		if masks[strings.ToLower(key)] {
			row[key] = "[masked]"
		}
	}
}

func toSet(values []string) map[string]bool {
	out := make(map[string]bool, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value != "" {
			out[value] = true
		}
	}
	return out
}
