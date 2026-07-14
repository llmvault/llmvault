package mcpservers

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type boundedSSEReader struct {
	scanner   *bufio.Scanner
	bytesRead int
}

func newBoundedSSEReader(body io.Reader) *boundedSSEReader {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 4096), maxOAuthResponseBytes)
	return &boundedSSEReader{scanner: scanner}
}

func (r *boundedSSEReader) Next() (string, string, error) {
	var event string
	var data []string
	for r.scanner.Scan() {
		line := r.scanner.Text()
		r.bytesRead += len(line) + 1
		if r.bytesRead > maxOAuthResponseBytes {
			return "", "", validationErrorf("legacy MCP event stream exceeded the response limit")
		}
		line = strings.TrimSuffix(line, "\r")
		if line == "" {
			if len(data) > 0 {
				return event, strings.Join(data, "\n"), nil
			}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		field, value, found := strings.Cut(line, ":")
		if !found {
			field = line
			value = ""
		} else {
			value = strings.TrimPrefix(value, " ")
		}
		switch field {
		case "event":
			event = value
		case "data":
			data = append(data, value)
		}
	}
	if err := r.scanner.Err(); err != nil {
		return "", "", err
	}
	if len(data) > 0 {
		return event, strings.Join(data, "\n"), nil
	}
	return "", "", io.ErrUnexpectedEOF
}

func (s *Service) sendInitializedNotification(ctx context.Context, server RuntimeServer, sessionID, protocolVersion string) error {
	raw := []byte(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL, bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("create MCP initialized notification: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	request.Header.Set("MCP-Protocol-Version", protocolVersion)
	if sessionID != "" {
		request.Header.Set("MCP-Session-Id", sessionID)
	}
	for key, value := range server.Headers {
		request.Header.Set(key, value)
	}
	response, err := s.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("send MCP initialized notification: %w", err)
	}
	defer response.Body.Close()
	_, _ = io.CopyN(io.Discard, response.Body, maxOAuthResponseBytes)
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("MCP initialized notification returned status %d", response.StatusCode)
	}
	return nil
}

func firstSSEData(body []byte) []byte {
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "data:") {
			return []byte(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	return body
}
