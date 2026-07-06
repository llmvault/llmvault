package linear

import (
	"encoding/json"
	"fmt"

	"github.com/usehivy/hivy/internal/rag/connectors/interfaces"
)

type Stage string

const (
	StageStart    Stage = "START"
	StageProjects Stage = "PROJECTS"
	StageIssues   Stage = "ISSUES"
	StageDone     Stage = "DONE"
)

func (s Stage) IsValid() bool {
	switch s {
	case StageStart, StageProjects, StageIssues, StageDone:
		return true
	default:
		return false
	}
}

// LinearCheckpoint carries resumable state across runs. The run is staged:
// PROJECTS walks each selected team's projects (emitting project + collected
// initiative docs) then ISSUES does one combined cursor walk over issues
// filtered by all team IDs. It embeds interfaces.AnyCheckpoint to satisfy the
// Checkpoint marker — the unexported isCheckpoint() method can only be
// implemented by types in the interfaces package, so embedding is the
// cross-package conformance pattern. Completion is signalled via
// Stage = DONE.
type LinearCheckpoint struct {
	interfaces.AnyCheckpoint

	Stage          Stage    `json:"stage"`
	PendingTeamIDs []string `json:"pending_team_ids,omitempty"`
	ProjectsCursor string   `json:"projects_cursor,omitempty"`
	IssuesCursor   string   `json:"issues_cursor,omitempty"`
}

func dummyCheckpoint() LinearCheckpoint {
	return LinearCheckpoint{Stage: StageStart}
}

// unmarshalCheckpoint parses persisted bytes back into a checkpoint.
// Empty/nil/"null"/"{}" raw yields a fresh dummy checkpoint. Malformed JSON
// returns an error so the scheduler restarts the run from a fresh checkpoint
// rather than resuming into a corrupt state.
func unmarshalCheckpoint(raw json.RawMessage) (LinearCheckpoint, error) {
	trimmed := string(raw)
	if len(raw) == 0 || trimmed == "null" || trimmed == "{}" {
		return dummyCheckpoint(), nil
	}
	var cp LinearCheckpoint
	if err := json.Unmarshal(raw, &cp); err != nil {
		return LinearCheckpoint{}, fmt.Errorf("linear: parse checkpoint: %w", err)
	}
	if cp.Stage == "" {
		cp.Stage = StageStart
	}
	return cp, nil
}
