package notion

import (
	"bytes"
	"context"
	"io"
	"net/http"

	"github.com/usehivy/hivy/internal/nango"
)

// proxyClient is the transport the connector drives. It returns the raw
// status/headers/body so the connector can decide for itself which 4xx
// codes are tolerable (403/404 = unshared) versus fatal. A nil body
// means a GET; a non-nil body is sent as a JSON POST payload.
type proxyClient interface {
	Do(ctx context.Context, method, path, rawQuery string, body []byte) (int, http.Header, []byte, error)
}

// nangoProxy routes requests through Nango's raw proxy, adding the
// Notion-Version header every call requires and a JSON Content-Type when
// a body is present. RawProxyRequestWithHeaders (unlike ProxyRequest)
// neither parses JSON nor raises on non-2xx, which is what the
// connector needs to inspect rate-limit and permission responses.
type nangoProxy struct {
	client            *nango.Client
	providerConfigKey string
	connectionID      string
}

func newNangoProxy(c *nango.Client, providerConfigKey, connectionID string) proxyClient {
	return &nangoProxy{
		client:            c,
		providerConfigKey: providerConfigKey,
		connectionID:      connectionID,
	}
}

func (p *nangoProxy) Do(
	ctx context.Context, method, path, rawQuery string, body []byte,
) (int, http.Header, []byte, error) {
	headers := map[string]string{"Notion-Version": notionVersion}

	var reader io.Reader
	if len(body) > 0 {
		reader = bytes.NewReader(body)
		headers["Content-Type"] = "application/json"
	}

	resp, err := p.client.RawProxyRequestWithHeaders(
		ctx, method, p.providerConfigKey, p.connectionID, path, rawQuery, reader, headers,
	)
	if err != nil {
		return 0, nil, nil, err
	}
	return resp.StatusCode, resp.Header, resp.Body, nil
}
