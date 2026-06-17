package handler

import (
	"fmt"
	"regexp"
	"strings"
)

var agentRuntimeVersionPattern = regexp.MustCompile(`(?:^|[^A-Za-z0-9])v([0-9]+)[.-]([0-9]+)[.-]([0-9]+)(?:[^0-9]|$)`)

func agentRuntimeVersionLabelFromPtr(ref *string) string {
	if ref == nil {
		return ""
	}
	return agentRuntimeVersionLabel(*ref)
}

func agentRuntimeVersionLabel(ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}
	match := agentRuntimeVersionPattern.FindStringSubmatch(ref)
	if len(match) != 4 {
		return ""
	}
	return fmt.Sprintf("v%s.%s.%s", match[1], match[2], match[3])
}
