package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func validateArtifact(path string) (artifactValidationResult, error) {
	artifactDir, manifestPath, manifest, _, err := loadArtifact(path)
	if err != nil {
		return artifactValidationResult{}, err
	}
	return validateLoadedArtifact(artifactDir, manifestPath, manifest)
}

func loadArtifact(path string) (string, string, canvasArtifactManifest, map[string]any, error) {
	clean := filepath.Clean(strings.TrimSpace(path))
	if clean == "." || clean == "" {
		return "", "", canvasArtifactManifest{}, nil, errors.New("artifact path is required")
	}
	info, err := os.Stat(clean)
	if err != nil {
		return "", "", canvasArtifactManifest{}, nil, err
	}
	manifestPath := clean
	artifactDir := filepath.Dir(clean)
	if info.IsDir() {
		artifactDir = clean
		manifestPath = filepath.Join(clean, "artifact.json")
	}
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return "", "", canvasArtifactManifest{}, nil, fmt.Errorf("read artifact.json: %w", err)
	}
	var manifest canvasArtifactManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return "", "", canvasArtifactManifest{}, nil, fmt.Errorf("parse artifact.json: %w", err)
	}
	var manifestObject map[string]any
	if err := json.Unmarshal(data, &manifestObject); err != nil {
		return "", "", canvasArtifactManifest{}, nil, fmt.Errorf("parse artifact.json: %w", err)
	}
	return artifactDir, manifestPath, manifest, manifestObject, nil
}

func validateLoadedArtifact(artifactDir, manifestPath string, manifest canvasArtifactManifest) (artifactValidationResult, error) {
	if manifest.SchemaVersion == 0 {
		return artifactValidationResult{}, errors.New("artifact.json missing schema_version")
	}
	if strings.TrimSpace(manifest.Name) == "" {
		return artifactValidationResult{}, errors.New("artifact.json missing name")
	}
	if strings.TrimSpace(manifest.Project) == "" {
		return artifactValidationResult{}, errors.New("artifact.json missing project")
	}
	if !supportedArtifactType(manifest.Type) {
		return artifactValidationResult{}, errors.New("artifact.json type must be web_page or presentation")
	}
	if len(manifest.Files) == 0 {
		return artifactValidationResult{}, errors.New("artifact.json files must include at least one file")
	}
	if manifest.Type == artifactTypeWebPage && strings.TrimSpace(manifest.Entrypoint) == "" {
		return artifactValidationResult{}, errors.New("web_page artifacts require entrypoint")
	}
	if manifest.Type == artifactTypePresentation && len(manifest.Slides) == 0 {
		return artifactValidationResult{}, errors.New("presentation artifacts require slides")
	}
	return validateArtifactFiles(artifactDir, manifestPath, manifest)
}
