package databaseintegration

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	sqlDenyPattern  = regexp.MustCompile(`(?i)\b(delete|drop|truncate|alter|update|insert|replace|create|rename|grant|revoke|vacuum|copy|execute|call|merge)\b`)
	sqlTablePattern = regexp.MustCompile(`(?i)\b(from|join)\s+([a-zA-Z_][a-zA-Z0-9_."` + "`" + `]*)`)
)

func ValidateSQL(provider, query string, policy Policy) error {
	query = strings.TrimSpace(query)
	if query == "" {
		return fmt.Errorf("query is required")
	}
	if strings.Count(query, ";") > 1 || strings.Contains(strings.TrimSuffix(query, ";"), ";") {
		return fmt.Errorf("only one SQL statement is allowed")
	}
	normalized := stripSQLComments(query)
	if sqlDenyPattern.MatchString(normalized) {
		return fmt.Errorf("write, schema, privilege, and destructive SQL operations are denied")
	}
	first := firstSQLWord(normalized)
	if !allowedSQLVerb(provider, first) {
		return fmt.Errorf("only read-only SQL statements are allowed")
	}
	return enforceSQLPolicy(normalized, policy)
}

func allowedSQLVerb(provider, first string) bool {
	switch first {
	case "select", "with", "explain":
		return true
	case "show", "describe", "desc":
		return provider == ProviderMySQL
	default:
		return false
	}
}

func enforceSQLPolicy(query string, policy Policy) error {
	allowedTables := toSet(policy.AllowedTables)
	allowedSchemas := toSet(policy.AllowedSchemas)
	if len(allowedTables) == 0 && len(allowedSchemas) == 0 {
		return nil
	}
	for _, table := range sqlTables(query) {
		schema, name := splitQualifiedName(table)
		if len(allowedTables) > 0 && !allowedTables[strings.ToLower(name)] && !allowedTables[strings.ToLower(table)] {
			return fmt.Errorf("table %q is outside the configured database access policy", table)
		}
		if schema != "" && len(allowedSchemas) > 0 && !allowedSchemas[strings.ToLower(schema)] {
			return fmt.Errorf("schema %q is outside the configured database access policy", schema)
		}
	}
	return nil
}

func sqlTables(query string) []string {
	matches := sqlTablePattern.FindAllStringSubmatch(query, -1)
	out := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) > 2 {
			out = append(out, strings.Trim(match[2], `"`+"`"))
		}
	}
	return out
}

func splitQualifiedName(value string) (string, string) {
	parts := strings.Split(strings.Trim(value, `"`+"`"), ".")
	if len(parts) >= 2 {
		return strings.Trim(parts[len(parts)-2], `"`+"`"), strings.Trim(parts[len(parts)-1], `"`+"`")
	}
	return "", strings.Trim(value, `"`+"`")
}

func stripSQLComments(query string) string {
	lines := strings.Split(query, "\n")
	for i, line := range lines {
		if idx := strings.Index(line, "--"); idx >= 0 {
			lines[i] = line[:idx]
		}
	}
	return strings.Join(lines, "\n")
}

func firstSQLWord(query string) string {
	query = strings.TrimLeft(query, " \t\r\n(")
	for _, part := range strings.Fields(query) {
		return strings.ToLower(strings.Trim(part, "();"))
	}
	return ""
}
