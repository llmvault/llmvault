package evals

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestJudgeClassifyFinalTextUsesProxyAndJSONSchema(t *testing.T) {
	var gotAuth string
	var gotModel string
	var gotFormat map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/proxy/v1/chat/completions" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		gotModel, _ = body["model"].(string)
		gotFormat, _ = body["response_format"].(map[string]any)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]any{
					"content": `{"behavior":"clarify","confidence":0.98,"reason":"asks for missing context"}`,
				},
			}},
		})
	}))
	defer srv.Close()

	judgement, err := NewJudge("deepseek-v4-flash").ClassifyFinalText(t.Context(), srv.URL+"/v1/proxy/v1", "ptok_test", Case{
		Message:          "Can you help with the thing?",
		ExpectedBehavior: BehaviorClarify,
	}, "Can you remind me what the thing is?")
	if err != nil {
		t.Fatalf("ClassifyFinalText: %v", err)
	}
	if judgement.Behavior != BehaviorClarify || judgement.Model != "deepseek-v4-flash" {
		t.Fatalf("judgement = %#v", judgement)
	}
	if gotAuth != "Bearer ptok_test" {
		t.Fatalf("authorization = %q", gotAuth)
	}
	if gotModel != "deepseek-v4-flash" {
		t.Fatalf("model = %q", gotModel)
	}
	if gotFormat["type"] != "json_schema" {
		t.Fatalf("response_format = %#v", gotFormat)
	}
}

func TestJudgeGenerateFollowUpRetriesMalformedJSON(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/proxy/v1/chat/completions" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		_ = atomic.AddInt32(&calls, 1)
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if body["model"] != "deepseek-v4-flash" {
			t.Fatalf("model = %#v", body["model"])
		}
		content := `{"reply":`
		if atomic.LoadInt32(&calls) == 3 {
			content = `{"reply":"Please build the corporate catering page with the premium wedding style."}`
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]any{"content": content},
			}},
		})
	}))
	defer srv.Close()

	reply, err := NewJudge("deepseek-v4-flash").GenerateFollowUp(t.Context(), srv.URL+"/v1/proxy/v1", "ptok_test", Case{
		Message: "Can you build the page?",
		FollowUp: &FollowUpFixture{
			Mode:    "generated",
			Context: "The page is for corporate catering and should use the premium wedding style.",
		},
	}, "What kind of page should I build?")
	if err != nil {
		t.Fatalf("GenerateFollowUp: %v", err)
	}
	if reply != "Please build the corporate catering page with the premium wedding style." {
		t.Fatalf("reply = %q", reply)
	}
	if calls != 3 {
		t.Fatalf("calls = %d, want 3", calls)
	}
}

func TestJudgeGenerateFollowUpStaticDoesNotCallProxy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("static follow-up should not call proxy")
	}))
	defer srv.Close()

	reply, err := NewJudge("deepseek-v4-flash").GenerateFollowUp(t.Context(), srv.URL+"/v1/proxy/v1", "ptok_test", Case{
		FollowUp: &FollowUpFixture{
			Mode:    "static",
			Context: "Use the catering details from the brief.",
		},
	}, "What details should I use?")
	if err != nil {
		t.Fatalf("GenerateFollowUp: %v", err)
	}
	if reply != "Use the catering details from the brief." {
		t.Fatalf("reply = %q", reply)
	}
}
