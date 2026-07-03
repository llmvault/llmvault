package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/usehivy/hivy/internal/sheets"
)

// eventDeadline bounds each positive realtime assertion. Generous but bounded.
const eventDeadline = 15 * time.Second

// appsRealtimeAssertPillars drives the seven realtime pillars against the
// deployed app's SSE stream.
func appsRealtimeAssertPillars(t *testing.T, ctx context.Context, apiBase, appBase, token, orgID, channelID, cookie, runID string, bound appsRealtimeSheet) {
	t.Helper()
	nameF, roleF := bound.FieldIDs["Name"], bound.FieldIDs["Role"]

	// Pillar (a): the initial frame on subscribe is a refresh.
	clientA := appsRealtimeDialLive(t, ctx, appBase, cookie, "A")
	if first := clientA.next(t, eventDeadline, "initial refresh frame"); first.Event != "refresh" {
		t.Fatalf("first SSE frame event=%q data=%q want refresh", first.Event, first.Data)
	}
	t.Log("pillar a: subscribe delivered an `event: refresh` frame first")

	// Warm-up: guarantee the app's upstream Redis subscription is live before
	// the asserted writes, so no event can be missed to a connect race.
	appsRealtimeWarmUp(t, ctx, apiBase, token, orgID, bound, clientA)

	// Pillar (b): INSERT 2 rows → rows_changed(insert) with IDs + snapshots.
	writeAt := time.Now()
	insertIDs := appsRealtimeInsertRows(t, ctx, apiBase, token, orgID, bound, []map[string]any{
		{nameF: "Ada Lovelace", roleF: "Engineer", bound.FieldIDs["Email"]: "ada@example.com"},
		{nameF: "Grace Hopper", roleF: "Admiral", bound.FieldIDs["Email"]: "grace@example.com"},
	})
	id1, id2 := insertIDs[0], insertIDs[1]
	insertEv := clientA.waitRowsChanged(t, eventDeadline, "insert of 2 rows", "", func(ev sheets.Event) bool {
		return ev.Action == "insert" && eventHasRow(ev, id1) && eventHasRow(ev, id2)
	})
	latency := time.Since(writeAt)
	if insertEv.PageID != bound.PageID {
		t.Fatalf("insert event page_id=%q want %q", insertEv.PageID, bound.PageID)
	}
	appsRealtimeAssertSnapshot(t, insertEv, id1, map[string]string{nameF: "Ada Lovelace", roleF: "Engineer"})
	appsRealtimeAssertSnapshot(t, insertEv, id2, map[string]string{nameF: "Grace Hopper", roleF: "Admiral"})
	t.Logf("pillar b: insert event carried both IDs + full snapshots (write→event latency %s)", latency.Round(time.Millisecond))

	// Pillar (c): UPDATE row1 → rows_changed(update) with patch + snapshot.
	appsRealtimeUpdateRow(t, ctx, apiBase, token, orgID, bound, id1, map[string]any{roleF: "Staff Engineer"})
	updateEv := clientA.waitRowsChanged(t, eventDeadline, "update of row1", "", func(ev sheets.Event) bool {
		return ev.Action == "update" && eventHasRow(ev, id1)
	})
	if patch, ok := updateEv.Patches[id1]; !ok || patch[roleF] != "Staff Engineer" {
		t.Fatalf("update event patch for %s = %+v want %s=Staff Engineer", id1, updateEv.Patches[id1], roleF)
	}
	appsRealtimeAssertSnapshot(t, updateEv, id1, map[string]string{roleF: "Staff Engineer"})
	t.Log("pillar c: update event carried the patch and a refreshed snapshot")

	// Pillar (d): DELETE row2 → rows_changed(delete), ID only, no snapshots.
	appsRealtimeDeleteRow(t, ctx, apiBase, token, orgID, bound, id2)
	deleteEv := clientA.waitRowsChanged(t, eventDeadline, "delete of row2", "", func(ev sheets.Event) bool {
		return ev.Action == "delete" && eventHasRow(ev, id2)
	})
	if len(deleteEv.Rows) != 0 {
		t.Fatalf("delete event carried %d snapshots want 0: %+v", len(deleteEv.Rows), deleteEv.Rows)
	}
	t.Log("pillar d: delete event carried the ID and no snapshots")

	// Pillar (e): cross-sheet isolation — a SECOND sheet's writes must never
	// reach this app's stream. We write the other sheet first, then a positive
	// control on the bound sheet; observing the control (with the forbid guard
	// armed) proves the other sheet's event never leaked.
	other := appsRealtimeCreateSheet(t, ctx, apiBase, token, orgID, channelID, "Other "+runID, []string{"Name"})
	appsRealtimeInsertRows(t, ctx, apiBase, token, orgID, other, []map[string]any{{other.FieldIDs["Name"]: "must-not-appear"}})
	controlIDs := appsRealtimeInsertRows(t, ctx, apiBase, token, orgID, bound, []map[string]any{{nameF: "Control Row"}})
	clientA.waitRowsChanged(t, eventDeadline, "isolation positive control", other.SheetID, func(ev sheets.Event) bool {
		return ev.Action == "insert" && eventHasRow(ev, controlIDs[0])
	})
	t.Log("pillar e: bound-sheet control arrived; no unbound-sheet event ever leaked to the stream")

	// Pillar (g): a second concurrent SSE client also receives a later write.
	clientB := appsRealtimeDialLive(t, ctx, appBase, cookie, "B")
	if first := clientB.next(t, eventDeadline, "client B initial refresh"); first.Event != "refresh" {
		t.Fatalf("client B first frame event=%q want refresh", first.Event)
	}
	fanIDs := appsRealtimeInsertRows(t, ctx, apiBase, token, orgID, bound, []map[string]any{{nameF: "Fan Out"}})
	matchFan := func(ev sheets.Event) bool { return ev.Action == "insert" && eventHasRow(ev, fanIDs[0]) }
	clientA.waitRowsChanged(t, eventDeadline, "fan-out on client A", "", matchFan)
	clientB.waitRowsChanged(t, eventDeadline, "fan-out on client B", "", matchFan)
	t.Log("pillar g: both concurrent SSE clients received the same write's event")

	// Pillar (f): the app's own refetch path agrees with the events.
	final := appsRealtimeQueryAppRows(t, ctx, appBase, cookie, bound.PageID)
	if data, ok := final[id1]; !ok || data[roleF] != "Staff Engineer" {
		t.Fatalf("refetch: row1 %s = %+v want role Staff Engineer", id1, data)
	}
	if _, ok := final[id2]; ok {
		t.Fatalf("refetch: deleted row2 %s still present", id2)
	}
	if _, ok := final[controlIDs[0]]; !ok {
		t.Fatalf("refetch: control row %s missing", controlIDs[0])
	}
	if _, ok := final[fanIDs[0]]; !ok {
		t.Fatalf("refetch: fan-out row %s missing", fanIDs[0])
	}
	t.Log("pillar f: the app's rows/query refetch matches the event-described final state")
}

// appsRealtimeAssertSnapshot checks a row snapshot in a rows_changed event has
// the expected cell values and non-zero timestamps.
func appsRealtimeAssertSnapshot(t *testing.T, ev sheets.Event, rowID string, want map[string]string) {
	t.Helper()
	snap, ok := eventSnapshot(ev, rowID)
	if !ok {
		t.Fatalf("event has no snapshot for row %s: %+v", rowID, ev.Rows)
	}
	for field, value := range want {
		if got, _ := snap.Data[field].(string); got != value {
			t.Fatalf("snapshot %s field %s = %q want %q", rowID, field, got, value)
		}
	}
	if snap.CreatedAt.IsZero() || snap.UpdatedAt.IsZero() {
		t.Fatalf("snapshot %s missing timestamps: created=%v updated=%v", rowID, snap.CreatedAt, snap.UpdatedAt)
	}
}

// appsRealtimeWarmUp inserts throwaway probe rows until one is observed on the
// browser stream, proving the app's single upstream Redis subscription is live.
// It then deletes each probe and drains the residual events, so the asserted
// writes start from a clean stream. Fails if the pipe never becomes ready.
func appsRealtimeWarmUp(t *testing.T, ctx context.Context, apiBase, token, orgID string, bound appsRealtimeSheet, client *appsRealtimeStream) {
	t.Helper()
	nameF := bound.FieldIDs["Name"]
	deadline := time.Now().Add(30 * time.Second)
	for attempt := 0; time.Now().Before(deadline); attempt++ {
		ids := appsRealtimeInsertRows(t, ctx, apiBase, token, orgID, bound, []map[string]any{{nameF: "warmup-probe"}})
		probe := ids[0]
		_, ok := client.tryRowsChanged(3*time.Second, func(ev sheets.Event) bool {
			return ev.Action == "insert" && eventHasRow(ev, probe)
		})
		appsRealtimeDeleteRow(t, ctx, apiBase, token, orgID, bound, probe)
		if ok {
			appsRealtimeDrain(client, 750*time.Millisecond)
			t.Logf("warm-up: live pipe ready after %d probe(s)", attempt+1)
			return
		}
	}
	t.Fatal("app live pipe never delivered a warm-up event — the upstream /v1/live relay never connected")
}

// appsRealtimeDrain discards buffered frames until the stream is quiet for the
// given window.
func appsRealtimeDrain(client *appsRealtimeStream, quiet time.Duration) {
	for {
		select {
		case <-client.frames:
		case <-time.After(quiet):
			return
		}
	}
}
