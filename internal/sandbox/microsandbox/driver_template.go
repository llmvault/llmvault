package microsandbox

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	msbapi "github.com/usehivy/hivy/internal/microsandbox/api"
	"github.com/usehivy/hivy/internal/sandbox"
)

func (d *Driver) BuildTemplate(ctx context.Context, opts sandbox.TemplateBuildRequest) (string, error) {
	return d.BuildTemplateWithLogs(ctx, opts, nil)
}

func (d *Driver) BuildTemplateWithLogs(ctx context.Context, opts sandbox.TemplateBuildRequest, onLog func(string)) (string, error) {
	baseImage := strings.TrimSpace(opts.BaseImage)
	if baseImage == "" {
		baseImage = d.runtimeImage
	}
	if baseImage == "" {
		return "", fmt.Errorf("microsandbox: base image is required")
	}
	orgID := strings.TrimSpace(opts.OrgID)
	if orgID == "" {
		orgID = "system"
	}
	return d.postTemplateStream(ctx, map[string]any{
		"org_id":         orgID,
		"name":           opts.Name,
		"base_image_ref": baseImage,
		"size":           msbapi.DefaultSize,
		"commands":       opts.BuildCommands,
		"cpu":            opts.CPU,
		"memory_mb":      opts.Memory * 1024,
		"disk_gb":        opts.Disk,
	}, onLog)
}

func (d *Driver) GetTemplateStatus(ctx context.Context, externalID string) (*sandbox.TemplateBuildStatus, error) {
	if !isTemplateRef(externalID) {
		return nil, fmt.Errorf("microsandbox: invalid template id %q", externalID)
	}
	var out templateResponse
	if err := d.do(ctx, http.MethodGet, "/v1/templates/"+externalID, nil, &out); err != nil {
		return nil, err
	}
	return &sandbox.TemplateBuildStatus{State: out.Status, ErrorMsg: out.ErrorMessage}, nil
}

func (d *Driver) GetTemplateLogs(ctx context.Context, externalID string) (string, error) {
	if !isTemplateRef(externalID) {
		return "", fmt.Errorf("microsandbox: invalid template id %q", externalID)
	}
	var out templateResponse
	if err := d.do(ctx, http.MethodGet, "/v1/templates/"+externalID, nil, &out); err != nil {
		return "", err
	}
	return out.Logs, nil
}

func (d *Driver) DeleteTemplate(ctx context.Context, externalID string) error {
	if !isTemplateRef(externalID) {
		return fmt.Errorf("microsandbox: invalid template id %q", externalID)
	}
	return d.do(ctx, http.MethodDelete, "/v1/templates/"+externalID, nil, nil)
}

func isTemplateRef(ref string) bool {
	return strings.HasPrefix(strings.TrimSpace(ref), "tpl_")
}
