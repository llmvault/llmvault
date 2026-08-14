package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/usehivy/hivy/internal/model"
	llm "github.com/usehivy/hivy/internal/trigger/hivy"
)

type recordingCredentialTestClient struct {
	request llm.CompletionRequest
}

func (client *recordingCredentialTestClient) ChatCompletion(_ context.Context, req llm.CompletionRequest) (*llm.CompletionResponse, error) {
	client.request = req
	return &llm.CompletionResponse{Message: llm.Message{Role: "assistant", Content: "OK"}}, nil
}

func TestCredentialHandlerTestSystemRunsInferenceWithoutSaving(t *testing.T) {
	const testAPIKey = "test-deepseek-key" // #nosec G101 -- fake test fixture, not a credential
	handler := NewCredentialHandler(nil, nil, nil, nil)
	client := &recordingCredentialTestClient{}
	var receivedCredential *model.Credential
	var receivedAPIKey string
	handler.testClient = func(credential *model.Credential, apiKey string) llm.CompletionClient {
		copy := *credential
		receivedCredential = &copy
		receivedAPIKey = apiKey
		return client
	}

	body, err := json.Marshal(testSystemCredentialRequest{ProviderID: "deepseek", APIKey: testAPIKey})
	if err != nil {
		t.Fatalf("encode request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/admin/system-credentials/test", bytes.NewReader(body))
	recorder := httptest.NewRecorder()
	handler.TestSystem(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if receivedCredential == nil || receivedCredential.ProviderID != "deepseek" || receivedCredential.BaseURL != "https://api.deepseek.com" {
		t.Fatalf("credential = %#v", receivedCredential)
	}
	if receivedAPIKey != testAPIKey {
		t.Fatalf("API key = %q", receivedAPIKey)
	}
	if client.request.Model != "deepseek-v4-flash" || len(client.request.Messages) != 1 {
		t.Fatalf("completion request = %#v", client.request)
	}

	var response testSystemCredentialResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Status != "ok" || response.ModelID != "deepseek-v4-flash" {
		t.Fatalf("response = %#v", response)
	}
}
