package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func artifactCommand(args []string) error {
	if len(args) == 0 {
		return errors.New("artifact subcommand is required")
	}
	switch args[0] {
	case "create":
		return artifactCreateCommand(args[1:])
	case "list":
		return artifactListCommand(args[1:])
	case "validate":
		return artifactValidateCommand(args[1:], false)
	case "verify":
		return artifactValidateCommand(args[1:], true)
	case "sync":
		return artifactSyncCommand(args[1:])
	case "watch":
		return artifactWatchCommand(args[1:])
	default:
		return fmt.Errorf("unknown artifact subcommand %q", args[0])
	}
}

func artifactCreateCommand(args []string) error {
	fs := flag.NewFlagSet("artifact create", flag.ContinueOnError)
	project := fs.String("project", "", "project slug or id")
	artifactType := fs.String("type", "", "artifact type: web_page or presentation")
	name := fs.String("name", "", "artifact name")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("artifact create does not accept arguments")
	}
	if strings.TrimSpace(*project) == "" {
		return errors.New("--project is required")
	}
	if strings.TrimSpace(*name) == "" {
		return errors.New("--name is required")
	}
	if !supportedArtifactType(*artifactType) {
		return errors.New("--type must be web_page or presentation")
	}

	now := time.Now().UTC().Format(time.RFC3339)
	slug := slugify(*name)
	projectSlug := slugify(*project)
	projectDir := filepath.Join(canvasWorkspaceRoot(), "projects", projectSlug)
	root := filepath.Join(projectDir, "artifacts", slug)
	if _, err := os.Stat(filepath.Join(root, "artifact.json")); err == nil {
		return fmt.Errorf("artifact already exists at %s", root)
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	if err := writeProjectManifest(projectDir, projectSlug, strings.TrimSpace(*project), now); err != nil {
		return err
	}
	manifest := newArtifactManifest(*project, *artifactType, *name, slug, now)
	if err := writeArtifactFiles(root, manifest); err != nil {
		return err
	}
	result := map[string]any{
		"artifact_path": root,
		"manifest_path": filepath.Join(root, "artifact.json"),
		"project":       strings.TrimSpace(*project),
		"slug":          slug,
		"type":          strings.TrimSpace(*artifactType),
	}
	if canvasRuntimeConfigured() {
		if watch, err := startArtifactWatcher(root); err != nil {
			result["watch"] = map[string]any{"watching": false, "error": err.Error()}
		} else {
			result["watch"] = watch
		}
	}
	return printJSON(result)
}

func writeProjectManifest(projectDir, slug, name, now string) error {
	path := filepath.Join(projectDir, "project.json")
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	if name == "" {
		name = slug
	}
	data, err := json.MarshalIndent(map[string]any{
		"schema_version": 1,
		"kind":           "hivy.canvas.project",
		"slug":           slug,
		"name":           name,
		"created_at":     now,
		"updated_at":     now,
	}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}

func artifactListCommand(args []string) error {
	fs := flag.NewFlagSet("artifact list", flag.ContinueOnError)
	project := fs.String("project", "", "project slug or id")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("artifact list does not accept arguments")
	}
	if strings.TrimSpace(*project) == "" {
		return errors.New("--project is required")
	}
	var result map[string]any
	path := agentCanvasPath("artifacts") + "?project=" + url.QueryEscape(strings.TrimSpace(*project))
	if err := getControlPlane(path, &result); err != nil {
		return err
	}
	return printJSON(result)
}

func artifactValidateCommand(args []string, verify bool) error {
	if len(args) != 1 {
		return errors.New("artifact path is required")
	}
	result, err := validateArtifact(args[0])
	if err != nil {
		if validationErr, ok := err.(artifactValidationError); ok {
			return printValidationResult(validationErr.Result, verify, err)
		}
		return err
	}
	return printValidationResult(result, verify, nil)
}

func printValidationResult(result artifactValidationResult, verify bool, err error) error {
	if verify {
		out := map[string]any{
			"verified": result.Valid,
			"artifact": result,
		}
		if printErr := printJSON(out); printErr != nil {
			return printErr
		}
		return err
	}
	if printErr := printJSON(result); printErr != nil {
		return printErr
	}
	return err
}

func artifactSyncCommand(args []string) error {
	if len(args) != 1 {
		return errors.New("artifact path is required")
	}
	artifactDir, manifestPath, manifest, manifestObject, err := loadArtifact(args[0])
	if err != nil {
		return err
	}
	if _, err := validateLoadedArtifact(artifactDir, manifestPath, manifest); err != nil {
		if validationErr, ok := err.(artifactValidationError); ok {
			return printValidationResult(validationErr.Result, false, err)
		}
		return err
	}
	payload, err := buildArtifactSyncPayload(artifactDir, manifest, manifestObject)
	if err != nil {
		return err
	}
	var result map[string]any
	if err := postControlPlane(agentCanvasPath("artifacts/sync"), payload, &result); err != nil {
		return err
	}
	return printJSON(result)
}

func canvasWorkspaceRoot() string {
	if dir := strings.TrimSpace(os.Getenv(envCanvasWorkspace)); dir != "" {
		return dir
	}
	return defaultCanvasDir
}

func newArtifactManifest(project, artifactType, name, slug, now string) canvasArtifactManifest {
	manifest := canvasArtifactManifest{
		SchemaVersion: 1,
		Project:       strings.TrimSpace(project),
		Slug:          slug,
		Name:          strings.TrimSpace(name),
		Type:          strings.TrimSpace(artifactType),
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if artifactType == artifactTypePresentation {
		manifest.Files = []canvasArtifactFile{{Path: "slides/slide-001.html", Role: "slide", ContentType: "text/html"}}
		manifest.Slides = []canvasArtifactSlide{{ID: "slide-001", Name: "Slide 1", Path: "slides/slide-001.html"}}
		return manifest
	}
	manifest.Entrypoint = "index.html"
	manifest.Files = []canvasArtifactFile{{Path: "index.html", Role: "entry", ContentType: "text/html"}}
	return manifest
}

func writeArtifactFiles(root string, manifest canvasArtifactManifest) error {
	for _, file := range manifest.Files {
		target := filepath.Join(root, filepath.Clean(file.Path))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(target, []byte(defaultHTML(manifest, file)), 0o600); err != nil {
			return err
		}
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, "artifact.json"), append(data, '\n'), 0o600)
}

func defaultHTML(manifest canvasArtifactManifest, file canvasArtifactFile) string {
	title := html.EscapeString(manifest.Name)
	if file.Role == "slide" {
		return fmt.Sprintf("<!doctype html>\n<html><head><meta charset=\"utf-8\"><title>%s</title></head><body><main data-canvas-id=\"slide-001\"><section data-canvas-id=\"slide-001-content\"><h1>%s</h1></section></main></body></html>\n", title, title)
	}
	return fmt.Sprintf("<!doctype html>\n<html><head><meta charset=\"utf-8\"><title>%s</title></head><body><main data-canvas-id=\"page\"><section data-canvas-id=\"hero\"><h1>%s</h1></section></main></body></html>\n", title, title)
}
