// Package providerauth applies stored credential authentication to provider
// requests without depending on a particular inference client.
package providerauth

import "net/http"

// Attach sets the configured authentication header or query parameter.
func Attach(req *http.Request, scheme string, apiKey []byte) {
	switch scheme {
	case "bearer":
		req.Header.Set("Authorization", "Bearer "+string(apiKey))
	case "x-api-key":
		req.Header.Set("x-api-key", string(apiKey))
	case "xi-api-key":
		req.Header.Set("xi-api-key", string(apiKey))
	case "api-key":
		req.Header.Set("api-key", string(apiKey))
	case "query_param":
		query := req.URL.Query()
		query.Set("key", string(apiKey))
		req.URL.RawQuery = query.Encode()
	}
}
