package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestFileArtifactUploadReportsDigestAndDownloadCopies(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "snapshot.tar.zst")
	if err := os.WriteFile(src, []byte("snapshot data"), 0o600); err != nil {
		t.Fatal(err)
	}

	var store *SnapshotStore
	artifact, err := store.Upload(context.Background(), src, "snp12345")
	if err != nil {
		t.Fatal(err)
	}
	if artifact.SizeBytes != int64(len("snapshot data")) {
		t.Fatalf("SizeBytes = %d", artifact.SizeBytes)
	}
	if artifact.ContentType != "application/zstd" {
		t.Fatalf("ContentType = %q", artifact.ContentType)
	}
	if artifact.Digest == "" {
		t.Fatal("Digest is empty")
	}

	dst := filepath.Join(dir, "downloaded.tar.zst")
	if err := store.Download(context.Background(), artifact.URL, dst); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "snapshot data" {
		t.Fatalf("downloaded data = %q", got)
	}
}
