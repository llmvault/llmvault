package databaseintegration

import (
	"sort"
	"strings"
)

func buildPredicate(cols columnPair, allowedTables, allowedSchemas map[string]bool) string {
	if cols.tableCol == "" {
		return schemataPredicate(cols.schemaCol, allowedTables, allowedSchemas)
	}
	var parts []string
	if len(allowedSchemas) > 0 {
		parts = append(parts, inClause("lower("+cols.schemaCol+")", sortedKeys(allowedSchemas)))
	}
	if len(allowedTables) > 0 {
		parts = append(parts, tablePredicate(cols.schemaCol, cols.tableCol, allowedTables))
	}
	if len(parts) == 0 {
		return "1=0"
	}
	return strings.Join(parts, " AND ")
}

func tablePredicate(schemaCol, tableCol string, allowedTables map[string]bool) string {
	var terms []string
	for _, value := range sortedKeys(allowedTables) {
		if schema, table, ok := splitDot(value); ok {
			terms = append(terms, "("+equals("lower("+schemaCol+")", schema)+" AND "+equals("lower("+tableCol+")", table)+")")
			continue
		}
		terms = append(terms, equals("lower("+tableCol+")", value))
	}
	return "(" + strings.Join(terms, " OR ") + ")"
}

func schemataPredicate(schemaCol string, allowedTables, allowedSchemas map[string]bool) string {
	if len(allowedSchemas) > 0 {
		return inClause("lower("+schemaCol+")", sortedKeys(allowedSchemas))
	}
	seen := map[string]bool{}
	var schemas []string
	for _, value := range sortedKeys(allowedTables) {
		if schema, _, ok := splitDot(value); ok && !seen[schema] {
			seen[schema] = true
			schemas = append(schemas, schema)
		}
	}
	if len(schemas) == 0 {
		return "1=0"
	}
	return inClause("lower("+schemaCol+")", schemas)
}

func inClause(column string, values []string) string {
	quoted := make([]string, len(values))
	for i, value := range values {
		quoted[i] = quoteLiteral(value)
	}
	return column + " IN (" + strings.Join(quoted, ", ") + ")"
}

func equals(column, value string) string {
	return column + " = " + quoteLiteral(value)
}

func quoteLiteral(value string) string {
	var b strings.Builder
	b.WriteByte('\'')
	for _, r := range strings.ToLower(value) {
		switch r {
		case 0:
			continue
		case '\\':
			b.WriteString(`\\`)
		case '\'':
			b.WriteString(`''`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('\'')
	return b.String()
}

func splitDot(value string) (string, string, bool) {
	parts := strings.SplitN(value, ".", 2)
	if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
		return parts[0], parts[1], true
	}
	return "", "", false
}

func sortedKeys(values map[string]bool) []string {
	out := make([]string, 0, len(values))
	for key := range values {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}
