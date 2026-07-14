package mcpservers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
)

func (s *Service) testLegacySSEConnection(ctx context.Context, runtimeServer RuntimeServer) (*ConnectionTestResult, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, runtimeServer.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("create legacy MCP SSE request: %w", err)
	}
	request.Header.Set("Accept", "text/event-stream")
	setRuntimeHeaders(request, runtimeServer.Headers)

	response, err := s.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("connect to legacy MCP SSE server: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.CopyN(io.Discard, response.Body, maxOAuthResponseBytes)
		return nil, fmt.Errorf("legacy MCP SSE server returned status %d", response.StatusCode)
	}
	mediaType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || !strings.EqualFold(mediaType, "text/event-stream") {
		return nil, validationErrorf("legacy MCP server did not return an event stream")
	}

	reader := newBoundedSSEReader(response.Body)
	var messageEndpoint string
	for messageEndpoint == "" {
		event, data, readErr := reader.Next()
		if readErr != nil {
			return nil, fmt.Errorf("read legacy MCP SSE endpoint: %w", readErr)
		}
		if event != "endpoint" {
			continue
		}
		messageEndpoint, err = resolveLegacySSEEndpoint(response.Request.URL, data)
		if err != nil {
			return nil, err
		}
	}

	initialize := map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]any{
			"protocolVersion": "2024-11-05", "capabilities": map[string]any{},
			"clientInfo": map[string]any{"name": "hivy", "version": "1"},
		},
	}
	if err := s.postLegacySSEMessage(ctx, runtimeServer, messageEndpoint, initialize); err != nil {
		return nil, err
	}

	var initialized initializeResponse
	for initialized.Result.ProtocolVersion == "" && initialized.Error == nil {
		event, data, readErr := reader.Next()
		if readErr != nil {
			return nil, fmt.Errorf("read legacy MCP initialize response: %w", readErr)
		}
		if event != "" && event != "message" {
			continue
		}
		var candidate initializeResponse
		if err := json.Unmarshal([]byte(data), &candidate); err != nil || candidate.ID == nil {
			continue
		}
		if fmt.Sprint(candidate.ID) != "1" {
			continue
		}
		initialized = candidate
	}
	if initialized.Error != nil || initialized.Result.ProtocolVersion == "" {
		return nil, validationErrorf("MCP server rejected initialization")
	}
	if err := s.postLegacySSEMessage(ctx, runtimeServer, messageEndpoint, map[string]any{
		"jsonrpc": "2.0", "method": "notifications/initialized",
	}); err != nil {
		return nil, err
	}
	return &ConnectionTestResult{
		Connected: true, ProtocolVersion: initialized.Result.ProtocolVersion,
		ServerInfo: initialized.Result.ServerInfo, Capabilities: initialized.Result.Capabilities,
	}, nil
}

func (s *Service) postLegacySSEMessage(ctx context.Context, server RuntimeServer, endpoint string, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode legacy MCP message: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("create legacy MCP message request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	setRuntimeHeaders(request, server.Headers)
	response, err := s.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("send legacy MCP message: %w", err)
	}
	defer response.Body.Close()
	_, _ = io.CopyN(io.Discard, response.Body, maxOAuthResponseBytes)
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("legacy MCP message endpoint returned status %d", response.StatusCode)
	}
	return nil
}

func setRuntimeHeaders(request *http.Request, headers map[string]string) {
	for key, value := range headers {
		request.Header.Set(key, value)
	}
}

func resolveLegacySSEEndpoint(eventURL *url.URL, raw string) (string, error) {
	endpoint, err := eventURL.Parse(strings.TrimSpace(raw))
	if err != nil || endpoint.Host == "" || endpoint.User != nil || endpoint.Fragment != "" {
		return "", validationErrorf("legacy MCP server announced an invalid message endpoint")
	}
	if !sameOrigin(eventURL, endpoint) {
		return "", validationErrorf("legacy MCP server announced a cross-origin message endpoint")
	}
	return endpoint.String(), nil
}

func sameOrigin(left, right *url.URL) bool {
	return strings.EqualFold(left.Scheme, right.Scheme) &&
		strings.EqualFold(left.Hostname(), right.Hostname()) &&
		effectivePort(left) == effectivePort(right)
}

func effectivePort(value *url.URL) string {
	if value.Port() != "" {
		return value.Port()
	}
	if strings.EqualFold(value.Scheme, "https") {
		return "443"
	}
	if strings.EqualFold(value.Scheme, "http") {
		return "80"
	}
	return ""
}
