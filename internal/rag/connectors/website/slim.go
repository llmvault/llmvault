package website

import (
	"context"

	"github.com/usehivy/hivy/internal/goroutine"
	"github.com/usehivy/hivy/internal/rag/connectors/interfaces"
)

func (c *WebsiteConnector) ListAllSlim(
	ctx context.Context, _ interfaces.Source,
) (<-chan interfaces.SlimDocOrFailure, error) {
	out := make(chan interfaces.SlimDocOrFailure, 32)
	goroutine.Go(ctx, func(ctx context.Context) {
		defer close(out)
		for _, u := range c.urls() {
			slim := &interfaces.SlimDocument{DocID: canonicalURL(u)}
			if !interfaces.Send(ctx, out, interfaces.NewSlimResult(slim)) {
				return
			}
		}
	})
	return out, nil
}
