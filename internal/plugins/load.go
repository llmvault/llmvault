package plugins

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/usehivy/hivy/internal/model"
	skillpkg "github.com/usehivy/hivy/internal/skills"
)

func loadManifests(root string) ([]Manifest, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("read plugin dir %q: %w", root, err)
	}
	out := make([]Manifest, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(root, entry.Name())
		path := filepath.Join(dir, "plugin.json")
		body, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		var manifest Manifest
		if err := json.Unmarshal(body, &manifest); err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		manifest.raw = body
		manifest.sourcePath = path
		manifest.dir = dir
		if manifest.Slug == "" {
			manifest.Slug = entry.Name()
		}
		out = append(out, manifest)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	return out, nil
}

func validateManifests(manifests []Manifest) error {
	seen := map[string]string{}
	for _, manifest := range manifests {
		if manifest.Version != 1 {
			return fmt.Errorf("%s: unsupported version %d", manifest.sourcePath, manifest.Version)
		}
		for field, value := range map[string]string{
			"slug": manifest.Slug,
			"name": manifest.Name,
		} {
			if strings.TrimSpace(value) == "" {
				return fmt.Errorf("%s: %s is required", manifest.sourcePath, field)
			}
		}
		if prior, ok := seen[manifest.Slug]; ok {
			return fmt.Errorf("duplicate plugin slug %q in %s and %s", manifest.Slug, prior, manifest.sourcePath)
		}
		seen[manifest.Slug] = manifest.sourcePath
		for _, req := range manifest.RequiredConnections {
			if strings.TrimSpace(req.Provider) == "" {
				return fmt.Errorf("%s: required connection provider is required", manifest.sourcePath)
			}
			kind := strings.TrimSpace(req.Kind)
			if kind == "" {
				continue
			}
			if kind != model.PluginIntegrationKindIntegration && kind != model.PluginIntegrationKindDatabase {
				return fmt.Errorf("%s: unsupported connection kind %q", manifest.sourcePath, kind)
			}
		}
	}
	return nil
}

func loadSkills(ctx context.Context, manifest Manifest) ([]loadedSkill, error) {
	skillsDir := filepath.Join(manifest.dir, "skills")
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return nil, fmt.Errorf("read plugin skills dir %s: %w", skillsDir, err)
	}
	out := make([]loadedSkill, 0, len(entries))
	seen := map[string]string{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(skillsDir, entry.Name())
		item, err := loadSkill(ctx, dir, entry.Name())
		if err != nil {
			return nil, err
		}
		if prior, ok := seen[item.Slug]; ok {
			return nil, fmt.Errorf("duplicate skill slug %q in %s and %s", item.Slug, prior, dir)
		}
		seen[item.Slug] = dir
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	return out, nil
}

func loadSkill(ctx context.Context, dir, fallbackSlug string) (loadedSkill, error) {
	path := filepath.Join(dir, "skill.json")
	body, err := os.ReadFile(path)
	if err != nil {
		return loadedSkill{}, fmt.Errorf("read %s: %w", path, err)
	}
	var manifest skillManifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		return loadedSkill{}, fmt.Errorf("parse %s: %w", path, err)
	}
	if strings.TrimSpace(manifest.Name) == "" {
		return loadedSkill{}, fmt.Errorf("%s: name is required", path)
	}
	root := manifest.Root
	if strings.TrimSpace(root) == "" {
		root = "./SKILL.md"
	}
	rootPath := filepath.Join(dir, strings.TrimPrefix(root, "./"))
	content, err := os.ReadFile(rootPath)
	if err != nil {
		return loadedSkill{}, fmt.Errorf("read skill root %s: %w", rootPath, err)
	}
	files := map[string]string{}
	for _, file := range manifest.Files {
		if strings.TrimSpace(file.Path) == "" {
			return loadedSkill{}, fmt.Errorf("%s: file path is required", path)
		}
		body, err := loadSkillFile(ctx, dir, file)
		if err != nil {
			return loadedSkill{}, err
		}
		files[file.Path] = body
	}
	slug := model.GenerateSlug(manifest.Name)
	if slug == "" {
		slug = model.GenerateSlug(fallbackSlug)
	}
	return loadedSkill{
		Manifest: manifest,
		Slug:     slug,
		Dir:      dir,
		Content:  string(content),
		Files:    files,
	}, nil
}

func loadSkillFile(ctx context.Context, dir string, file skillManifestFile) (string, error) {
	if strings.TrimSpace(file.URL) != "" {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, file.URL, nil)
		if err != nil {
			return "", err
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return "", fmt.Errorf("fetch skill file %s: %w", file.URL, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return "", fmt.Errorf("fetch skill file %s: status %d", file.URL, resp.StatusCode)
		}
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", fmt.Errorf("read skill file %s: %w", file.URL, err)
		}
		return string(body), nil
	}
	body, err := os.ReadFile(filepath.Join(dir, file.Path))
	if err != nil {
		return "", fmt.Errorf("read skill file %s: %w", file.Path, err)
	}
	return string(body), nil
}

func sourceHash(manifest Manifest, skills []loadedSkill) (string, error) {
	type hashSkill struct {
		Manifest skillManifest     `json:"manifest"`
		Content  string            `json:"content"`
		Files    map[string]string `json:"files"`
	}
	payload := struct {
		Manifest json.RawMessage `json:"manifest"`
		Skills   []hashSkill     `json:"skills"`
	}{
		Manifest: manifest.raw,
	}
	for _, skill := range skills {
		payload.Skills = append(payload.Skills, hashSkill{
			Manifest: skill.Manifest,
			Content:  skill.Content,
			Files:    skill.Files,
		})
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:]), nil
}

func resolveDir(dir string) (string, error) {
	if stat, err := os.Stat(dir); err == nil && stat.IsDir() {
		return dir, nil
	}
	if filepath.IsAbs(dir) {
		return "", os.ErrNotExist
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get cwd: %w", err)
	}
	for {
		candidate := filepath.Join(cwd, dir)
		if stat, err := os.Stat(candidate); err == nil && stat.IsDir() {
			return candidate, nil
		}
		parent := filepath.Dir(cwd)
		if parent == cwd {
			break
		}
		cwd = parent
	}
	return "", os.ErrNotExist
}

func referencesFromFiles(files map[string]string) []skillpkg.Reference {
	if len(files) == 0 {
		return nil
	}
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	refs := make([]skillpkg.Reference, 0, len(paths))
	for _, path := range paths {
		refs = append(refs, skillpkg.Reference{Path: path, Body: files[path]})
	}
	return refs
}
