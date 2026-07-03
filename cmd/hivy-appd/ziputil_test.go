package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type zipEntry struct {
	name    string
	body    string
	mode    os.FileMode
	symlink bool
}

func makeZip(t *testing.T, entries []zipEntry) string {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range entries {
		header := &zip.FileHeader{Name: e.name, Method: zip.Deflate}
		mode := e.mode
		if mode == 0 {
			mode = 0o644
		}
		if e.symlink {
			mode |= os.ModeSymlink
		}
		header.SetMode(mode)
		w, err := zw.CreateHeader(header)
		if err != nil {
			t.Fatalf("create zip entry %q: %v", e.name, err)
		}
		if _, err := w.Write([]byte(e.body)); err != nil {
			t.Fatalf("write zip entry %q: %v", e.name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	path := filepath.Join(t.TempDir(), "bundle.zip")
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatalf("write zip file: %v", err)
	}
	return path
}

func TestExtractZipHappyPath(t *testing.T) {
	zipPath := makeZip(t, []zipEntry{
		{name: "server", body: "#!/bin/sh\necho hi\n", mode: 0o755},
		{name: "public/index.html", body: "<html></html>"},
		{name: "public/assets/", body: "", mode: os.ModeDir | 0o755},
	})
	dest := t.TempDir()
	if err := extractZip(zipPath, dest); err != nil {
		t.Fatalf("extractZip: %v", err)
	}
	info, err := os.Stat(filepath.Join(dest, "server"))
	if err != nil {
		t.Fatalf("server missing: %v", err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Errorf("server lost its exec bit: %v", info.Mode())
	}
	if _, err := os.Stat(filepath.Join(dest, "public", "index.html")); err != nil {
		t.Errorf("index.html missing: %v", err)
	}
}

func TestExtractZipRejectsTraversal(t *testing.T) {
	for _, name := range []string{"../evil", "a/../../evil", "..", "foo/../../../evil"} {
		zipPath := makeZip(t, []zipEntry{{name: name, body: "x"}})
		parent := t.TempDir()
		dest := filepath.Join(parent, "dest")
		if err := os.Mkdir(dest, 0o755); err != nil {
			t.Fatal(err)
		}
		err := extractZip(zipPath, dest)
		if err == nil {
			t.Fatalf("entry %q: expected traversal rejection, got nil", name)
		}
		if entries, _ := os.ReadDir(dest); len(entries) != 0 {
			t.Errorf("entry %q: files written inside dest despite rejection", name)
		}
		if _, statErr := os.Stat(filepath.Join(parent, "evil")); statErr == nil {
			t.Errorf("entry %q: extraction escaped the destination", name)
		}
	}
}

func TestExtractZipRejectsAbsolutePaths(t *testing.T) {
	zipPath := makeZip(t, []zipEntry{{name: "/etc/hivy-evil", body: "x"}})
	if err := extractZip(zipPath, t.TempDir()); err == nil {
		t.Fatal("expected absolute path rejection, got nil")
	}
}

func TestExtractZipRejectsSymlinks(t *testing.T) {
	zipPath := makeZip(t, []zipEntry{
		{name: "link", body: "/etc/passwd", symlink: true, mode: 0o777},
	})
	err := extractZip(zipPath, t.TempDir())
	if err == nil {
		t.Fatal("expected symlink rejection, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported archive entry type") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestExtractZipRejectsTooManyEntries(t *testing.T) {
	entries := make([]zipEntry, maxZipEntries+1)
	for i := range entries {
		entries[i] = zipEntry{name: fmt.Sprintf("many/f%05d", i), body: ""}
	}
	zipPath := makeZip(t, entries)
	err := extractZip(zipPath, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "entries; max") {
		t.Fatalf("expected entry count rejection, got %v", err)
	}
}

func TestExtractZipEnforcesSizeBudget(t *testing.T) {
	// Exercise the byte budget via writeZipFile's remaining parameter.
	zipPath := makeZip(t, []zipEntry{{name: "big", body: strings.Repeat("A", 4096)}})
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	defer reader.Close()
	_, err = writeZipFile(reader.File[0], filepath.Join(t.TempDir(), "big"), 1024)
	if err == nil || !strings.Contains(err.Error(), "max uncompressed size") {
		t.Fatalf("expected size budget rejection, got %v", err)
	}
}
