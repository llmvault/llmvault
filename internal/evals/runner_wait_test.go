package evals

import (
	"testing"
	"time"

	"github.com/usehivy/hivy/internal/model"
)

func TestEvalCheckpointLoggingOnlyRepeatsAfterInterval(t *testing.T) {
	now := time.Now()
	if !shouldLogEvalCheckpoint("a", "", time.Time{}) {
		t.Fatal("first checkpoint should log")
	}
	if !shouldLogEvalCheckpoint("b", "a", now) {
		t.Fatal("changed checkpoint should log")
	}
	if shouldLogEvalCheckpoint("a", "a", now) {
		t.Fatal("unchanged fresh checkpoint should not log")
	}
	if !shouldLogEvalCheckpoint("a", "a", now.Add(-evalCheckpointInterval-time.Second)) {
		t.Fatal("unchanged stale checkpoint should log")
	}
}

func TestGradeCheckpointSignatureIncludesObservedActivity(t *testing.T) {
	base := gradeCheckpointSignature(Evidence{}, "pending", Decision{Behavior: "pending"})
	withEvent := gradeCheckpointSignature(Evidence{Events: make([]model.EmployeeSessionEvent, 1)}, "pending", Decision{Behavior: "pending"})
	if base == withEvent {
		t.Fatal("signature should change when observed event count changes")
	}
}
