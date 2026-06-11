package handler

import (
	"net/http"
	"strings"
)

func slackProxyAPIPathAllowed(path string) bool {
	method := strings.Trim(strings.TrimLeft(path, "/"), "/")
	return method != "" && !strings.Contains(method, "/") && strings.Contains(method, ".")
}

func (h *RawProviderProxyHandler) nangoProxyPath(path string) string {
	if h.provider != "glitchtip" {
		return path
	}
	withoutAPIBase := strings.TrimPrefix(path, "/api/0")
	if withoutAPIBase == "" || withoutAPIBase == "/" {
		return "/"
	}
	return "/" + strings.TrimLeft(withoutAPIBase, "/")
}

func rawProviderProxyMethodAllowed(method string) bool {
	switch method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}
