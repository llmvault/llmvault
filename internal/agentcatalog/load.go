package agentcatalog

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/registry"
)

var subAgentKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)

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
	if err := loadSubAgentInstructions(&manifest); err != nil {
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

func loadSubAgentInstructions(manifest *Manifest) error {
	if len(manifest.SubAgents) == 0 {
		return nil
	}
	keys := make([]string, 0, len(manifest.SubAgents))
	for key := range manifest.SubAgents {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		subAgent := manifest.SubAgents[key]
		path := strings.TrimSpace(subAgent.Prompt.Instructions)
		if path == "" {
			manifest.SubAgents[key] = subAgent
			continue
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(manifest.dir, path)
		}
		raw, err := os.ReadFile(filepath.Clean(path))
		if err != nil {
			return fmt.Errorf("read instructions for subagent %q on agent %q: %w", key, manifest.Slug, err)
		}
		subAgent.instructions = strings.TrimSpace(string(raw))
		manifest.SubAgents[key] = subAgent
	}
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
		if !model.ValidSandboxImage(manifest.Runtime.SandboxImage) {
			return fmt.Errorf("agent %q has invalid runtime sandbox_image %q", manifest.Slug, manifest.Runtime.SandboxImage)
		}
		if err := validateToolSelection(manifest.Slug, "tools", manifest.Tools); err != nil {
			return err
		}
		if err := validateSubAgents(manifest); err != nil {
			return err
		}
		seen[manifest.Slug] = true
	}
	return nil
}

func validateSubAgents(manifest Manifest) error {
	keys := make([]string, 0, len(manifest.SubAgents))
	for key := range manifest.SubAgents {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if !subAgentKeyPattern.MatchString(key) {
			return fmt.Errorf("agent %q has invalid subagent key %q", manifest.Slug, key)
		}
		subAgent := manifest.SubAgents[key]
		if strings.TrimSpace(subAgent.Name) == "" {
			return fmt.Errorf("agent %q subagent %q is missing name", manifest.Slug, key)
		}
		if err := validateToolSelection(manifest.Slug, "subagent "+key+" tools", subAgent.Tools); err != nil {
			return err
		}
		modelID := strings.TrimSpace(subAgent.Model)
		if modelID != "" {
			if err := registry.Global().ValidateCanonicalModel(modelID); err != nil {
				return fmt.Errorf("agent %q subagent %q model: %w", manifest.Slug, key, err)
			}
		}
	}
	return nil
}

func validateToolSelection(agent, path string, tools map[string]any) error {
	keys := make([]string, 0, len(tools))
	for key := range tools {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	seen := map[string]bool{}
	for _, key := range keys {
		cleanKey := strings.TrimSpace(key)
		if cleanKey == "" {
			return fmt.Errorf("agent %q %s includes an empty tool key", agent, path)
		}
		if seen[cleanKey] {
			return fmt.Errorf("agent %q %s includes duplicate tool key %q after trimming", agent, path, cleanKey)
		}
		seen[cleanKey] = true
		if !model.IsValidRuntimeBuiltInToolID(cleanKey) {
			return fmt.Errorf("agent %q %s includes unknown runtime tool %q", agent, path, cleanKey)
		}
		switch tools[key].(type) {
		case nil, bool, map[string]any:
		default:
			return fmt.Errorf("agent %q %s tool %q must be a boolean or config object", agent, path, cleanKey)
		}
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
	keys := make([]string, 0, len(manifest.SubAgents))
	for key := range manifest.SubAgents {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if _, err := hash.Write([]byte{0}); err != nil {
			return "", err
		}
		if _, err := hash.Write([]byte(key)); err != nil {
			return "", err
		}
		if _, err := hash.Write([]byte{0}); err != nil {
			return "", err
		}
		if _, err := hash.Write([]byte(manifest.SubAgents[key].instructions)); err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
