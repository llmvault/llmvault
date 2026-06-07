package handler

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

const orgEnvPrefix = "HIVY_ORG_"

var orgEnvNamePattern = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)

type orgEnvironmentVariableResponse struct {
	Name   string `json:"name"`
	EnvKey string `json:"env_key"`
}

type orgEnvironmentVariablesResponse struct {
	Data []orgEnvironmentVariableResponse `json:"data"`
}

type createOrgEnvironmentVariableRequest struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type updateOrgEnvironmentVariableRequest struct {
	Name  *string `json:"name,omitempty"`
	Value *string `json:"value,omitempty"`
}

func normalizeOrgEnvName(name string) (string, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "", errors.New("name is required")
	}
	upper := strings.ToUpper(trimmed)
	if strings.HasPrefix(upper, orgEnvPrefix) {
		return "", fmt.Errorf("name must not include the %s prefix", orgEnvPrefix)
	}
	if !orgEnvNamePattern.MatchString(upper) {
		return "", errors.New("name must use uppercase letters, numbers, and underscores, and start with a letter or underscore")
	}
	return upper, nil
}

func orgEnvKey(name string) string {
	return orgEnvPrefix + name
}

func orgEnvResponse(name string) orgEnvironmentVariableResponse {
	return orgEnvironmentVariableResponse{Name: name, EnvKey: orgEnvKey(name)}
}

func customOrgEnvResponses(envVars map[string]string) []orgEnvironmentVariableResponse {
	out := make([]orgEnvironmentVariableResponse, 0)
	for key := range envVars {
		if !strings.HasPrefix(key, orgEnvPrefix) {
			continue
		}
		out = append(out, orgEnvironmentVariableResponse{
			Name:   strings.TrimPrefix(key, orgEnvPrefix),
			EnvKey: key,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}
