package gateway

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

// Connection-path events with route_id IS NULL must still dedupe on
// (org_id, dedupe_key). Postgres treats NULL as distinct, so without the partial
// unique index ON CONFLICT never fires and a redelivery double-inserts.
func TestInsertInboundEventNullRouteDedupe(t *testing.T) {
	db := connectGatewayTestDB(t)
	route := seedGatewayRoute(t, db)

	// Connection-path route: no persisted route row, so route_id resolves to NULL.
	nilRoute := model.EmployeeGatewayRoute{
		ID:         uuid.Nil,
		OrgID:      route.OrgID,
		EmployeeID: route.EmployeeID,
		Provider:   route.Provider,
	}
	service := NewService(db, &flakyRuntime{}, nil, NewFakeSlackAdapter())

	inbound := InboundEnvelope{
		Provider:          nilRoute.Provider,
		DedupeKey:         "null-route-dedupe-key",
		ExternalMessageID: "msg-1",
		ChannelID:         "C1",
		ThreadID:          "T1",
		SenderID:          "U1",
		Raw:               map[string]any{"hello": "world"},
		ReceivedAt:        time.Now().UTC(),
	}

	first, dupFirst, err := service.insertInboundEvent(t.Context(), nilRoute, inbound)
	if err != nil {
		t.Fatalf("first insert: %v", err)
	}
	if dupFirst {
		t.Fatalf("first insert must not be a duplicate")
	}

	second, dupSecond, err := service.insertInboundEvent(t.Context(), nilRoute, inbound)
	if err != nil {
		t.Fatalf("second insert (redelivery): %v", err)
	}
	if !dupSecond {
		t.Fatalf("redelivery of a NULL-route event with the same dedupe key must be deduped")
	}
	if second.ID != first.ID {
		t.Fatalf("redelivery must resolve to the original row: got %s, want %s", second.ID, first.ID)
	}

	var rows int64
	db.Model(&model.EmployeeGatewayEvent{}).
		Where("route_id IS NULL AND org_id = ? AND dedupe_key = ?", nilRoute.OrgID, inbound.DedupeKey).
		Count(&rows)
	if rows != 1 {
		t.Fatalf("expected exactly one NULL-route inbound event row, got %d", rows)
	}
}
