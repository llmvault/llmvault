package proxy

import (
	"net/http"

	"github.com/usehivy/hivy/internal/providerauth"
)

// AttachAuth sets the appropriate authentication header or query parameter
// on the outbound request based on the credential's auth scheme.
func AttachAuth(req *http.Request, scheme string, apiKey []byte) {
	providerauth.Attach(req, scheme, apiKey)
}
