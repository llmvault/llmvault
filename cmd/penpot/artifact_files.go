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

	"golang.org/x/net/html"
)

func validateArtifactFiles(artifactDir, manifestPath string, manifest canvasArtifactManifest) (artifactValidationResult, error) {
	result := artifactValidationResult{
		Valid:        true,
		ArtifactPath: artifactDir,
		ManifestPath: manifestPath,
		Type:         manifest.Type,
		Name:         manifest.Name,
		Files:        make([]string, 0, len(manifest.Files)),
	}
	htmlFiles := 0
	for _, file := range manifest.Files {
		rel, err := cleanArtifactRelPath(file.Path)
		if err != nil {
			return artifactValidationResult{}, err
		}
		fullPath := filepath.Join(artifactDir, rel)
		info, err := os.Stat(fullPath)
		if err != nil {
			return artifactValidationResult{}, fmt.Errorf("artifact file %q: %w", rel, err)
		}
		if info.IsDir() {
			return artifactValidationResult{}, fmt.Errorf("artifact file %q is a directory", rel)
		}
		result.Files = append(result.Files, rel)
		if isHTMLArtifactFile(file) {
			htmlFiles++
			if err := validateHTMLFile(fullPath); err != nil {
				return artifactValidationResult{}, fmt.Errorf("%s: %w", rel, err)
			}
		}
	}
	if htmlFiles == 0 {
		return artifactValidationResult{}, errors.New("artifact files must include at least one HTML file")
	}
	return result, nil
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

func validateHTMLFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	doc, err := html.Parse(strings.NewReader(string(data)))
	if err != nil {
		return fmt.Errorf("invalid HTML: %w", err)
	}
	if !containsElement(doc, "html") || !containsElement(doc, "body") {
		return errors.New("HTML must include html and body elements")
	}
	if !containsDataHivyID(doc) {
		return errors.New("HTML must include at least one data-hivy-id attribute")
	}
	return nil
}

func containsElement(node *html.Node, name string) bool {
	if node.Type == html.ElementNode && node.Data == name {
		return true
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if containsElement(child, name) {
			return true
		}
	}
	return false
}

func containsDataHivyID(node *html.Node) bool {
	if node.Type == html.ElementNode {
		for _, attr := range node.Attr {
			if attr.Key == "data-hivy-id" && strings.TrimSpace(attr.Val) != "" {
				return true
			}
		}
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if containsDataHivyID(child) {
			return true
		}
	}
	return false
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
