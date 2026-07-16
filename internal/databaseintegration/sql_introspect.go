package databaseintegration

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var (
	fromJoinTargetPattern = regexp.MustCompile(`(?i)\b(?:from|join)\s+("?[a-zA-Z_][a-zA-Z0-9_$]*"?(?:\s*\.\s*"?[a-zA-Z_][a-zA-Z0-9_$]*"?)?)`)
	catalogSchemaPattern  = regexp.MustCompile(`(?i)\b(information_schema|pg_catalog)\b`)
	aliasFollowPattern    = regexp.MustCompile(`(?i)^\s+(?:as\s+)?("?[a-zA-Z_][a-zA-Z0-9_$]*"?)`)
)

type columnPair struct {
	schemaCol string
	tableCol  string
}

var infoSchemaRelations = map[string]columnPair{
	"tables":                  {"table_schema", "table_name"},
	"columns":                 {"table_schema", "table_name"},
	"views":                   {"table_schema", "table_name"},
	"table_constraints":       {"table_schema", "table_name"},
	"key_column_usage":        {"table_schema", "table_name"},
	"constraint_column_usage": {"table_schema", "table_name"},
	"schemata":                {"schema_name", ""},
}

var pgCatalogRelations = map[string]columnPair{
	"pg_tables": {"schemaname", "tablename"},
	"pg_views":  {"schemaname", "viewname"},
}

const (
	targetUser = iota
	targetCatalog
	targetDeny
)

type catalogTarget struct {
	start    int
	end      int
	schema   string
	relation string
	cols     columnPair
}

var reservedFollow = map[string]bool{
	"on": true, "where": true, "group": true, "order": true, "having": true,
	"limit": true, "offset": true, "union": true, "intersect": true, "except": true,
	"join": true, "inner": true, "left": true, "right": true, "full": true,
	"outer": true, "cross": true, "natural": true, "using": true, "and": true,
	"or": true, "window": true, "fetch": true, "for": true, "into": true,
}

func rewriteRestrictedSQL(provider, exec, masked string, allowedTables, allowedSchemas map[string]bool) (string, error) {
	matches := fromJoinTargetPattern.FindAllStringSubmatchIndex(masked, -1)
	var rewrites []catalogTarget
	var catalogRanges [][2]int
	for _, m := range matches {
		gs, ge := m[2], m[3]
		raw := masked[gs:ge]
		schema, name := parseQualified(raw)
		kind, canonicalSchema, cols := classifyTarget(provider, schema, name, allowedTables)
		switch kind {
		case targetDeny:
			return "", fmt.Errorf("relation %q is outside the configured database access policy", strings.TrimSpace(raw))
		case targetCatalog:
			rewrites = append(rewrites, catalogTarget{gs, ge, canonicalSchema, strings.ToLower(name), cols})
			catalogRanges = append(catalogRanges, [2]int{gs, ge})
		case targetUser:
			if err := checkUserTable(schema, name, raw, allowedTables, allowedSchemas); err != nil {
				return "", err
			}
		}
	}
	if err := guardCatalogTokens(masked, catalogRanges); err != nil {
		return "", err
	}
	if len(rewrites) == 0 {
		return exec, nil
	}
	return spliceRewrites(exec, masked, rewrites, allowedTables, allowedSchemas), nil
}

func classifyTarget(provider, schema, name string, allowedTables map[string]bool) (int, string, columnPair) {
	ls := strings.ToLower(strings.TrimSpace(schema))
	ln := strings.ToLower(strings.TrimSpace(name))
	switch ls {
	case "information_schema":
		if cols, ok := infoSchemaRelations[ln]; ok {
			return targetCatalog, "information_schema", cols
		}
		return targetDeny, "", columnPair{}
	case "pg_catalog":
		if provider == ProviderPostgres {
			if cols, ok := pgCatalogRelations[ln]; ok {
				return targetCatalog, "pg_catalog", cols
			}
		}
		return targetDeny, "", columnPair{}
	case "":
		if allowedTables[ln] {
			return targetUser, "", columnPair{}
		}
		if provider == ProviderPostgres && strings.HasPrefix(ln, "pg_") {
			if cols, ok := pgCatalogRelations[ln]; ok {
				return targetCatalog, "pg_catalog", cols
			}
			return targetDeny, "", columnPair{}
		}
		if cols, ok := infoSchemaRelations[ln]; ok {
			return targetCatalog, "information_schema", cols
		}
		return targetUser, "", columnPair{}
	default:
		return targetUser, "", columnPair{}
	}
}

func checkUserTable(schema, name, raw string, allowedTables, allowedSchemas map[string]bool) error {
	ln := strings.ToLower(strings.TrimSpace(name))
	full := ln
	ls := strings.ToLower(strings.TrimSpace(schema))
	schemas := sortedSetValues(allowedSchemas)
	if ls == "" && len(schemas) > 1 {
		exampleSchema := schemas[0]
		for _, allowedSchema := range schemas {
			if allowedTables[allowedSchema+"."+ln] {
				exampleSchema = allowedSchema
				break
			}
		}
		return fmt.Errorf(
			"%w: table %q does not include a schema, but this connection permits multiple schemas (%s); always use <schema>.<table>, for example %s.%s",
			ErrSchemaQualificationRequired,
			strings.TrimSpace(raw),
			strings.Join(schemas, ", "),
			exampleSchema,
			ln,
		)
	}
	if ls == "" && len(schemas) == 1 {
		ls = schemas[0]
	}
	if ls != "" {
		full = ls + "." + ln
	}
	if len(allowedTables) > 0 && !allowedTables[ln] && !allowedTables[full] {
		return fmt.Errorf("table %q is outside the configured database access policy", strings.TrimSpace(raw))
	}
	if ls != "" && len(allowedSchemas) > 0 && !allowedSchemas[ls] {
		return fmt.Errorf("schema %q is outside the configured database access policy", ls)
	}
	return nil
}

func sortedSetValues(values map[string]bool) []string {
	out := make([]string, 0, len(values))
	for value, allowed := range values {
		if allowed {
			out = append(out, value)
		}
	}
	sort.Strings(out)
	return out
}

func guardCatalogTokens(masked string, ranges [][2]int) error {
	for _, loc := range catalogSchemaPattern.FindAllStringIndex(masked, -1) {
		if !withinRange(loc[0], ranges) {
			return fmt.Errorf("catalog reference is outside the supported introspection surface")
		}
	}
	return nil
}

func withinRange(pos int, ranges [][2]int) bool {
	for _, r := range ranges {
		if pos >= r[0] && pos < r[1] {
			return true
		}
	}
	return false
}

func spliceRewrites(exec, masked string, rewrites []catalogTarget, allowedTables, allowedSchemas map[string]bool) string {
	sort.Slice(rewrites, func(a, b int) bool { return rewrites[a].start > rewrites[b].start })
	used := map[string]int{}
	out := exec
	for _, rw := range rewrites {
		pred := buildPredicate(rw.cols, allowedTables, allowedSchemas)
		sub := "(SELECT * FROM " + rw.schema + "." + rw.relation + " WHERE " + pred + ")"
		if !hasAliasFollowing(masked, rw.end) {
			sub += " AS " + uniqueAlias(rw.relation, used)
		}
		out = out[:rw.start] + sub + out[rw.end:]
	}
	return out
}

func hasAliasFollowing(masked string, end int) bool {
	m := aliasFollowPattern.FindStringSubmatch(masked[end:])
	if m == nil {
		return false
	}
	ident := strings.ToLower(strings.Trim(m[1], `"`))
	return !reservedFollow[ident]
}

func uniqueAlias(relation string, used map[string]int) string {
	used[relation]++
	if used[relation] == 1 {
		return relation
	}
	return fmt.Sprintf("%s_%d", relation, used[relation])
}

func parseQualified(raw string) (string, string) {
	parts := strings.SplitN(raw, ".", 2)
	if len(parts) == 2 {
		return cleanIdent(parts[0]), cleanIdent(parts[1])
	}
	return "", cleanIdent(parts[0])
}

func cleanIdent(value string) string {
	return strings.TrimSpace(strings.Trim(strings.TrimSpace(value), `"`))
}
