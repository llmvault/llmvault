package gateway

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestExternalGatewaySecretGenerateAndVerify(t *testing.T) {
	secret, hash, prefix, err := GenerateExternalGatewaySecret()
	if err != nil {
		t.Fatalf("generate secret: %v", err)
	}
	if secret == "" || hash == "" || prefix == "" {
		t.Fatalf("secret fields should be set: secret=%q hash=%q prefix=%q", secret, hash, prefix)
	}
	if !VerifyExternalGatewaySecret(secret, hash) {
		t.Fatal("generated secret should verify")
	}
	if VerifyExternalGatewaySecret(secret+"wrong", hash) {
		t.Fatal("wrong secret should not verify")
	}
}

func TestExternalAdapterDecodesNormalizedMessage(t *testing.T) {
	adapter := NewExternalAdapter()
	routeID := uuid.New()
	inbound, ok, err := adapter.DecodeInbound(context.Background(), WebhookEnvelope{
		RouteID: routeID,
		Body: []byte(`{
			"message_id":"msg-1",
			"thread_id":"group-1",
			"channel_id":"channel-1",
			"sender":{"id":"user-1","name":"Ada"},
			"text":"Can you check sales?",
			"metadata":{"source_url":"https://example.test/messages/msg-1"}
		}`),
	})
	if err != nil {
		t.Fatalf("decode inbound: %v", err)
	}
	if !ok {
		t.Fatal("message should be accepted")
	}
	if inbound.RouteID != routeID || inbound.DedupeKey != "msg-1" || inbound.ThreadKey != "group-1" {
		t.Fatalf("unexpected routing fields: %#v", inbound)
	}
	if inbound.ChannelID != "channel-1" || inbound.ThreadID != "group-1" || inbound.SenderID != "user-1" || inbound.Text != "Can you check sales?" {
		t.Fatalf("unexpected inbound fields: %#v", inbound)
	}
}
