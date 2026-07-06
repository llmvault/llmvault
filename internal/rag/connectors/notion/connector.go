package notion

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync/atomic"
	"time"

	"github.com/usehivy/hivy/internal/goroutine"
	"github.com/usehivy/hivy/internal/rag/connectors/interfaces"
)

// Kind is the connector identifier. It must equal the Nango provider
// string so registration and connection lookup line up.
const Kind = "notion"

// channelBufSize buffers the document stream so the producer goroutine
// stays ahead of the scheduler's drain.
const channelBufSize = 200

var _ interfaces.CheckpointedConnector[NotionCheckpoint] = (*NotionConnector)(nil)

// NotionConnector indexes Notion pages via Nango-proxied API calls. It
// implements only the checkpointed surface: Notion exposes no cheap
// total count (so no estimate).
type NotionConnector struct {
	cfg        NotionConfig
	client     *Client
	channelBuf int

	finalCp atomic.Pointer[NotionCheckpoint]
}

// NewConnector builds a connector over a proxy transport. Exposed for
// tests, which inject a fake proxy.
func NewConnector(cfg NotionConfig, p proxyClient) *NotionConnector {
	return &NotionConnector{
		cfg:        cfg,
		client:     newClient(p),
		channelBuf: channelBufSize,
	}
}

func (c *NotionConnector) Kind() string { return Kind }

// ValidateConfig parses the config and probes that each configured root is
// reachable and shared with the integration. A source with no roots ingests
// nothing, so there is nothing to validate.
func (c *NotionConnector) ValidateConfig(ctx context.Context, src interfaces.Source) error {
	cfg, err := LoadConfig(src.Config())
	if err != nil {
		return err
	}
	for _, id := range cfg.RootPageIDs {
		_, ok, err := c.client.page(ctx, id)
		if err != nil {
			return fmt.Errorf("notion: root %s not reachable: %w", id, err)
		}
		if !ok {
			return fmt.Errorf("notion: root %s is not shared with the integration", id)
		}
	}
	return nil
}

func (c *NotionConnector) DummyCheckpoint() NotionCheckpoint { return dummyCheckpoint() }

func (c *NotionConnector) UnmarshalCheckpoint(raw json.RawMessage) (NotionCheckpoint, error) {
	return unmarshalCheckpoint(raw)
}

// LoadFromCheckpoint starts or resumes a run seeded with the configured roots.
// A source with no roots has an empty frontier and ingests nothing (scope is
// deny-by-default). The run streams documents on a buffered channel and stores
// the final checkpoint for FinalCheckpoint to read.
func (c *NotionConnector) LoadFromCheckpoint(
	ctx context.Context,
	_ interfaces.Source,
	cp NotionCheckpoint,
	_, _ time.Time,
) (<-chan interfaces.DocumentOrFailure, error) {
	if cp.Mode == "" {
		cp.Mode = modeSubtree
		cp.PendingPageIDs = append([]string(nil), c.cfg.RootPageIDs...)
		cp.HasMore = true
	}

	out := make(chan interfaces.DocumentOrFailure, c.channelBuf)
	goroutine.Go(ctx, func(ctx context.Context) {
		c.run(ctx, cp, out)
	})
	return out, nil
}

// run drains the page frontier seeded from the configured roots. Every emitted
// page can queue further child pages; the indexed set prevents cycles and
// re-work.
func (c *NotionConnector) run(
	ctx context.Context,
	cp NotionCheckpoint,
	out chan<- interfaces.DocumentOrFailure,
) {
	defer close(out)

	indexed := make(map[string]struct{}, len(cp.IndexedPageIDs))
	for _, id := range cp.IndexedPageIDs {
		indexed[id] = struct{}{}
	}

	// Persist whatever progress we made, even on early return.
	defer func() {
		cp.IndexedPageIDs = sortedSet(indexed)
		final := cp
		c.finalCp.Store(&final)
	}()

	w := newWalker(c.client, c.cfg.IncludeDatabases)

	// Drain the frontier: pages seeded from the configured roots, plus any
	// child pages queued during processing.
	for len(cp.PendingPageIDs) > 0 {
		if ctx.Err() != nil {
			return
		}
		id := cp.PendingPageIDs[0]
		cp.PendingPageIDs = cp.PendingPageIDs[1:]
		if _, seen := indexed[id]; seen {
			continue
		}

		page, ok, err := c.client.page(ctx, id)
		if err != nil {
			indexed[id] = struct{}{} // don't retry a hard-failing id forever
			if !interfaces.Send(ctx, out, interfaces.NewDocFailure(
				docFailure(docIDForPage(id), "", "notion: fetch page failed", err))) {
				return
			}
			continue
		}
		if !ok {
			indexed[id] = struct{}{} // unshared: skip permanently
			continue
		}
		if !c.processPage(ctx, w, page, indexed, &cp, out) {
			return
		}
	}

	cp.HasMore = false
}

// processPage walks a page's blocks, emits its document, and queues any
// child pages discovered. On a block-walk error it emits a per-page
// failure instead of a document. Returns false if the context was
// cancelled while sending.
func (c *NotionConnector) processPage(
	ctx context.Context,
	w *walker,
	page NotionPage,
	indexed map[string]struct{},
	cp *NotionCheckpoint,
	out chan<- interfaces.DocumentOrFailure,
) bool {
	if _, seen := indexed[page.ID]; seen {
		return true
	}
	indexed[page.ID] = struct{}{}

	blockOut, err := w.readBlocks(ctx, page.ID, page.ID)
	if err != nil {
		if ctx.Err() != nil {
			return false
		}
		return interfaces.Send(ctx, out, interfaces.NewDocFailure(
			docFailure(docIDForPage(page.ID), page.URL, "notion: read page blocks failed", err)))
	}

	doc := pageToDocument(page, blockOut.Blocks)
	if !interfaces.Send(ctx, out, interfaces.NewDocResult(&doc)) {
		return false
	}

	for _, childID := range blockOut.ChildPageIDs {
		if _, seen := indexed[childID]; seen {
			continue
		}
		cp.PendingPageIDs = append(cp.PendingPageIDs, childID)
	}
	return true
}

// sortedSet renders a set as a sorted slice for stable serialisation.
func sortedSet(set map[string]struct{}) []string {
	if len(set) == 0 {
		return nil
	}
	out := make([]string, 0, len(set))
	for id := range set {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// Build constructs a connector bound to a source and its Nango
// connection. Mirrors the github/slack factory shape.
func Build(src interfaces.Source, deps interfaces.BuildDeps) (interfaces.Connector, error) {
	cfg, err := LoadConfig(src.Config())
	if err != nil {
		return nil, err
	}
	connectionID, providerKey := connectionFromSource(src)
	return NewConnector(cfg, newNangoProxy(deps.Nango, providerKey, connectionID)), nil
}

// connectionFromSource returns ("","") when the Source lacks Nango
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
