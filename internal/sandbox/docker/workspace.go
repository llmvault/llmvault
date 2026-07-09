package docker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/volume"
)

const workspaceMountPath = "/workspace"

func workspaceVolumeName(key string) string {
	sum := sha256.Sum256([]byte(key))
	return "hivy-workspace-" + hex.EncodeToString(sum[:8])
}

func (d *Driver) ensureWorkspaceVolume(ctx context.Context, name string, labels map[string]string) error {
	volumeLabels := map[string]string{
		"purpose": "workspace",
	}
	for key, value := range labels {
		volumeLabels[key] = value
	}
	_, err := d.cli.VolumeCreate(ctx, volume.CreateOptions{
		Name:   name,
		Labels: d.labels(volumeLabels),
	})
	if err != nil {
		return fmt.Errorf("creating docker workspace volume %s: %w", name, err)
	}
	return nil
}

func (d *Driver) removeWorkspaceVolume(ctx context.Context, name string) error {
	err := d.cli.VolumeRemove(ctx, name, true)
	if err != nil && !cerrdefs.IsNotFound(err) {
		return fmt.Errorf("removing docker workspace volume %s: %w", name, err)
	}
	return nil
}

func workspaceVolumeFromInspect(info container.InspectResponse) string {
	for _, mount := range info.Mounts {
		if mount.Destination == workspaceMountPath && mount.Name != "" {
			return mount.Name
		}
	}
	return ""
}
