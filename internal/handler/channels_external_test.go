package handler_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/handler"
)

type fakeChannelProvisioner struct {
	calls []handler.ChannelExternalProvisionRequest
	err   error
}

func (f *fakeChannelProvisioner) EnsureExternalChannel(_ context.Context, _ uuid.UUID, req handler.ChannelExternalProvisionRequest) error {
	f.calls = append(f.calls, req)
	return f.err
}

func TestIntegration_ChannelsCreate_ExternalSlackResource(t *testing.T) {
	provisioner := &fakeChannelProvisioner{}
	h := newChannelHarness(t, handler.WithChannelExternalProvisioner(provisioner))
	fx := h.seed(t)
	conn := seedChannelConnection(t, h, fx, "slack", "slack-workspace-2")

	rr := h.doJSON(t, http.MethodPost, "/v1/channels", fx, fx.owner, map[string]any{
		"external_provider":      "slack",
		"external_connection_id": conn.ID.String(),
		"external_resource_key":  "CENG",
		"external_resource_name": "engineering",
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", rr.Code, rr.Body.String())
	}
	out := decodeChannelCreate(t, rr)
	if out.Channel.Name != "slack-engineering" {
		t.Fatalf("channel name=%q, want slack-engineering", out.Channel.Name)
	}
	if out.Channel.ExternalWorkspaceKey != conn.NangoConnectionID {
		t.Fatalf("workspace key=%q, want connection nango id", out.Channel.ExternalWorkspaceKey)
	}
	if out.Channel.ExternalResourceKey != "CENG" {
		t.Fatalf("external resource key=%q, want CENG", out.Channel.ExternalResourceKey)
	}
	if len(provisioner.calls) != 1 {
		t.Fatalf("provisioner calls=%d, want 1", len(provisioner.calls))
	}
	call := provisioner.calls[0]
	if call.Provider != "slack" || call.ConnectionID != conn.ID || call.ResourceKey != "CENG" {
		t.Fatalf("provisioner call mismatch: %+v", call)
	}
}
