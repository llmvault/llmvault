package databaseintegration

import (
	"fmt"
	"regexp"
	"strings"
)

var sqlDenyPattern = regexp.MustCompile(`(?i)\b(delete|drop|truncate|alter|update|insert|replace|create|rename|grant|revoke|vacuum|copy|execute|call|merge)\b`)

func ValidateSQL(provider, query string, policy Policy) error {
	_, err := PrepareSQL(provider, query, policy)
	return err
}

func PrepareSQL(provider, query string, policy Policy) (string, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return "", fmt.Errorf("query is required")
	}
	exec, masked, err := normalizeSQL(provider, query)
	if err != nil {
		return "", err
	}
	if strings.Count(masked, ";") > 1 || strings.Contains(strings.TrimSuffix(masked, ";"), ";") {
		return "", fmt.Errorf("only one SQL statement is allowed")
	}
	if sqlDenyPattern.MatchString(masked) {
		return "", fmt.Errorf("write, schema, privilege, and destructive SQL operations are denied")
	}
	first := firstSQLWord(masked)
	if !allowedSQLVerb(provider, first) {
		return "", fmt.Errorf("only read-only SQL statements are allowed")
	}
	return enforceSQLPolicy(provider, query, exec, masked, first, policy)
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

func enforceSQLPolicy(provider, original, exec, masked, first string, policy Policy) (string, error) {
	allowedTables := toSet(policy.AllowedTables)
	allowedSchemas := toSet(policy.AllowedSchemas)
	if len(allowedTables) == 0 && len(allowedSchemas) == 0 {
		return original, nil
	}
	if provider == ProviderMySQL && (first == "show" || first == "describe" || first == "desc") {
		return "", fmt.Errorf("catalog listing commands are denied under an access policy; query information_schema instead")
	}
	return rewriteRestrictedSQL(provider, exec, masked, allowedTables, allowedSchemas)
}

func firstSQLWord(query string) string {
	query = strings.TrimLeft(query, " \t\r\n(")
	for _, part := range strings.Fields(query) {
		return strings.ToLower(strings.Trim(part, "();"))
	}
	return ""
}
