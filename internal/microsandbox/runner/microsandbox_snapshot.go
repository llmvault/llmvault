package runner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	microsandbox "github.com/superradcompany/microsandbox/sdk/go"
)

const snapshotExportDir = "/var/lib/hivy/microsandbox-snapshots"

func (m *MicrosandboxBackend) CreateSnapshot(ctx context.Context, req CreateSnapshotRequest) (*CreateSnapshotResponse, error) {
	sbReq := CreateSandboxRequest{
		ID: req.ID, Name: req.Name, ImageRef: req.BaseImageRef, CPU: req.CPU,
		MemoryMB: req.MemoryMB, DiskGB: req.DiskGB, Env: req.Env,
	}
	if _, err := m.CreateSandbox(ctx, sbReq); err != nil {
		return nil, err
	}
	defer func() {
		_ = m.DeleteSandbox(context.WithoutCancel(ctx), req.ID)
	}()
	var logs strings.Builder
	for _, cmd := range req.Commands {
		out, err := m.Exec(ctx, req.ID, cmd, 0)
		if out != nil {
			logs.WriteString(out.Stdout)
			logs.WriteString(out.Stderr)
		}
		if err != nil {
			return nil, fmt.Errorf("snapshot command failed %q: %w", cmd, err)
		}
		if out != nil && out.ExitCode != 0 {
			return nil, fmt.Errorf("snapshot command failed %q with exit code %d", cmd, out.ExitCode)
		}
	}
	if err := m.StopSandbox(ctx, req.ID); err != nil {
		return nil, err
	}
	snapshot, err := microsandbox.Snapshot.Create(ctx, req.ID, microsandbox.SnapshotCreateOptions{
		Name:            req.ID,
		Force:           true,
		RecordIntegrity: true,
		Labels:          map[string]string{"snapshot_id": req.ID, "name": req.Name},
	})
	if err != nil {
		return nil, err
	}
	if _, err := snapshot.Verify(ctx); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(snapshotExportDir, 0o755); err != nil {
		return nil, err
	}
	exportPath := filepath.Join(snapshotExportDir, req.ID+".tar")
	defer func() {
		_ = os.Remove(exportPath)
	}()
	if err := microsandbox.Snapshot.Export(ctx, snapshot.Path(), exportPath, microsandbox.SnapshotExportOptions{
		WithParents: true,
		WithImage:   true,
		PlainTar:    true,
	}); err != nil {
		return nil, err
	}
	artifactURL, err := m.store.Upload(ctx, exportPath, req.ID)
	if err != nil {
		return nil, err
	}
	return &CreateSnapshotResponse{ID: req.ID, ArtifactURL: artifactURL, Logs: logs.String()}, nil
}

func (m *MicrosandboxBackend) DeleteSnapshot(ctx context.Context, snapshotID string) error {
	return microsandbox.Snapshot.Remove(ctx, snapshotID, true)
}
