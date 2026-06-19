package microsandbox

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/usehivy/hivy/internal/sandbox"
)

func (d *Driver) post(ctx context.Context, path string, in, out any) error {
	return d.do(ctx, http.MethodPost, path, in, out)
}

func (d *Driver) postTemplateStream(ctx context.Context, in any, onLog func(string)) (string, error) {
	raw, err := json.Marshal(in)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.controlURL+"/v1/templates", bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+d.apiToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := d.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("microsandbox returned %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	var templateID string
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		var event templateBuildEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return templateID, err
		}
		if event.ID != "" {
			templateID = event.ID
		}
		switch event.Type {
		case "log":
			if onLog != nil && event.Message != "" {
				onLog(event.Message)
			}
		case "error":
			if event.Message == "" {
				event.Message = "template build failed"
			}
			return templateID, fmt.Errorf("%s", event.Message)
		case "result":
			if event.ID != "" {
				templateID = event.ID
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return templateID, err
	}
	if templateID == "" {
		return "", fmt.Errorf("template build did not return a template id")
	}
	return templateID, nil
}

func (d *Driver) do(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		raw, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, d.controlURL+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+d.apiToken)
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := d.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return sandbox.ErrSandboxNotFound
	}
	if resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("microsandbox returned %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}
