package precontext

import "testing"

func TestMapTimestampFormatsMemoryTimestamp(t *testing.T) {
	out := mapTimestamp(map[string]any{
		"created_at": "2026-06-03T10:15:30+01:00",
	}, "created_at")

	if out != "2026-06-03T09:15:30Z" {
		t.Fatalf("timestamp = %q", out)
	}
}
