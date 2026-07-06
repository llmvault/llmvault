package linear

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"time"

	"github.com/usehivy/hivy/internal/goroutine"
	"github.com/usehivy/hivy/internal/rag/connectors/interfaces"
)

// Kind is the connector identifier. It must equal the Nango provider
// string so registration and connection lookup line up.
const Kind = "linear"

// channelBufSize buffers the document stream so the producer goroutine
// stays ahead of the scheduler's drain.
const channelBufSize = 200

// pollOverlap absorbs clock skew + Linear indexing latency between
// successive incremental fetches. Applied connector-side to the start
// bound so the effective window is slightly wider than requested.
const pollOverlap = 10 * time.Minute

var (
	_ interfaces.CheckpointedConnector[LinearCheckpoint] = (*LinearConnector)(nil)
	_ interfaces.SlimConnector                           = (*LinearConnector)(nil)
)

// LinearConnector indexes Linear teams via Nango-proxied GraphQL calls.
// It walks each selected team's projects (emitting project + collected
// initiative docs) in a PROJECTS stage, then does one combined cursor
// walk over issues filtered by all team IDs in an ISSUES stage.
type LinearConnector struct {
	cfg        Config
	client     *Client
	channelBuf int

	finalCp atomic.Pointer[LinearCheckpoint]
}

// NewConnector builds a connector over a proxy transport. Exposed for
// tests, which inject a fake proxy.
func NewConnector(cfg Config, p proxyClient) *LinearConnector {
	return &LinearConnector{
		cfg:        cfg,
		client:     newClient(p),
		channelBuf: channelBufSize,
	}
}

func (c *LinearConnector) Kind() string { return Kind }

// ValidateConfig parses the config and surfaces any parse error. A
// source with no teams ingests nothing (deny-by-default) but is still a
// valid configuration, so an empty TeamIDs list is not an error.
func (c *LinearConnector) ValidateConfig(_ context.Context, src interfaces.Source) error {
	_, err := LoadConfig(src.Config())
	return err
}

func (c *LinearConnector) DummyCheckpoint() LinearCheckpoint { return dummyCheckpoint() }

func (c *LinearConnector) UnmarshalCheckpoint(raw json.RawMessage) (LinearCheckpoint, error) {
	return unmarshalCheckpoint(raw)
}

// LoadFromCheckpoint starts or resumes a run seeded with the configured
// teams. A source with no teams has an empty frontier and ingests
// nothing (scope is deny-by-default): it jumps straight to DONE. The run
// streams documents on a buffered channel and stores the final
// checkpoint for FinalCheckpoint to read.
//
// pollOverlap is applied to start internally, so the effective window
// may be wider than the window passed in. A zero start means a full
// walk (no updatedSince filter).
func (c *LinearConnector) LoadFromCheckpoint(
	ctx context.Context,
	_ interfaces.Source,
	cp LinearCheckpoint,
	start, _ time.Time,
) (<-chan interfaces.DocumentOrFailure, error) {
	if !cp.Stage.IsValid() {
		cp = dummyCheckpoint()
	}
	if cp.Stage == StageStart {
		cp.PendingTeamIDs = append([]string(nil), c.cfg.TeamIDs...)
		cp.Stage = StageProjects
		if len(c.cfg.TeamIDs) == 0 {
			cp.Stage = StageDone // deny-by-default: nothing to walk
		}
	}

	var updatedSince *time.Time
	if !start.IsZero() {
		effectiveStart := start.Add(-pollOverlap)
		updatedSince = &effectiveStart
	}

	out := make(chan interfaces.DocumentOrFailure, c.channelBuf)
	goroutine.Go(ctx, func(ctx context.Context) {
		c.run(ctx, cp, updatedSince, out)
	})
	return out, nil
}

// Build constructs a connector bound to a source and its Nango
// connection. Mirrors the notion/github factory shape.
func Build(src interfaces.Source, deps interfaces.BuildDeps) (interfaces.Connector, error) {
	cfg, err := LoadConfig(src.Config())
	if err != nil {
		return nil, err
	}
	connectionID, providerKey := connectionFromSource(src)
	return NewConnector(cfg, newNangoProxy(deps.Nango, providerKey, connectionID)), nil
}

// connectionFromSource returns ("", Kind) when the Source lacks Nango
// fields, so the connector fails fast on its first proxy call.
func connectionFromSource(src interfaces.Source) (string, string) {
	type connectionSource interface {
		NangoConnectionID() string
		NangoProviderConfigKey() string
	}
	if cs, ok := src.(connectionSource); ok {
		return cs.NangoConnectionID(), cs.NangoProviderConfigKey()
	}
	return "", Kind
}
