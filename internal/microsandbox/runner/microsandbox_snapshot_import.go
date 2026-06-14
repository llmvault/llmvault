package runner

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	microsandbox "github.com/superradcompany/microsandbox/sdk/go"

	"github.com/usehivy/hivy/internal/microsandbox/storage"
)

func (m *MicrosandboxBackend) ensureSnapshotAvailable(ctx context.Context, req CreateSandboxRequest) error {
	if _, err := microsandbox.Snapshot.Get(ctx, req.SnapshotID); err == nil {
		return nil
	}
	if req.SnapshotArtifactURL == "" {
		return fmt.Errorf("snapshot %s is not local and no artifact URL was provided", req.SnapshotID)
	}
	if m.store == nil {
		return fmt.Errorf("snapshot %s is not local and snapshot storage is not configured", req.SnapshotID)
	}
	if err := m.ensureImageAvailable(ctx, req.ImageRef, req.SnapshotImageDigest); err != nil {
		return err
	}
	if err := os.MkdirAll(snapshotExportDir, 0o755); err != nil {
		return err
	}
	archivePath := filepath.Join(snapshotExportDir, req.SnapshotID+"-import"+snapshotArchiveExtension(req.SnapshotArtifactURL))
	defer func() {
		_ = os.Remove(archivePath)
	}()
	if err := m.store.Download(ctx, req.SnapshotArtifactURL, archivePath); err != nil {
		return fmt.Errorf("download snapshot artifact: %w", err)
	}
	if req.SnapshotArtifactDigest != "" {
		got, err := storage.FileSHA256(archivePath)
		if err != nil {
			return err
		}
		if got != req.SnapshotArtifactDigest {
			return fmt.Errorf("snapshot artifact digest mismatch: got %s want %s", got, req.SnapshotArtifactDigest)
		}
	}
	if _, err := microsandbox.Snapshot.Import(ctx, archivePath, ""); err != nil {
		return fmt.Errorf("import snapshot artifact: %w", err)
	}
	if _, err := microsandbox.Snapshot.Get(ctx, req.SnapshotID); err != nil {
		return fmt.Errorf("snapshot %s imported but was not indexed by name: %w", req.SnapshotID, err)
	}
	return nil
}

func (m *MicrosandboxBackend) ensureImageAvailable(ctx context.Context, imageRef, imageDigest string) error {
	if imageRef == "" {
		return nil
	}
	ref := pinnedImageRef(imageRef, imageDigest)
	if _, err := microsandbox.Image.Get(ctx, ref); err == nil {
		return nil
	}
	seedName := fmt.Sprintf("hivy-image-seed-%d", time.Now().UnixNano())
	sb, err := microsandbox.CreateSandbox(ctx, seedName,
		microsandbox.WithImage(ref),
		microsandbox.WithCPUs(1),
		microsandbox.WithMemory(1024),
		microsandbox.WithDetached(),
	)
	if err != nil {
		return fmt.Errorf("prewarm snapshot base image %s: %w", ref, err)
	}
	_ = sb.Stop(ctx)
	_ = sb.Close()
	_ = microsandbox.RemoveSandbox(context.WithoutCancel(ctx), seedName)
	return nil
}

func pinnedImageRef(imageRef, digest string) string {
	if imageRef == "" || digest == "" || strings.Contains(imageRef, "@") {
		return imageRef
	}
	lastSlash := strings.LastIndex(imageRef, "/")
	lastColon := strings.LastIndex(imageRef, ":")
	name := imageRef
	if lastColon > lastSlash {
		name = imageRef[:lastColon]
	}
	return name + "@" + digest
}

func snapshotArchiveExtension(artifactURL string) string {
	parsed, err := url.Parse(artifactURL)
	path := artifactURL
	if err == nil {
		path = parsed.Path
	}
	if strings.HasSuffix(path, ".tar.zst") {
		return ".tar.zst"
	}
	if ext := filepath.Ext(path); ext != "" {
		return ext
	}
	return ".tar.zst"
}
