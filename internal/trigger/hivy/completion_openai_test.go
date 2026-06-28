package hivy

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	openai "github.com/sashabaranov/go-openai"
)

type captureDoer struct {
	req *http.Request
}

func (d *captureDoer) Do(req *http.Request) (*http.Response, error) {
	d.req = req
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("")),
	}, nil
}

func TestOpenRouterHeaderDoerUsesHivyAppHeaders(t *testing.T) {
	inner := &captureDoer{}
	doer := openRouterHeaderDoer{inner: inner}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "https://openrouter.ai/api/v1/chat/completions", nil)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := doer.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if inner.req == nil {
		t.Fatal("inner doer was not called")
	}
	if got := inner.req.Header.Get("HTTP-Referer"); got != "https://usehivy.com" {
		t.Fatalf("HTTP-Referer = %q, want https://usehivy.com", got)
	}
	if got := inner.req.Header.Get("X-Title"); got != "Hivy" {
		t.Fatalf("X-Title = %q, want Hivy", got)
	}
}

type captureCompletionDoer struct {
	body []byte
}

func (d *captureCompletionDoer) Do(req *http.Request) (*http.Response, error) {
	if req.Body != nil {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		d.body = body
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Body: io.NopCloser(strings.NewReader(`{
			"id": "chatcmpl-test",
			"object": "chat.completion",
			"created": 0,
			"model": "gpt-4o-mini",
			"choices": [{
				"index": 0,
				"message": {"role": "assistant", "content": "{\"memories\":[]}"},
				"finish_reason": "stop"
			}]
		}`)),
	}, nil
}

func TestOpenAICompletionClientSendsJSONSchemaResponseFormat(t *testing.T) {
	doer := &captureCompletionDoer{}
	cfg := openai.DefaultConfig("sk-test")
	cfg.BaseURL = "https://api.openai.test/v1"
	cfg.HTTPClient = doer
	client := &OpenAICompletionClient{client: openai.NewClientWithConfig(cfg)}

	_, err := client.ChatCompletion(context.Background(), CompletionRequest{
		Model: "gpt-4o-mini",
		Messages: []Message{
			{Role: "user", Content: "remember this"},
		},
		ResponseFormat: &ResponseFormat{
			Type: ResponseFormatJSONSchema,
			JSONSchema: &ResponseJSONSchema{
				Name:   "session_reflection",
				Schema: json.RawMessage(`{"type":"object","additionalProperties":false}`),
				Strict: true,
			},
		},
	})
	if err != nil {
		t.Fatalf("chat completion: %v", err)
	}

	var body map[string]any
	if err := json.Unmarshal(doer.body, &body); err != nil {
		t.Fatalf("request body json: %v", err)
	}
	format, ok := body["response_format"].(map[string]any)
	if !ok {
		t.Fatalf("missing response_format in request body: %s", string(doer.body))
	}
	if format["type"] != ResponseFormatJSONSchema {
		t.Fatalf("response_format.type=%#v", format["type"])
	}
	schema, ok := format["json_schema"].(map[string]any)
	if !ok {
		t.Fatalf("missing json_schema: %#v", format)
	}
	if schema["name"] != "session_reflection" || schema["strict"] != true {
		t.Fatalf("json_schema=%#v", schema)
	}
	rawSchema, ok := schema["schema"].(map[string]any)
	if !ok || rawSchema["additionalProperties"] != false {
		t.Fatalf("schema=%#v", schema["schema"])
	}
}
