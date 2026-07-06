package linear

import (
	"bytes"
	"context"
	"net/http"

	"github.com/usehivy/hivy/internal/nango"
)

// proxyClient is the transport the connector drives. It returns the raw
// status/headers/body so the client can inspect rate-limit responses
// (Linear signals throttling with an HTTP 400 + a GraphQL error, not a
// 429) and GraphQL-level errors for itself. Every call is a POST of a
// JSON GraphQL request body to the /graphql endpoint.
type proxyClient interface {
	Do(ctx context.Context, body []byte) (status int, header http.Header, respBody []byte, err error)
}

// nangoProxy routes GraphQL requests through Nango's raw proxy. It always
// POSTs to "/graphql" with a JSON Content-Type. RawProxyRequestWithHeaders
// (unlike ProxyRequest) neither parses JSON nor raises on non-2xx, which
// is what the client needs to detect RATELIMITED 400s and surface GraphQL
// errors.
type nangoProxy struct {
	client            *nango.Client
	providerConfigKey string
	connectionID      string
}

func newNangoProxy(n *nango.Client, providerConfigKey, connectionID string) proxyClient {
	return &nangoProxy{
		client:            n,
		providerConfigKey: providerConfigKey,
		connectionID:      connectionID,
	}
}

func (p *nangoProxy) Do(ctx context.Context, body []byte) (int, http.Header, []byte, error) {
	headers := map[string]string{"Content-Type": "application/json"}
	resp, err := p.client.RawProxyRequestWithHeaders(
		ctx, http.MethodPost, p.providerConfigKey, p.connectionID,
		"/graphql", "", bytes.NewReader(body), headers,
	)
	if err != nil {
		return 0, nil, nil, err
	}
	return resp.StatusCode, resp.Header, resp.Body, nil
}
