package main

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// extractZip safely extracts a zip archive into dest. Safety properties
// (mirrors cmd/agent-debug-pack/archive.go for tarballs):
//   - path traversal rejected: "..", absolute paths, and any entry that
//     resolves outside dest
//   - symlinks and other non-regular entries rejected
//   - entry count and total uncompressed size are capped
func extractZip(zipPath, dest string) error {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}
	defer reader.Close()

	dest, err = filepath.Abs(dest)
	if err != nil {
		return err
	}
	if len(reader.File) > maxZipEntries {
		return fmt.Errorf("zip has %d entries; max %d", len(reader.File), maxZipEntries)
	}

	var totalBytes int64
	for _, entry := range reader.File {
		remaining := maxUnpackedBytes - totalBytes
		written, err := extractZipEntry(entry, dest, remaining)
		if err != nil {
			return err
		}
		totalBytes += written
	}
	return nil
}

func extractZipEntry(entry *zip.File, dest string, remaining int64) (int64, error) {
	name := path.Clean(strings.ReplaceAll(entry.Name, `\`, "/"))
	if name == "." {
		return 0, nil
	}
	if strings.HasPrefix(name, "../") || path.IsAbs(name) {
		return 0, fmt.Errorf("unsafe archive path %q", entry.Name)
	}
	target := filepath.Join(dest, filepath.FromSlash(name))
	if !isWithinDir(dest, target) {
		return 0, fmt.Errorf("archive path escapes destination: %q", entry.Name)
	}

	mode := entry.Mode()
	switch {
	case mode.IsDir():
		return 0, os.MkdirAll(target, 0o755)
	case mode.IsRegular():
		return writeZipFile(entry, target, remaining)
	default:
		// Symlinks, devices, fifos: never allowed from an archive.
		return 0, fmt.Errorf("unsupported archive entry type for %q (mode %s)", entry.Name, mode)
	}
}

func writeZipFile(entry *zip.File, target string, remaining int64) (int64, error) {
	if remaining <= 0 {
		return 0, fmt.Errorf("zip exceeds max uncompressed size of %d bytes", int64(maxUnpackedBytes))
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return 0, err
	}
	src, err := entry.Open()
	if err != nil {
		return 0, err
	}
	defer src.Close()

	perm := os.FileMode(0o644)
	if entry.Mode().Perm()&0o111 != 0 {
		perm = 0o755
	}
	dst, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return 0, err
	}
	// Copy at most remaining+1 bytes so an oversized archive is detected
	// without unbounded writes.
	written, err := io.Copy(dst, io.LimitReader(src, remaining+1))
	if err != nil {
		_ = dst.Close()
		return written, err
	}
	if err := dst.Close(); err != nil {
		return written, err
	}
	if written > remaining {
		return written, fmt.Errorf("zip exceeds max uncompressed size of %d bytes", int64(maxUnpackedBytes))
	}
	return written, nil
}

func isWithinDir(parent, target string) bool {
	parent = filepath.Clean(parent)
	target = filepath.Clean(target)
	rel, err := filepath.Rel(parent, target)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
