package runner

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/microsandbox/security"
)

func (m *MicrosandboxBackend) validateTemplateImage(ctx context.Context, req CreateTemplateRequest, imageRef string, onLog func(string)) (string, error) {
	short, err := security.ShortID("val")
	if err != nil {
		return "", err
	}
	validationID := "val-" + short
	if onLog != nil {
		onLog("validating template image with sandbox " + validationID)
	}
	createReq := CreateSandboxRequest{
		ID:           validationID,
		Name:         "validate-" + req.ID,
		ImageRef:     imageRef,
		CPU:          req.CPU,
		MemoryMB:     req.MemoryMB,
		DiskGB:       req.DiskGB,
		Env:          templateValidationEnv(req, validationID),
		PreviewPorts: []int{templateValidationPort},
		Init: &SandboxInitConfig{
			Cmd:  "/usr/local/bin/hivy-runtime-entrypoint",
			Args: []string{"/usr/local/bin/hivy-sandboxes-runtime"},
		},
		Labels: map[string]string{
			"org_id":      req.OrgID,
			"sandbox_id":  validationID,
			"harness":     "template-validation",
			"template_id": req.ID,
		},
	}
	if _, err := m.CreateSandbox(ctx, createReq); err != nil {
		return validationID, fmt.Errorf("create validation sandbox: %w", err)
	}
	defer func() {
		_ = m.DeleteSandbox(context.WithoutCancel(ctx), validationID)
	}()
	if err := m.waitForTemplateRuntime(ctx, validationID); err != nil {
		return validationID, err
	}
	if onLog != nil {
		onLog("template validation runtime ready")
	}
	return validationID, nil
}

func (m *MicrosandboxBackend) waitForTemplateRuntime(ctx context.Context, sandboxID string) error {
	baseURL, err := m.ProxyURL(ctx, sandboxID, templateValidationPort)
	if err != nil {
		return err
	}
	healthURL := strings.TrimRight(baseURL, "/") + "/healthz"
	deadline := time.Now().Add(templateValidationTimeout)
	client := &http.Client{Timeout: 5 * time.Second}
	for time.Now().Before(deadline) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err == nil {
			status := resp.StatusCode
			resp.Body.Close()
			if status == http.StatusOK {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(templateValidationInterval):
		}
	}
	return fmt.Errorf("template runtime not live within %s", templateValidationTimeout)
}

func templateValidationEnv(req CreateTemplateRequest, sandboxID string) map[string]string {
	return map[string]string{
		agentruntime.AgentEnvRuntimeSecret:     "template-validation",
		agentruntime.AgentEnvDriveUploadBearer: "template-validation",
		agentruntime.AgentEnvRuntimeBindAddr:   fmt.Sprintf("0.0.0.0:%d", templateValidationPort),
		agentruntime.AgentEnvWorkspaceRoot:     "/workspace",
		agentruntime.AgentEnvDBPath:            agentruntime.AgentRuntimeDBPath,
		agentruntime.AgentEnvTunnelPassword:    "template-validation",
		agentruntime.AgentEnvSandboxID:         sandboxID,
		agentruntime.AgentEnvOrgID:             req.OrgID,
	}
}
