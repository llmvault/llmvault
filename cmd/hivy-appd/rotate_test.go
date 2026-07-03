package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRotateIfNeededTriggersAtThreshold(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")

	// Below threshold: no rotation.
	if err := os.WriteFile(path, []byte("small\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	rotated, err := rotateIfNeeded(path, 100, 3)
	if err != nil || rotated {
		t.Fatalf("below threshold: rotated=%v err=%v, want no rotation", rotated, err)
	}

	// At/over threshold: rotate into .1 and truncate the live file.
	content := strings.Repeat("x", 150) + "\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	rotated, err = rotateIfNeeded(path, 100, 3)
	if err != nil || !rotated {
		t.Fatalf("over threshold: rotated=%v err=%v, want rotation", rotated, err)
	}
	moved, err := os.ReadFile(path + ".1")
	if err != nil || string(moved) != content {
		t.Errorf("rotated content mismatch (err %v)", err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Size() != 0 {
		t.Errorf("live file not truncated: size=%d err=%v", info.Size(), err)
	}
}

func TestRotateIfNeededShiftsGenerationsAndDropsOldest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")
	big := strings.Repeat("y", 200)

	writeAll := func(gen string, content string) {
		name := path
		if gen != "" {
			name = path + "." + gen
		}
		if err := os.WriteFile(name, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeAll("", big)
	writeAll("1", "gen1")
	writeAll("2", "gen2")
	writeAll("3", "gen3")

	rotated, err := rotateIfNeeded(path, 100, 3)
	if err != nil || !rotated {
		t.Fatalf("rotated=%v err=%v", rotated, err)
	}
	for gen, want := range map[string]string{"1": big, "2": "gen1", "3": "gen2"} {
		data, err := os.ReadFile(path + "." + gen)
		if err != nil || string(data) != want {
			t.Errorf("gen %s = %q (err %v), want %q", gen, data, err, want)
		}
	}
	// The old .3 was overwritten (dropped), no .4 appears.
	if _, err := os.Stat(path + ".4"); err == nil {
		t.Error("unexpected .4 generation created")
	}
}

// TestRotationPreservesOpenAppendWriters proves the copy-truncate strategy:
// a writer holding the file open in append mode keeps logging into the live
// file after rotation, with no lost fd.
func TestRotationPreservesOpenAppendWriters(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	if _, err := f.WriteString(strings.Repeat("z", 128) + "\n"); err != nil {
		t.Fatal(err)
	}
	rotated, err := rotateIfNeeded(path, 100, 3)
	if err != nil || !rotated {
		t.Fatalf("rotated=%v err=%v", rotated, err)
	}
	if _, err := f.WriteString("after rotation\n"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "after rotation\n" {
		t.Errorf("live file = %q, want only post-rotation content", data)
	}
}
