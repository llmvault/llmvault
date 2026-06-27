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
	result := artifactValidationResult{
		Valid:        true,
		ArtifactPath: artifactDir,
		ManifestPath: manifestPath,
		Type:         manifest.Type,
		Name:         manifest.Name,
		Files:        make([]string, 0, len(manifest.Files)),
	}
	addManifestIssue := func(code, message, fix string) {
		result.Issues = append(result.Issues, artifactValidationIssue{
			Code:     code,
			Severity: "error",
			File:     manifestPath,
			Message:  message,
			Fix:      fix,
		})
	}
	if manifest.SchemaVersion == 0 {
		addManifestIssue("manifest_missing_schema_version", "artifact.json is missing schema_version.", "Set schema_version to 1.")
	}
	if strings.TrimSpace(manifest.Name) == "" {
		addManifestIssue("manifest_missing_name", "artifact.json is missing name.", "Add a human-readable artifact name.")
	}
	if strings.TrimSpace(manifest.Project) == "" {
		addManifestIssue("manifest_missing_project", "artifact.json is missing project.", "Set project to the canvas project slug or ID.")
	}
	if !supportedArtifactType(manifest.Type) {
		addManifestIssue("manifest_invalid_type", "artifact.json type must be web_page or presentation.", "Set type to web_page or presentation.")
	}
	if len(manifest.Files) == 0 {
		addManifestIssue("manifest_missing_files", "artifact.json files must include at least one file.", "Add at least one HTML file entry to files.")
	}
	if manifest.Type == artifactTypeWebPage && strings.TrimSpace(manifest.Entrypoint) == "" {
		addManifestIssue("manifest_missing_entrypoint", "web_page artifacts require entrypoint.", "Set entrypoint to the primary HTML file, usually index.html.")
	}
	if manifest.Type == artifactTypePresentation && len(manifest.Slides) == 0 {
		addManifestIssue("manifest_missing_slides", "presentation artifacts require slides.", "Add slides with stable slide IDs and HTML paths.")
	}
	result = validateArtifactFiles(artifactDir, manifestPath, manifest, result)
	if hasArtifactValidationErrors(result.Issues) {
		result.Valid = false
		result.Guidance = artifactValidationGuidance()
		return result, artifactValidationError{Result: result}
	}
	return result, nil
}
