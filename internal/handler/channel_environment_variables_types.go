package handler

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// channelEnvInjectPrefix is the sentinel applied to user-supplied env vars when
// they are injected into the runtime env. The sandbox runtime strips it so the
// workload sees the clean name. User-chosen names must not include it.
const channelEnvInjectPrefix = "__ENV__"

// hivyReservedEnvPrefix is reserved for platform control-plane vars. User names
// are rejected so a (stripped) user var can never shadow a control-plane key.
const hivyReservedEnvPrefix = "HIVY_"

var channelEnvNamePattern = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)

type channelEnvironmentVariableResponse struct {
	Name string `json:"name"`
}

type channelEnvironmentVariablesResponse struct {
	Data []channelEnvironmentVariableResponse `json:"data"`
}

type createChannelEnvironmentVariableRequest struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type updateChannelEnvironmentVariableRequest struct {
	Name  *string `json:"name,omitempty"`
	Value *string `json:"value,omitempty"`
}

// normalizeChannelEnvName uppercases and validates a user-supplied env var name.
func normalizeChannelEnvName(name string) (string, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "", errors.New("name is required")
	}
	upper := strings.ToUpper(trimmed)
	if strings.HasPrefix(upper, channelEnvInjectPrefix) {
		return "", fmt.Errorf("name must not start with the %s prefix", channelEnvInjectPrefix)
	}
	if strings.HasPrefix(upper, hivyReservedEnvPrefix) {
		return "", fmt.Errorf("name must not start with the reserved %s prefix", hivyReservedEnvPrefix)
	}
	if !channelEnvNamePattern.MatchString(upper) {
		return "", errors.New("name must use uppercase letters, numbers, and underscores, and start with a letter or underscore")
	}
	return upper, nil
}
