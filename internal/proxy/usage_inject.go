package proxy

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

func EnsureOpenRouterUsage(req *http.Request, endUserID string) error {
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
	rewritten, ok, err := injectUsageAccounting(body, endUserID)
	if err != nil || !ok {
		rewindRequestBody(req, body)
		return err
	}
	replaceRequestBody(req, rewritten)
	return nil
}

func injectUsageAccounting(body []byte, endUserID string) ([]byte, bool, error) {
	payload := map[string]json.RawMessage{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, false, nil
	}

	payload["usage"] = json.RawMessage(`{"include":true}`)

	if bodyRequestsStreaming(payload) {
		payload["stream_options"] = mergeStreamOptions(payload["stream_options"])
	}

	setEndUser(payload, endUserID)

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
	opts["include_usage"] = json.RawMessage(`true`)
	merged, err := json.Marshal(opts)
	if err != nil {
		return json.RawMessage(`{"include_usage":true}`)
	}
	return merged
}
