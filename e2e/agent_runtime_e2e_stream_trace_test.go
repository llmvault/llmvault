package e2e

import (
	"fmt"
	"strings"
)

func traceRuntimeSSEEvent(trace *agentRuntimeE2ETrace, label string, index int, event runtimeSSEEvent) {
	tool, _ := event.Payload["tool"].(string)
	agent, _ := event.Payload["agent_name"].(string)
	scope, _ := event.Payload["scope"].(string)
	if agent == "" {
		if subagent := payloadMap(event.Payload["subagent"]); subagent != nil {
			agent, _ = subagent["agent_name"].(string)
		}
	}
	trace.Logf("sse", "%s event #%d name=%s scope=%s tool=%s agent=%s", label, index, event.Name, scope, tool, agent)
	if event.RawData != "" {
		trace.Body("sse", fmt.Sprintf("%s event #%d %s payload", label, index, event.Name), []byte(event.RawData))
	}
}

func summarizeEvents(events []runtimeSSEEvent) string {
	parts := make([]string, 0, len(events))
	for _, event := range events {
		if event.Name == "tool_call" {
			parts = append(parts, fmt.Sprintf("%s:%v", event.Name, event.Payload["tool"]))
		} else {
			parts = append(parts, event.Name)
		}
	}
	if len(parts) > 80 {
		parts = parts[len(parts)-80:]
	}
	return strings.Join(parts, ",")
}
