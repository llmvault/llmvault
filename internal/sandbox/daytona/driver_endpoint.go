package daytona

import (
	"context"
	"fmt"
)

// GetEndpoint returns a 24-hour signed preview URL for the given sandbox port.
// The sandbox remains private; Daytona embeds preview authentication in the URL
// while the Hivy runtime continues to enforce its own bearer token.
func (d *Driver) GetEndpoint(ctx context.Context, externalID string, port int) (string, error) {
	if port <= 0 || port > 1<<31-1 {
		return "", fmt.Errorf("invalid preview port %d", port)
	}
	sb, err := d.sdk.Get(ctx, externalID)
	if err != nil {
		return "", fmt.Errorf("getting sandbox %s: %w", externalID, err)
	}
	resp, err := sb.GetSignedPreviewLink(ctx, port, signedURLTTLSeconds)
	if err != nil {
		return "", fmt.Errorf("getting signed preview URL for sandbox %s port %d: %w", externalID, port, err)
	}
	if resp == nil {
		return "", fmt.Errorf("daytona returned no signed preview URL for sandbox %s port %d", externalID, port)
	}
	return resp.URL, nil
}
