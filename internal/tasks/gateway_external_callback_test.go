package tasks

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestExternalCallbackSinkPostsSignedFinalResponse(t *testing.T) {
	secret := "hvgw_test_secret"
	var gotSignature string
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSignature = r.Header.Get("X-Hivy-Signature")
		if r.Header.Get("X-Hivy-Gateway-Route-ID") != "route-1" {
			t.Fatalf("route header = %q", r.Header.Get("X-Hivy-Gateway-Route-ID"))
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode callback body: %v", err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	sink := &ExternalCallbackSink{payload: GatewayExternalCallbackPayload{
		RouteID:     "route-1",
		EventID:     "event-1",
		SessionID:   "session-1",
		TraceID:     "trace-1",
		TurnID:      "turn-1",
		Provider:    "whatsapp",
		CallbackURL: server.URL,
		RouteSecret: secret,
		ThreadID:    "thread-1",
		ChannelID:   "channel-1",
	}, client: server.Client()}

	handles, err := sink.SendFinal(context.Background(), GatewayStreamPayload{ChannelID: "channel-1", ThreadID: "thread-1"}, "Hello from Hivy")
	if err != nil {
		t.Fatalf("send final: %v", err)
	}
	if len(handles) != 1 || handles[0].ChannelID != "channel-1" || handles[0].ThreadID != "thread-1" {
		t.Fatalf("handles = %#v", handles)
	}
	if gotBody["text"] != "Hello from Hivy" || gotBody["provider"] != "whatsapp" {
		t.Fatalf("callback body = %#v", gotBody)
	}
	if gotSignature == "" || gotSignature[:7] != "sha256=" {
		t.Fatalf("signature = %q", gotSignature)
	}
}

func TestHmacHexMatchesSHA256HMAC(t *testing.T) {
	body := []byte(`{"text":"hello"}`)
	mac := hmac.New(sha256.New, []byte("secret"))
	mac.Write(body)
	want := hex.EncodeToString(mac.Sum(nil))
	if got := hmacHex("secret", body); got != want {
		t.Fatalf("hmacHex = %q, want %q", got, want)
	}
}
