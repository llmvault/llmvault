package agentcatalog

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func loadManifests(dir string) ([]Manifest, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := make([]Manifest, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		manifest, err := loadManifest(filepath.Join(dir, entry.Name(), "agent.json"))
		if err != nil {
			return nil, err
		}
		out = append(out, manifest)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	return out, nil
}

func loadManifest(path string) (Manifest, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("read manifest %q: %w", path, err)
	}
	var manifest Manifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("parse manifest %q: %w", path, err)
	}
	manifest.raw = append([]byte(nil), raw...)
	manifest.sourcePath = path
	manifest.dir = filepath.Dir(path)
	if err := loadInstructions(&manifest); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func loadInstructions(manifest *Manifest) error {
	path := strings.TrimSpace(manifest.Prompt.Instructions)
	if path == "" {
		return nil
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(manifest.dir, path)
	}
	raw, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return fmt.Errorf("read instructions for agent %q: %w", manifest.Slug, err)
	}
	manifest.instructions = strings.TrimSpace(string(raw))
	return nil
}

func validateManifests(manifests []Manifest) error {
	seen := map[string]bool{}
	for _, manifest := range manifests {
		if manifest.Version != 1 {
			return fmt.Errorf("agent %q has unsupported manifest version %d", manifest.Slug, manifest.Version)
		}
		if strings.TrimSpace(manifest.Slug) == "" {
			return fmt.Errorf("agent manifest %q is missing slug", manifest.sourcePath)
		}
		if strings.TrimSpace(manifest.Name) == "" {
			return fmt.Errorf("agent %q is missing name", manifest.Slug)
		}
		if seen[manifest.Slug] {
			return fmt.Errorf("duplicate agent slug %q", manifest.Slug)
		}
		seen[manifest.Slug] = true
	}
	return nil
}

func sourceHash(manifest Manifest) (string, error) {
	hash := sha256.New()
	if _, err := hash.Write(manifest.raw); err != nil {
		return "", err
	}
	if _, err := hash.Write([]byte{0}); err != nil {
		return "", err
	}
	if _, err := hash.Write([]byte(manifest.instructions)); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
