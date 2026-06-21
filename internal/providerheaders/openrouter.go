package providerheaders

import (
	"net/http"
	"strings"
)

const (
	OpenRouterHTTPReferer = "https://usehivy.com"
	OpenRouterAppTitle    = "Hivy"
)

func IsOpenRouter(providerID, baseURL string) bool {
	if strings.EqualFold(strings.TrimSpace(providerID), "openrouter") {
		return true
	}
	return strings.Contains(strings.ToLower(strings.TrimSpace(baseURL)), "openrouter.ai")
}

func ApplyOpenRouter(req *http.Request) {
	if req == nil {
		return
	}
	req.Header.Set("HTTP-Referer", OpenRouterHTTPReferer)
	req.Header.Set("X-Title", OpenRouterAppTitle)
}
