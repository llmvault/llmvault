package handler

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

const teamEnvInjectPrefix = "__ENV__"
const hivyReservedEnvPrefix = "HIVY_"
const maxTeamEnvDescriptionLen = 500

var teamEnvNamePattern = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)

type teamEnvironmentVariableResponse struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type teamEnvironmentVariablesResponse struct {
	Data []teamEnvironmentVariableResponse `json:"data"`
}

type createTeamEnvironmentVariableRequest struct {
	Name        string `json:"name"`
	Value       string `json:"value"`
	Description string `json:"description,omitempty"`
}

type updateTeamEnvironmentVariableRequest struct {
	Name        *string `json:"name,omitempty"`
	Value       *string `json:"value,omitempty"`
	Description *string `json:"description,omitempty"`
}

func normalizeTeamEnvName(name string) (string, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "", errors.New("name is required")
	}
	upper := strings.ToUpper(trimmed)
	if strings.HasPrefix(upper, teamEnvInjectPrefix) {
		return "", fmt.Errorf("name must not start with the %s prefix", teamEnvInjectPrefix)
	}
	if strings.HasPrefix(upper, hivyReservedEnvPrefix) {
		return "", fmt.Errorf("name must not start with the reserved %s prefix", hivyReservedEnvPrefix)
	}
	if !teamEnvNamePattern.MatchString(upper) {
		return "", errors.New("name must use uppercase letters, numbers, and underscores, and start with a letter or underscore")
	}
	return upper, nil
}
