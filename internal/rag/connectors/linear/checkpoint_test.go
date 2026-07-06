package linear

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/usehivy/hivy/internal/rag/connectors/interfaces"
)

func TestCheckpoint_RoundTrip(t *testing.T) {
	orig := LinearCheckpoint{
		AnyCheckpoint:  interfaces.AnyCheckpoint{HasMore: true},
		Stage:          StageProjects,
		PendingTeamIDs: []string{"team-a", "team-b"},
		ProjectsCursor: "proj-cursor",
		IssuesCursor:   "issue-cursor",
	}
	b, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got, err := unmarshalCheckpoint(b)
	if err != nil {
		t.Fatalf("unmarshalCheckpoint: %v", err)
	}
	if !reflect.DeepEqual(got, orig) {
		t.Fatalf("round-trip = %+v, want %+v", got, orig)
	}
}

func TestUnmarshalCheckpoint_EmptyVariants(t *testing.T) {
	want := dummyCheckpoint()
	for _, in := range []string{``, `null`, `{}`} {
		got, err := unmarshalCheckpoint(json.RawMessage(in))
		if err != nil {
			t.Fatalf("unmarshalCheckpoint(%q): %v", in, err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("unmarshalCheckpoint(%q) = %+v, want dummy %+v", in, got, want)
		}
		if got.Stage != StageStart {
			t.Fatalf("unmarshalCheckpoint(%q).Stage = %q, want START", in, got.Stage)
		}
	}
}

func TestUnmarshalCheckpoint_Garbage(t *testing.T) {
	if _, err := unmarshalCheckpoint(json.RawMessage(`{not json`)); err == nil {
		t.Fatal("expected error for garbage JSON, got nil")
	}
}

func TestStage_IsValid(t *testing.T) {
	valid := []Stage{StageStart, StageProjects, StageIssues, StageDone}
	for _, s := range valid {
		if !s.IsValid() {
			t.Fatalf("Stage %q should be valid", s)
		}
	}
	for _, s := range []Stage{"", "BOGUS", "start", "projects"} {
		if s.IsValid() {
			t.Fatalf("Stage %q should be invalid", s)
		}
	}
}
