package correlation

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func TestProvisioningCorrelationRoundTripsAcrossHeadersAndLabels(t *testing.T) {
	t.Parallel()
	sessionID := uuid.New()
	values := NewProvisioning(sessionID)
	values.OrgID = uuid.NewString()
	values.AgentID = uuid.NewString()

	ctx := WithValues(context.Background(), values)
	headers := http.Header{}
	InjectHeaders(ctx, headers)
	fromHeaders := FromHeaders(headers)
	if fromHeaders.SessionID != sessionID.String() ||
		fromHeaders.ProvisioningAttemptID != values.ProvisioningAttemptID ||
		fromHeaders.TraceID != values.TraceID {
		t.Fatalf("header correlation mismatch: %#v", fromHeaders)
	}

	labels := map[string]string{}
	ApplyLabels(labels, values)
	fromLabels := FromLabels(labels)
	if fromLabels != values {
		t.Fatalf("label correlation mismatch: got %#v want %#v", fromLabels, values)
	}
}
