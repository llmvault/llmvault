package proxy

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

func EnsureOpenRouterUsage(req *http.Request, endUserID string) error {
	return ensureUsageAccounting(req, true, endUserID)
}

// EnsureOpenAICompatibleUsage requests the final usage summary event from
// OpenAI-compatible providers. Unlike OpenRouter, direct providers do not
// accept OpenRouter's top-level usage/include extension.
func EnsureOpenAICompatibleUsage(req *http.Request) error {
	return ensureUsageAccounting(req, false, "")
}

func ensureUsageAccounting(req *http.Request, openRouter bool, endUserID string) error {
	if req.Method != http.MethodPost || req.Body == nil {
		return nil
	}
	ct := req.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "application/json") {
		return nil
	}
	if !strings.Contains(req.URL.Path, "/chat/completions") && !strings.HasSuffix(req.URL.Path, "/completions") {
		return nil
	}

	body, err := io.ReadAll(req.Body)
	if err != nil {
		return err
	}
	rewritten, ok, err := injectUsageAccounting(body, openRouter, endUserID)
	if err != nil || !ok {
		rewindRequestBody(req, body)
		return err
	}
	replaceRequestBody(req, rewritten)
	return nil
}

func injectUsageAccounting(body []byte, openRouter bool, endUserID string) ([]byte, bool, error) {
	payload := map[string]json.RawMessage{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, false, nil
	}

	if openRouter {
		payload["usage"] = json.RawMessage(`{"include":true}`)
		setEndUser(payload, endUserID)
	}

	if bodyRequestsStreaming(payload) {
		payload["stream_options"] = mergeStreamOptions(payload["stream_options"])
	}

	rewritten, err := json.Marshal(payload)
	if err != nil {
		return nil, false, err
	}
	return rewritten, true, nil
}

func setEndUser(payload map[string]json.RawMessage, endUserID string) {
	if endUserID == "" {
		return
	}
	if _, exists := payload["user"]; exists {
		return
	}
	encoded, err := json.Marshal(endUserID)
	if err != nil {
		return
	}
	payload["user"] = encoded
}

func bodyRequestsStreaming(payload map[string]json.RawMessage) bool {
	raw, ok := payload["stream"]
	if !ok {
		return false
	}
	var stream bool
	if err := json.Unmarshal(raw, &stream); err != nil {
		return false
	}
	return stream
}

func mergeStreamOptions(existing json.RawMessage) json.RawMessage {
	opts := map[string]json.RawMessage{}
	if len(existing) > 0 {
		_ = json.Unmarshal(existing, &opts)
	}
	if opts == nil {
		opts = map[string]json.RawMessage{}
	}
	opts["include_usage"] = json.RawMessage(`true`)
	merged, err := json.Marshal(opts)
	if err != nil {
		return json.RawMessage(`{"include_usage":true}`)
	}
	return merged
}
