package daytona

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	daytonasdk "github.com/daytona/clients/sdk-go/pkg/daytona"
	daytonaoptions "github.com/daytona/clients/sdk-go/pkg/options"
	sdktypes "github.com/daytona/clients/sdk-go/pkg/types"
	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/sandbox"
)

func (d *Driver) BuildTemplate(ctx context.Context, opts sandbox.TemplateBuildRequest) (string, error) {
	return d.buildImage(ctx, opts, nil)
}

func (d *Driver) BuildTemplateWithLogs(ctx context.Context, opts sandbox.TemplateBuildRequest, onLog func(string)) (string, error) {
	return d.buildImage(ctx, opts, onLog)
}

func (d *Driver) buildImage(ctx context.Context, opts sandbox.TemplateBuildRequest, onLog func(string)) (string, error) {
	baseImage := opts.BaseImage
	if baseImage == "" {
		baseImage = "node:22-bookworm-slim"
	}
	if isHivyRuntimeImageRef(baseImage) {
		return d.buildFromRuntimeSnapshot(ctx, opts, baseImage, onLog)
	}

	image := daytonasdk.Base(baseImage)

	// Minimal runtime tools — the canonical runtime image with uv/Go/Rust
	// lives in cmd/buildtemplates; user templates layer on top via BuildCommands.
	image = image.AptGet([]string{"ca-certificates", "curl", "git", "jq", "unzip", "openssh-client"})

	image = image.Run(
		"curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg | dd of=/usr/share/keyrings/githubcli-archive-keyring.gpg && " +
			`echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" | tee /etc/apt/sources.list.d/github-cli.list > /dev/null && ` +
			"apt-get update && apt-get install -y --no-install-recommends gh && rm -rf /var/lib/apt/lists/*",
	)

	image = image.Env("HOME", "/workspace")
	image = image.Env("NO_BROWSER", "1")

	if len(opts.BuildCommands) > 0 {
		commands := make([]string, 0, len(opts.BuildCommands))
		for _, cmd := range opts.BuildCommands {
			trimmed := strings.TrimSpace(cmd)
			if trimmed != "" {
				commands = append(commands, trimmed)
			}
		}
		if len(commands) > 0 {
			image = image.Run(strings.Join(commands, " && "))
		}
	}

	image = image.Workdir("/workspace")

	params := &sdktypes.CreateSnapshotParams{
		Name:  opts.Name,
		Image: image,
	}
	if opts.CPU > 0 || opts.Memory > 0 || opts.Disk > 0 {
		cpu := min(opts.CPU, d.limits.CPU)
		memory := min(opts.Memory, d.limits.Memory)
		disk := min(opts.Disk, d.limits.Disk)
		if size, ok := model.TemplateSizeForResources(opts.CPU, opts.Memory, opts.Disk); ok && size == "micro" {
			memory = 1
		}
		if cpu != opts.CPU || memory != opts.Memory || disk != opts.Disk {
			logging.FromContext(ctx).InfoContext(ctx, "adjust Daytona template allocation",
				"requested_cpu", opts.CPU,
				"daytona_cpu", cpu,
				"requested_memory_gb", opts.Memory,
				"daytona_memory_gb", memory,
				"requested_disk_gb", opts.Disk,
				"daytona_disk_gb", disk,
				"template", opts.Name,
			)
		}
		params.Resources = &sdktypes.Resources{
			CPU:    cpu,
			Memory: memory,
			Disk:   disk,
		}
	}

	snapshot, logChan, err := d.sdk.Snapshot.Create(ctx, params)
	if err != nil {
		return "", fmt.Errorf("creating snapshot: %w", err)
	}

	if logChan != nil {
		go func() {
			for line := range logChan {
				if onLog != nil {
					onLog(line)
				}
			}
		}()
	} else if onLog != nil {
		onLog("no log channel available from provider")
	}

	return snapshot.Name, nil
}

func (d *Driver) buildFromRuntimeSnapshot(
	ctx context.Context,
	opts sandbox.TemplateBuildRequest,
	baseImage string,
	onLog func(string),
) (string, error) {
	snapshotRef, err := resolveRuntimeSnapshotRef(sandbox.CreateSandboxOpts{
		TemplateRef: baseImage,
		CPU:         opts.CPU,
		Memory:      opts.Memory,
		Disk:        opts.Disk,
	})
	if err != nil {
		return "", fmt.Errorf("resolving Daytona runtime snapshot: %w", err)
	}
	if adjustment, ok := daytonaRuntimeResourceAdjustment(sandbox.CreateSandboxOpts{
		TemplateRef: baseImage,
		CPU:         opts.CPU,
		Memory:      opts.Memory,
		Disk:        opts.Disk,
	}, d.limits); ok && adjustment.Changed() {
		logging.FromContext(ctx).InfoContext(ctx, "adjust Daytona runtime template allocation",
			"size", adjustment.Size,
			"requested_cpu", adjustment.RequestedCPU,
			"daytona_cpu", adjustment.CPU,
			"requested_memory_gb", adjustment.RequestedMemory,
			"daytona_memory_gb", adjustment.Memory,
			"requested_disk_gb", adjustment.RequestedDisk,
			"daytona_disk_gb", adjustment.Disk,
			"template", opts.Name,
		)
	}

	buildName := fmt.Sprintf("hivy-template-build-%s", uuid.NewString())
	buildSandbox, err := d.sdk.Create(ctx, sdktypes.SnapshotParams{
		SandboxBaseParams: sdktypes.SandboxBaseParams{
			Name:            buildName,
			User:            daytonaUser,
			Public:          false,
			NetworkBlockAll: false,
			EnvVars: map[string]string{
				"HOME":                   daytonaHome,
				"HIVY_SANDBOX_DATA_ROOT": daytonaDataRoot,
				"HIVY_DB_PATH":           daytonaDBPath,
				"HIVY_WORKSPACE_ROOT":    daytonaWorkspaceRoot,
			},
		},
		Snapshot: snapshotRef,
	})
	if err != nil {
		return "", fmt.Errorf("creating Daytona template build sandbox: %w", err)
	}
	defer func() {
		// Template-build sandboxes are temporary. Use a fresh context so caller
		// cancellation does not leave paid infrastructure behind.
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
		defer cancel()
		_ = buildSandbox.Delete(cleanupCtx)
	}()

	if err := buildSandbox.WaitForStart(ctx, 3*time.Minute); err != nil {
		return "", fmt.Errorf("waiting for Daytona template build sandbox: %w", err)
	}
	if onLog != nil {
		onLog(fmt.Sprintf("building template from Daytona snapshot %s", snapshotRef))
	}

	commands := nonEmptyCommands(opts.BuildCommands)
	if len(commands) > 0 {
		result, execErr := buildSandbox.Process.ExecuteCommand(
			ctx,
			strings.Join(commands, " && "),
			daytonaoptions.WithCwd(daytonaWorkspaceRoot),
			daytonaoptions.WithExecuteTimeout(15*time.Minute),
		)
		if execErr != nil {
			return "", fmt.Errorf("executing Daytona template build commands: %w", execErr)
		}
		if onLog != nil && strings.TrimSpace(result.Result) != "" {
			onLog(result.Result)
		}
		if result.ExitCode != 0 {
			return "", fmt.Errorf("Daytona template build commands exited with status %d", result.ExitCode)
		}
	}

	// The runtime starts while the template is being prepared. Do not bake its
	// transient database or logs into every sandbox created from the template.
	cleanupResult, execErr := buildSandbox.Process.ExecuteCommand(
		ctx,
		fmt.Sprintf("rm -rf %s/runtime/* %s/logs/*", daytonaDataRoot, daytonaDataRoot),
		daytonaoptions.WithExecuteTimeout(time.Minute),
	)
	if execErr != nil {
		return "", fmt.Errorf("cleaning Daytona template runtime state: %w", execErr)
	}
	if cleanupResult.ExitCode != 0 {
		return "", fmt.Errorf("cleaning Daytona template runtime state exited with status %d", cleanupResult.ExitCode)
	}
	if err := buildSandbox.Stop(ctx); err != nil {
		return "", fmt.Errorf("stopping Daytona template build sandbox before capture: %w", err)
	}

	if onLog != nil {
		onLog("capturing prepared sandbox filesystem")
	}
	if err := buildSandbox.ExperimentalCreateSnapshotWithTimeout(ctx, opts.Name, 15*time.Minute); err != nil {
		return "", fmt.Errorf("capturing Daytona template snapshot: %w", err)
	}
	return opts.Name, nil
}

func nonEmptyCommands(commands []string) []string {
	nonEmpty := make([]string, 0, len(commands))
	for _, command := range commands {
		if trimmed := strings.TrimSpace(command); trimmed != "" {
			nonEmpty = append(nonEmpty, trimmed)
		}
	}
	return nonEmpty
}

func (d *Driver) DeleteTemplate(ctx context.Context, externalID string) error {
	snapshot, err := d.sdk.Snapshot.Get(ctx, externalID)
	if err != nil {
		// Treat "not found" as success — delete is idempotent.
		return nil
	}
	if snapshot.State == "building" || snapshot.State == "pending" {
		return fmt.Errorf("cannot delete snapshot while in state: %s", snapshot.State)
	}
	if err := d.sdk.Snapshot.Delete(ctx, snapshot); err != nil {
		return fmt.Errorf("deleting snapshot %s: %w", externalID, err)
	}
	return nil
}

func (d *Driver) GetTemplateStatus(ctx context.Context, externalID string) (*sandbox.TemplateBuildStatus, error) {
	snapshot, err := d.sdk.Snapshot.Get(ctx, externalID)
	if err != nil {
		return nil, fmt.Errorf("getting snapshot %s: %w", externalID, err)
	}
	result := &sandbox.TemplateBuildStatus{State: snapshot.State}
	if snapshot.ErrorReason != nil {
		result.ErrorReason = *snapshot.ErrorReason
		result.ErrorMsg = *snapshot.ErrorReason
	}
	return result, nil
}

// GetSnapshotLogs fetches build logs for an existing snapshot. Used as a
// diagnostic when a snapshot build fails. The high-level pkg/daytona SDK
// only streams build logs as part of Snapshot.Create, so we drop down to
// api-client-go's GetSnapshotBuildLogs (raw response body) here.
func (d *Driver) GetTemplateLogs(ctx context.Context, externalID string) (string, error) {
	resp, err := d.apiClient.SnapshotsAPI.
		GetSnapshotBuildLogs(d.authCtx(ctx), externalID).
		Execute()
	if err != nil {
		return "", fmt.Errorf("getting snapshot logs %s: %w", externalID, err)
	}
	if resp == nil || resp.Body == nil {
		return "", nil
	}
	defer resp.Body.Close()
	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return "", fmt.Errorf("reading snapshot logs %s: %w", externalID, readErr)
	}
	return string(body), nil
}
