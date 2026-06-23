package docker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"strings"

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

func (d *Driver) prepareUpgradeWorkspace(ctx context.Context, externalID string, info container.InspectResponse, targetName string, imageRef string, labels map[string]string) (string, error) {
	if volumeName := workspaceVolumeFromInspect(info); volumeName != "" {
		return volumeName, nil
	}
	volumeName := workspaceVolumeName(targetName)
	if err := d.ensureWorkspaceVolume(ctx, volumeName, labels); err != nil {
		return "", err
	}
	if err := d.copyWorkspaceToVolume(ctx, externalID, imageRef, volumeName, labels); err != nil {
		_ = d.removeWorkspaceVolume(context.WithoutCancel(ctx), volumeName)
		return "", err
	}
	return volumeName, nil
}

func (d *Driver) copyWorkspaceToVolume(ctx context.Context, externalID string, imageRef string, volumeName string, labels map[string]string) error {
	archive, _, err := d.cli.CopyFromContainer(ctx, externalID, workspaceMountPath)
	if err != nil {
		if isWorkspaceCopyNotFound(err) {
			return nil
		}
		return fmt.Errorf("copying workspace from docker container %s: %w", externalID, err)
	}
	defer archive.Close()

	helper, err := d.createDockerContainer(ctx, dockerContainerSpec{
		Name:            containerName("hivy-workspace-copy"),
		ImageRef:        imageRef,
		Labels:          labels,
		WorkspaceVolume: volumeName,
	})
	if err != nil {
		return fmt.Errorf("creating docker workspace copy helper: %w", err)
	}
	defer func() {
		_ = d.cli.ContainerRemove(context.WithoutCancel(ctx), helper.ID, container.RemoveOptions{Force: true})
	}()

	if err := d.cli.CopyToContainer(ctx, helper.ID, "/", archive, container.CopyToContainerOptions{AllowOverwriteDirWithFile: true}); err != nil && err != io.EOF {
		return fmt.Errorf("copying workspace into docker volume %s: %w", volumeName, err)
	}
	return nil
}

func isWorkspaceCopyNotFound(err error) bool {
	if cerrdefs.IsNotFound(err) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "could not find the file") || strings.Contains(msg, "no such file")
}
