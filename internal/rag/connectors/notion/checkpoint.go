package notion

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/usehivy/hivy/internal/rag/connectors/interfaces"
)

const (
	modeSubtree   = "SUBTREE"
	modeWorkspace = "WORKSPACE"
)

// NotionCheckpoint carries resumable state across runs. The page frontier
// (PendingPageIDs) and dedup set (IndexedPageIDs) let a run pick up mid
// traversal; SearchCursor/SearchDone track workspace-mode enumeration.
// It embeds interfaces.AnyCheckpoint to satisfy the Checkpoint marker;
// HasMore is set false once a run completes.
type NotionCheckpoint struct {
	interfaces.AnyCheckpoint

	// Mode is "" until the first run classifies the source as SUBTREE
	// or WORKSPACE from its config.
	Mode string `json:"mode,omitempty"`

	// PendingPageIDs is the BFS frontier of pages still to index.
	PendingPageIDs []string `json:"pending_page_ids,omitempty"`

	// IndexedPageIDs is the serialised form of the in-run dedup set.
	IndexedPageIDs []string `json:"indexed_page_ids,omitempty"`

	// SearchCursor is the next-page cursor for workspace enumeration.
	SearchCursor *string `json:"search_cursor,omitempty"`

	// SearchDone marks that workspace enumeration has been exhausted.
	SearchDone bool `json:"search_done,omitempty"`

	// LastSeenEdited is the newest last-edited time observed, retained
	// for diagnostics on incremental runs.
	LastSeenEdited *time.Time `json:"last_seen_edited,omitempty"`
}

func dummyCheckpoint() NotionCheckpoint {
	return NotionCheckpoint{AnyCheckpoint: interfaces.AnyCheckpoint{HasMore: true}}
}

// unmarshalCheckpoint parses persisted bytes back into a checkpoint.
// Malformed JSON returns an error so the scheduler restarts the run from
// a fresh checkpoint rather than resuming into a corrupt state.
func unmarshalCheckpoint(raw json.RawMessage) (NotionCheckpoint, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return dummyCheckpoint(), nil
	}
	var cp NotionCheckpoint
	if err := json.Unmarshal(raw, &cp); err != nil {
		return NotionCheckpoint{}, fmt.Errorf("notion: parse checkpoint: %w", err)
	}
	return cp, nil
}
