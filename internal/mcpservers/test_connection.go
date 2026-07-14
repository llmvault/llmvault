package mcpservers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

type ConnectionTestResult struct {
	Connected       bool           `json:"connected"`
	ProtocolVersion string         `json:"protocol_version,omitempty"`
	ServerInfo      map[string]any `json:"server_info,omitempty"`
	Capabilities    map[string]any `json:"capabilities,omitempty"`
}

type initializeResponse struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id"`
	Result  struct {
		ProtocolVersion string         `json:"protocolVersion"`
		ServerInfo      map[string]any `json:"serverInfo"`
		Capabilities    map[string]any `json:"capabilities"`
	} `json:"result"`
	Error *struct {
		Code int `json:"code"`
	} `json:"error"`
}

func (s *Service) TestConnection(ctx context.Context, server model.MCPServer, actorUserID *uuid.UUID) (*ConnectionTestResult, error) {
	runtimeServer, usable, err := s.runtimeServer(ctx, server, actorUserID)
	if err != nil {
		return nil, err
	}
	if !usable {
		return nil, validationErrorf("authorization is required before testing this server")
	}
	if runtimeServer.Transport == model.MCPTransportSSE {
		return s.testLegacySSEConnection(ctx, runtimeServer)
	}
	return s.testStreamableHTTPConnection(ctx, runtimeServer)
}

func (s *Service) testStreamableHTTPConnection(ctx context.Context, runtimeServer RuntimeServer) (*ConnectionTestResult, error) {
	payload := map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]any{
			"protocolVersion": "2025-11-25", "capabilities": map[string]any{},
			"clientInfo": map[string]any{"name": "hivy", "version": "1"},
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode MCP initialize request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, runtimeServer.URL, bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("create MCP initialize request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	request.Header.Set("MCP-Protocol-Version", "2025-11-25")
	for key, value := range runtimeServer.Headers {
		request.Header.Set(key, value)
	}
	response, err := s.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("connect to MCP server: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.CopyN(io.Discard, response.Body, maxOAuthResponseBytes)
		return nil, fmt.Errorf("MCP server returned status %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxOAuthResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("read MCP initialize response: %w", err)
	}
	if strings.HasPrefix(strings.ToLower(response.Header.Get("Content-Type")), "text/event-stream") {
		body = firstSSEData(body)
	}
	var initialized initializeResponse
	if err := json.Unmarshal(body, &initialized); err != nil {
		return nil, fmt.Errorf("decode MCP initialize response: %w", err)
	}
	if initialized.Error != nil || initialized.Result.ProtocolVersion == "" {
		return nil, validationErrorf("MCP server rejected initialization")
	}
	if err := s.sendInitializedNotification(ctx, runtimeServer, response.Header.Get("MCP-Session-Id"), initialized.Result.ProtocolVersion); err != nil {
		return nil, err
	}
	return &ConnectionTestResult{
		Connected: true, ProtocolVersion: initialized.Result.ProtocolVersion,
		ServerInfo: initialized.Result.ServerInfo, Capabilities: initialized.Result.Capabilities,
	}, nil
}
