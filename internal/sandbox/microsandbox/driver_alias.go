package microsandbox

import (
	"context"
	"net/http"

	"github.com/usehivy/hivy/internal/sandbox"
)

// Driver implements the optional alias capability; callers type-assert against
// sandbox.AliasProvider.
var _ sandbox.AliasProvider = (*Driver)(nil)

type aliasClaimRequest struct {
	SandboxID string `json:"sandbox_id"`
	Port      int    `json:"port"`
}

type aliasClaimResponse struct {
	Alias     string `json:"alias"`
	URL       string `json:"url"`
	SandboxID string `json:"sandbox_id"`
	Port      int    `json:"port"`
}

func (d *Driver) ClaimAlias(ctx context.Context, alias string, sandboxExternalID string, port int) (string, error) {
	var out aliasClaimResponse
	if err := d.do(ctx, http.MethodPut, "/v1/aliases/"+alias, aliasClaimRequest{
		SandboxID: sandboxExternalID,
		Port:      port,
	}, &out); err != nil {
		return "", err
	}
	return out.URL, nil
}

func (d *Driver) ReleaseAlias(ctx context.Context, alias string) error {
	return d.do(ctx, http.MethodDelete, "/v1/aliases/"+alias, nil, nil)
}
