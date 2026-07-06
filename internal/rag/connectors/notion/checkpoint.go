package notion

import (
	"encoding/json"
	"fmt"

	"github.com/usehivy/hivy/internal/rag/connectors/interfaces"
)

const modeSubtree = "SUBTREE"

// NotionCheckpoint carries resumable state across runs. The page frontier
// (PendingPageIDs) and dedup set (IndexedPageIDs) let a run pick up mid
// traversal. It embeds interfaces.AnyCheckpoint to satisfy the Checkpoint
// marker; HasMore is set false once a run completes.
type NotionCheckpoint struct {
	interfaces.AnyCheckpoint

	// Mode is "" until the first run initializes the frontier from the roots.
	Mode string `json:"mode,omitempty"`

	// PendingPageIDs is the BFS frontier of pages still to index.
	PendingPageIDs []string `json:"pending_page_ids,omitempty"`

	// IndexedPageIDs is the serialised form of the in-run dedup set.
	IndexedPageIDs []string `json:"indexed_page_ids,omitempty"`
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
