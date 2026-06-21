package e2e

import "encoding/json"

func agentSessionsEventID(event runtimeSSEEvent) string {
	return eventString(event.Payload, "id")
}

func agentSessionsToolResultText(event runtimeSSEEvent) string {
	if output := agentSessionsToolResultOutput(event); output != "" {
		return output
	}
	raw, err := json.Marshal(event.Payload["result"])
	if err != nil {
		return event.RawData
	}
	return string(raw)
}

func agentSessionsToolResultError(event runtimeSSEEvent) string {
	result, _ := event.Payload["result"].(map[string]any)
	if result == nil {
		return ""
	}
	for _, key := range []string{"isError", "is_error"} {
		if failed, _ := result[key].(bool); failed {
			return agentSessionsToolResultText(event)
		}
	}
	for _, key := range []string{"error", "safe_error"} {
		if errText := eventString(result, key); errText != "" {
			return errText
		}
	}
	return ""
}
