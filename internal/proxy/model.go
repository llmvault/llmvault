package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/usehivy/hivy/internal/registry"
)

// maxPeekBytes is the maximum number of bytes to read from the request body
// when extracting the model field. This avoids buffering large payloads.
const maxPeekBytes = 2048

// ExtractModel peeks at the request body to extract the "model" field from
// JSON payloads. It only attempts extraction for POST requests with JSON
// content types to paths that typically contain a model field.
// The request body is always left intact for the upstream.
func ExtractModel(req *http.Request) string {
	if req.Method != http.MethodPost || req.Body == nil {
		return ""
	}

	ct := req.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "application/json") {
		return ""
	}

	// Read up to maxPeekBytes from the body
	peek := make([]byte, maxPeekBytes)
	n, err := req.Body.Read(peek)
	if n == 0 {
		return ""
	}
	peek = peek[:n]

	// Reconstruct the body: peeked bytes + remaining
	if err == io.EOF {
		// Entire body fit in peek buffer
		req.Body = io.NopCloser(bytes.NewReader(peek))
	} else {
		req.Body = io.NopCloser(io.MultiReader(bytes.NewReader(peek), req.Body))
	}

	// Try fast JSON extraction of the "model" field
	return extractModelFromJSON(peek)
}

func RewriteModel(req *http.Request, modelID string) error {
	if req.Method != http.MethodPost || req.Body == nil {
		return nil
	}
	ct := req.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "application/json") {
		return nil
	}

	body, err := io.ReadAll(req.Body)
	if err != nil {
		return err
	}
	rewritten, ok, err := rewriteModelJSONBody(body, modelID)
	if err != nil {
		rewindRequestBody(req, body)
		return err
	}
	if !ok {
		rewindRequestBody(req, body)
		return nil
	}
	replaceRequestBody(req, rewritten)
	return nil
}

func RewriteRoutedModel(req *http.Request, providerID string) (string, bool, error) {
	if req.Method != http.MethodPost || req.Body == nil {
		return "", false, nil
	}
	ct := req.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "application/json") {
		return "", false, nil
	}

	body, err := io.ReadAll(req.Body)
	if err != nil {
		return "", false, err
	}
	rewindRequestBody(req, body)

	payload, modelName, ok := parseModelJSONBody(body)
	if !ok {
		return "", false, nil
	}
	route, ok := registry.Global().ResolveModel(providerID, modelName)
	if !ok || route.UpstreamID == modelName {
		return modelName, false, nil
	}

	rewritten, err := rewriteParsedModelJSONBody(payload, route.UpstreamID)
	if err != nil {
		return modelName, false, err
	}
	replaceRequestBody(req, rewritten)
	return modelName, true, nil
}

func parseModelJSONBody(body []byte) (map[string]json.RawMessage, string, bool) {
	payload := map[string]json.RawMessage{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, "", false
	}
	rawModel, ok := payload["model"]
	if !ok {
		return nil, "", false
	}
	var modelName string
	if err := json.Unmarshal(rawModel, &modelName); err != nil {
		return nil, "", false
	}
	return payload, modelName, true
}

func rewriteModelJSONBody(body []byte, modelID string) ([]byte, bool, error) {
	payload, _, ok := parseModelJSONBody(body)
	if !ok {
		return nil, false, nil
	}
	rewritten, err := rewriteParsedModelJSONBody(payload, modelID)
	if err != nil {
		return nil, false, err
	}
	return rewritten, true, nil
}

func rewriteParsedModelJSONBody(payload map[string]json.RawMessage, modelID string) ([]byte, error) {
	rawModel, err := json.Marshal(modelID)
	if err != nil {
		return nil, err
	}
	payload["model"] = rawModel
	return json.Marshal(payload)
}

func rewindRequestBody(req *http.Request, body []byte) {
	req.Body = io.NopCloser(bytes.NewReader(body))
}

func replaceRequestBody(req *http.Request, body []byte) {
	req.Body = io.NopCloser(bytes.NewReader(body))
	req.ContentLength = int64(len(body))
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(body)), nil
	}
	req.Header.Set("Content-Length", fmt.Sprintf("%d", len(body)))
}

// extractModelFromJSON extracts the top-level "model" string field from
// a potentially incomplete JSON payload.
func extractModelFromJSON(data []byte) string {
	// Use json.Decoder for robustness with partial payloads
	dec := json.NewDecoder(bytes.NewReader(data))
	t, err := dec.Token()
	if err != nil {
		return ""
	}
	// Must start with '{'
	if delim, ok := t.(json.Delim); !ok || delim != '{' {
		return ""
	}

	// Scan top-level keys looking for "model"
	for dec.More() {
		t, err = dec.Token()
		if err != nil {
			return ""
		}
		key, ok := t.(string)
		if !ok {
			return ""
		}
		if key == "model" {
			t, err = dec.Token()
			if err != nil {
				return ""
			}
			if s, ok := t.(string); ok {
				return s
			}
			return ""
		}
		// Skip value for non-model keys
		if !skipValue(dec) {
			return ""
		}
	}

	return ""
}

// skipValue skips a single JSON value in the decoder (object, array, or primitive).
func skipValue(dec *json.Decoder) bool {
	t, err := dec.Token()
	if err != nil {
		return false
	}
	if delim, ok := t.(json.Delim); ok {
		switch delim {
		case '{':
			for dec.More() {
				// skip key
				if _, err := dec.Token(); err != nil {
					return false
				}
				// skip value
				if !skipValue(dec) {
					return false
				}
			}
			// consume closing '}'
			if _, err := dec.Token(); err != nil {
				return false
			}
		case '[':
			for dec.More() {
				if !skipValue(dec) {
					return false
				}
			}
			// consume closing ']'
			if _, err := dec.Token(); err != nil {
				return false
			}
		}
	}
	return true
}
