package linear

import (
	"context"
	"encoding/json"
	"time"

	"github.com/usehivy/hivy/internal/rag/connectors/interfaces"
)

// Run adapts the generic LoadFromCheckpoint surface to the non-generic
// RunnableCheckpointed interface the ingest handler drives.
func (c *LinearConnector) Run(
	ctx context.Context,
	src interfaces.Source,
	checkpointJSON json.RawMessage,
	start, end time.Time,
) (<-chan interfaces.DocumentOrFailure, error) {
	cp, err := c.UnmarshalCheckpoint(checkpointJSON)
	if err != nil {
		return nil, err
	}
	return c.LoadFromCheckpoint(ctx, src, cp, start, end)
}

// FinalCheckpoint returns the serialised checkpoint at the end of a run,
// or nil if no run has completed.
func (c *LinearConnector) FinalCheckpoint() (json.RawMessage, error) {
	cp := c.finalCp.Load()
	if cp == nil {
		return nil, nil
	}
	return json.Marshal(*cp)
}
