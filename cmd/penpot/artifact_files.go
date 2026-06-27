package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

func validateArtifactFiles(artifactDir, manifestPath string, manifest canvasArtifactManifest, result artifactValidationResult) artifactValidationResult {
	htmlFiles := 0
	for _, file := range manifest.Files {
		rel, err := cleanArtifactRelPath(file.Path)
		if err != nil {
			result.Issues = append(result.Issues, artifactValidationIssue{
				Code:     "artifact_file_invalid_path",
				Severity: "error",
				File:     manifestPath,
				Message:  err.Error(),
				Fix:      "Use a relative file path that stays inside the artifact directory.",
			})
			continue
		}
		fullPath := filepath.Join(artifactDir, rel)
		info, err := os.Stat(fullPath)
		if err != nil {
			result.Issues = append(result.Issues, artifactValidationIssue{
				Code:     "artifact_file_missing",
				Severity: "error",
				File:     rel,
				Message:  fmt.Sprintf("artifact file %q is listed in artifact.json but was not found: %v", rel, err),
				Fix:      "Create the file or remove it from artifact.json files.",
			})
			continue
		}
		if info.IsDir() {
			result.Issues = append(result.Issues, artifactValidationIssue{
				Code:     "artifact_file_is_directory",
				Severity: "error",
				File:     rel,
				Message:  fmt.Sprintf("artifact file %q is a directory.", rel),
				Fix:      "Point artifact.json files to an HTML file, not a directory.",
			})
			continue
		}
		result.Files = append(result.Files, rel)
		if isHTMLArtifactFile(file) {
			htmlFiles++
			report, issues := validateHTMLFile(rel, fullPath, manifest.Type)
			result.HTML = append(result.HTML, report)
			result.Issues = append(result.Issues, issues...)
		}
	}
	if htmlFiles == 0 {
		result.Issues = append(result.Issues, artifactValidationIssue{
			Code:     "artifact_missing_html_file",
			Severity: "error",
			File:     manifestPath,
			Message:  "artifact files must include at least one HTML file.",
			Fix:      "Add an .html file to artifact.json files and make it the web_page entrypoint or a presentation slide.",
		})
	}
	return result
}

func buildArtifactSyncPayload(artifactDir string, manifest canvasArtifactManifest, manifestObject map[string]any) (artifactSyncPayload, error) {
	files := make([]artifactFilePayload, 0, len(manifest.Files))
	for _, file := range manifest.Files {
		rel, err := cleanArtifactRelPath(file.Path)
		if err != nil {
			return artifactSyncPayload{}, err
		}
		data, err := os.ReadFile(filepath.Join(artifactDir, rel))
		if err != nil {
			return artifactSyncPayload{}, fmt.Errorf("read artifact file %q: %w", rel, err)
		}
		sum := sha256.Sum256(data)
		contentType := strings.TrimSpace(file.ContentType)
		if contentType == "" {
			contentType = detectContentType(rel)
		}
		files = append(files, artifactFilePayload{
			Path:        rel,
			Role:        strings.TrimSpace(file.Role),
			ContentType: contentType,
			SizeBytes:   int64(len(data)),
			SHA256:      hex.EncodeToString(sum[:]),
			Content:     string(data),
		})
	}
	project := artifactProjectPayload{Ref: strings.TrimSpace(manifest.Project)}
	if looksLikeUUID(project.Ref) {
		project.ID = project.Ref
	} else {
		project.Slug = project.Ref
	}
	return artifactSyncPayload{
		SessionID: strings.TrimSpace(os.Getenv("HIVY_SESSION_ID")),
		Project:   project,
		Artifact: artifactDetailsPayload{
			Slug:     strings.TrimSpace(manifest.Slug),
			Name:     strings.TrimSpace(manifest.Name),
			Type:     strings.TrimSpace(manifest.Type),
			Manifest: manifestObject,
		},
		Files: files,
	}, nil
}

func cleanArtifactRelPath(path string) (string, error) {
	rel := filepath.Clean(strings.TrimSpace(path))
	if rel == "." || rel == "" {
		return "", errors.New("artifact file path is required")
	}
	if filepath.IsAbs(rel) || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
		return "", fmt.Errorf("artifact file path %q must stay inside the artifact directory", path)
	}
	return rel, nil
}

func isHTMLArtifactFile(file canvasArtifactFile) bool {
	contentType := strings.ToLower(strings.TrimSpace(file.ContentType))
	if strings.Contains(contentType, "html") {
		return true
	}
	ext := strings.ToLower(filepath.Ext(file.Path))
	return ext == ".html" || ext == ".htm"
}

func detectContentType(path string) string {
	if value := mime.TypeByExtension(filepath.Ext(path)); value != "" {
		return value
	}
	if strings.EqualFold(filepath.Ext(path), ".html") {
		return "text/html"
	}
	return "application/octet-stream"
}

func supportedArtifactType(value string) bool {
	switch strings.TrimSpace(value) {
	case artifactTypeWebPage, artifactTypePresentation:
		return true
	default:
		return false
	}
}

var slugPattern = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(value string) string {
	slug := strings.Trim(slugPattern.ReplaceAllString(strings.ToLower(strings.TrimSpace(value)), "-"), "-")
	if slug == "" {
		return "artifact"
	}
	return slug
}

func looksLikeUUID(value string) bool {
	if len(value) != 36 {
		return false
	}
	for i, char := range value {
		switch i {
		case 8, 13, 18, 23:
			if char != '-' {
				return false
			}
		default:
			if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f') || (char >= 'A' && char <= 'F')) {
				return false
			}
		}
	}
	return true
}
