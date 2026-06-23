package microsandbox

import (
	"context"
	"fmt"
	"strings"

	"github.com/usehivy/hivy/internal/sandbox"
)

func (d *Driver) UpgradeSandbox(ctx context.Context, externalID string, opts sandbox.UpgradeSandboxOpts) (*sandbox.SandboxInfo, error) {
	templateRef := strings.TrimSpace(opts.TemplateRef)
	imageRef := templateRef
	templateID := ""
	if isTemplateRef(templateRef) {
		templateID = templateRef
		imageRef = ""
	}
	if imageRef == "" && templateID == "" {
		imageRef = d.runtimeImage
	}
	if imageRef == "" && templateID == "" {
		return nil, fmt.Errorf("microsandbox: image ref is required")
	}
	body := map[string]any{
		"name":          opts.Name,
		"image_ref":     imageRef,
		"template_id":   templateID,
		"cpu":           opts.CPU,
		"memory_mb":     opts.Memory * 1024,
		"disk_gb":       opts.Disk,
		"env":           opts.EnvVars,
		"metadata":      opts.Labels,
		"preview_ports": d.previewPorts(opts.ExposedPorts),
		"health_checks": d.runtimeHealthChecks(d.runtimePort),
		"init":          agentRuntimeInit,
	}
	var out createSandboxResponse
	if err := d.post(ctx, "/v1/sandboxes/"+externalID+"/upgrade", body, &out); err != nil {
		return nil, err
	}
	return &sandbox.SandboxInfo{ExternalID: out.ID, Status: sandbox.StatusRunning}, nil
}
