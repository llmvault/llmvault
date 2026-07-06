package notion

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func TestCheckpoint_MarshalRoundTrip(t *testing.T) {
	edited := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	original := NotionCheckpoint{
		Mode:           modeWorkspace,
		PendingPageIDs: []string{"page-a", "page-b"},
		IndexedPageIDs: []string{"page-x"},
		SearchCursor:   ptr("cursor-1"),
		SearchDone:     true,
		LastSeenEdited: &edited,
	}
	raw, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	got, err := unmarshalCheckpoint(raw)
	if err != nil {
		t.Fatalf("unmarshalCheckpoint: %v", err)
	}
	if !reflect.DeepEqual(got, original) {
		t.Fatalf("round-trip mismatch:\n got = %+v\n want = %+v", got, original)
	}
}

func TestCheckpoint_DummyHasMore(t *testing.T) {
	if cp := dummyCheckpoint(); !cp.HasMore {
		t.Fatalf("dummy checkpoint HasMore = false, want true")
	}
}

func TestCheckpoint_EmptyAndNullProduceDummy(t *testing.T) {
	for _, in := range []string{``, `null`} {
		cp, err := unmarshalCheckpoint(json.RawMessage(in))
		if err != nil {
			t.Fatalf("unmarshalCheckpoint(%q): %v", in, err)
		}
		if !cp.HasMore || cp.Mode != "" {
			t.Fatalf("expected dummy for %q, got %+v", in, cp)
		}
	}
}

func TestCheckpoint_MalformedErrors(t *testing.T) {
	if _, err := unmarshalCheckpoint(json.RawMessage(`{not json`)); err == nil {
		t.Fatal("expected error on malformed checkpoint JSON")
	}
}
