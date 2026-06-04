package databaseintegration

import (
	"encoding/json"
	"fmt"
	"strings"
)

var deniedMongoCommands = map[string]bool{
	"insert": true, "update": true, "delete": true, "findandmodify": true,
	"drop": true, "dropdatabase": true, "dropindexes": true, "create": true,
	"createindexes": true, "collmod": true, "renamecollection": true,
	"grantrolestouser": true, "revokerolesfromuser": true,
}

var allowedMongoCommands = map[string]bool{
	"find": true, "aggregate": true, "count": true, "distinct": true,
}

func ValidateMongoCommand(body []byte, policy Policy) (map[string]any, error) {
	var cmd map[string]any
	if err := json.Unmarshal(body, &cmd); err != nil {
		return nil, fmt.Errorf("invalid MongoDB command JSON")
	}
	if len(cmd) == 0 {
		return nil, fmt.Errorf("MongoDB command is required")
	}
	command, collection := mongoCommandAndCollection(cmd)
	if deniedMongoCommands[command] {
		return nil, fmt.Errorf("write, schema, privilege, and destructive MongoDB commands are denied")
	}
	if !allowedMongoCommands[command] {
		return nil, fmt.Errorf("only read-only MongoDB commands are allowed")
	}
	if err := enforceMongoPolicy(collection, policy); err != nil {
		return nil, err
	}
	applyMongoLimit(cmd, policy.MaxRows)
	return cmd, nil
}

func mongoCommandAndCollection(cmd map[string]any) (string, string) {
	for key, value := range cmd {
		lower := strings.ToLower(strings.TrimSpace(key))
		if allowedMongoCommands[lower] || deniedMongoCommands[lower] {
			if s, ok := value.(string); ok {
				return lower, s
			}
			return lower, ""
		}
	}
	return "", ""
}

func enforceMongoPolicy(collection string, policy Policy) error {
	allowed := toSet(policy.AllowedCollections)
	if len(allowed) == 0 || collection == "" {
		return nil
	}
	if !allowed[strings.ToLower(collection)] {
		return fmt.Errorf("collection %q is outside the configured database access policy", collection)
	}
	return nil
}

func applyMongoLimit(cmd map[string]any, maxRows int) {
	if maxRows <= 0 || maxRows > 1000 {
		maxRows = DefaultMaxRows
	}
	if current, ok := cmd["limit"].(float64); ok && current > 0 && current < float64(maxRows) {
		return
	}
	cmd["limit"] = maxRows
}
