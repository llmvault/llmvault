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
// implements only the checkpointed surface: Notion exposes no per-page
// ACL (so no permission sync) and no cheap total count (so no estimate).
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

// ValidateConfig parses the config and probes reachability. Workspace
// sources are validated against the bot-user endpoint; subtree sources
// probe each configured root (page, falling back to database).
func (c *NotionConnector) ValidateConfig(ctx context.Context, src interfaces.Source) error {
	cfg, err := LoadConfig(src.Config())
	if err != nil {
		return err
	}
	if len(cfg.RootPageIDs) == 0 {
		if _, err := c.client.usersMe(ctx); err != nil {
			return fmt.Errorf("notion: workspace not reachable: %w", err)
		}
		return nil
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

// LoadFromCheckpoint starts or resumes a run. A fresh checkpoint is
// classified into SUBTREE (roots configured) or WORKSPACE mode and, for
// subtree mode, seeded with the configured roots. The run streams
// documents on a buffered channel and stores the final checkpoint for
// FinalCheckpoint to read.
func (c *NotionConnector) LoadFromCheckpoint(
	ctx context.Context,
	src interfaces.Source,
	cp NotionCheckpoint,
	start, end time.Time,
) (<-chan interfaces.DocumentOrFailure, error) {
	if cp.Mode == "" {
		if len(c.cfg.RootPageIDs) > 0 {
			cp.Mode = modeSubtree
			cp.PendingPageIDs = append([]string(nil), c.cfg.RootPageIDs...)
		} else {
			cp.Mode = modeWorkspace
		}
		cp.HasMore = true
	}

	out := make(chan interfaces.DocumentOrFailure, c.channelBuf)
	goroutine.Go(ctx, func(ctx context.Context) {
		c.run(ctx, cp, start, end, out)
	})
	return out, nil
}

// run drives the whole traversal. In workspace mode it first enumerates
// pages via search (honouring the incremental window client-side), then
// drains the page frontier that both modes accumulate. Every emitted
// page can queue further child pages; the indexed set prevents cycles
// and re-work.
func (c *NotionConnector) run(
	ctx context.Context,
	cp NotionCheckpoint,
	start, end time.Time,
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

	if cp.Mode == modeWorkspace && !cp.SearchDone {
		if !c.enumerateWorkspace(ctx, w, &cp, indexed, start, end, out) {
			return // context cancelled mid-enumeration
		}
	}

	// Drain the frontier: pages seeded from roots (subtree) or queued as
	// children during processing (both modes).
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

// enumerateWorkspace pages through search results, processing each page
// inline and advancing the search cursor on the checkpoint. Search
// returns newest-edited first, so an incremental window early-breaks
// once results predate start. Returns false if the context was
// cancelled.
func (c *NotionConnector) enumerateWorkspace(
	ctx context.Context,
	w *walker,
	cp *NotionCheckpoint,
	indexed map[string]struct{},
	start, end time.Time,
	out chan<- interfaces.DocumentOrFailure,
) bool {
	for {
		if ctx.Err() != nil {
			return false
		}
		res, err := c.client.search(ctx, "page", true, cp.SearchCursor)
		if err != nil {
			// Search is the enumeration spine — a failure here can't be
			// attributed to one document, so report it as an entity
			// failure and stop enumerating (the frontier still drains).
			interfaces.Send(ctx, out, interfaces.NewDocFailure(
				entityFailure("notion:search", "notion: page search failed", err)))
			cp.SearchDone = true
			return true
		}

		olderThanStart := false
		for _, raw := range res.Results {
			var page NotionPage
			if !decodeInto(raw, &page) || page.ID == "" {
				continue
			}
			edited := parseNotionTime(page.LastEditedTime)
			if edited != nil {
				if !end.IsZero() && edited.After(end) {
					continue
				}
				if !start.IsZero() && edited.Before(start) {
					olderThanStart = true
					break
				}
				if cp.LastSeenEdited == nil || edited.After(*cp.LastSeenEdited) {
					cp.LastSeenEdited = edited
				}
			}
			if _, seen := indexed[page.ID]; seen {
				continue
			}
			if !c.processPage(ctx, w, page, indexed, cp, out) {
				return false
			}
		}

		if olderThanStart {
			cp.SearchDone = true
			return true
		}
		if res.HasMore && res.NextCursor != nil && *res.NextCursor != "" {
			cursor := *res.NextCursor
			cp.SearchCursor = &cursor
			continue
		}
		cp.SearchDone = true
		return true
	}
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

// decodeInto re-decodes a loosely-typed search result into a struct via
// a JSON round-trip.
func decodeInto(raw map[string]any, dst any) bool {
	b, err := json.Marshal(raw)
	if err != nil {
		return false
	}
	return json.Unmarshal(b, dst) == nil
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
