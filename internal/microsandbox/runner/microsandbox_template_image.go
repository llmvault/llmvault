package runner

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/sourcegraph/conc/pool"

	"github.com/usehivy/hivy/internal/microsandbox/security"
)

const (
	defaultImageRegistry       = "10.80.0.3:5000"
	templateImageBuildTimeout  = 30 * time.Minute
	templateValidationPort     = 7080
	templateValidationTimeout  = 4 * time.Minute
	templateValidationInterval = 2 * time.Second
)

var buildctlCommandContext = exec.CommandContext

func (m *MicrosandboxBackend) CreateTemplate(ctx context.Context, req CreateTemplateRequest, onEvent func(TemplateBuildEvent)) (*CreateTemplateResponse, error) {
	registry := strings.TrimSpace(m.imageRegistry)
	if registry == "" {
		registry = defaultImageRegistry
	}
	buildID := strings.TrimSpace(req.BuildID)
	if buildID == "" {
		id, err := security.ShortID("bld")
		if err != nil {
			return nil, err
		}
		buildID = "bld-" + id
	}

	mutableRef := templateMutableImageRef(registry, req.OrgID, req.ID, buildID)
	emitLog := func(message string) {
		if onEvent != nil && strings.TrimSpace(message) != "" {
			onEvent(TemplateBuildEvent{Type: "log", ID: req.ID, Message: message})
		}
	}

	logs, digest, err := buildTemplateImage(ctx, req.BaseImageRef, mutableRef, req.Commands, emitLog)
	if err != nil {
		return nil, err
	}
	finalRef := templateDigestImageRef(mutableRef, digest)
	validationID, err := m.validateTemplateImage(ctx, req, finalRef, emitLog)
	if err != nil {
		return nil, err
	}
	return &CreateTemplateResponse{
		ID:                  req.ID,
		ImageRef:            finalRef,
		ImageDigest:         digest,
		ValidationSandboxID: validationID,
		Logs:                logs,
	}, nil
}

func buildTemplateImage(ctx context.Context, baseImage, imageRef string, commands []string, onLog func(string)) (string, string, error) {
	workDir, err := os.MkdirTemp("", "hivy-template-build-*")
	if err != nil {
		return "", "", err
	}
	defer func() {
		_ = os.RemoveAll(workDir)
	}()
	if err := os.WriteFile(filepath.Join(workDir, "Dockerfile"), []byte(templateDockerfile(baseImage, commands)), 0o600); err != nil {
		return "", "", err
	}
	metadataPath := filepath.Join(workDir, "metadata.json")
	buildCtx, cancel := context.WithTimeout(ctx, templateImageBuildTimeout)
	defer cancel()

	args := []string{
		"build",
		"--frontend=dockerfile.v0",
		"--local", "context=" + workDir,
		"--local", "dockerfile=" + workDir,
		"--output", "type=image,name=" + imageRef + ",push=true,registry.insecure=true",
		"--metadata-file", metadataPath,
	}
	cmd := buildctlCommandContext(buildCtx, "buildctl", args...)
	cmd.Env = append(os.Environ(), "BUILDKIT_PROGRESS=plain")

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", "", err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", "", err
	}
	if err := cmd.Start(); err != nil {
		return "", "", err
	}

	var logs strings.Builder
	var logMu sync.Mutex
	capture := func(line string) {
		logMu.Lock()
		logs.WriteString(line)
		logs.WriteByte('\n')
		logMu.Unlock()
		if onLog != nil {
			onLog(line)
		}
	}
	p := pool.New().WithMaxGoroutines(2)
	p.Go(func() {
		scanBuildOutput(stdout, capture)
	})
	p.Go(func() {
		scanBuildOutput(stderr, capture)
	})
	p.Wait()

	if err := cmd.Wait(); err != nil {
		return logs.String(), "", fmt.Errorf("buildctl build: %w", err)
	}
	digest, err := readBuildImageDigest(metadataPath)
	if err != nil {
		return logs.String(), "", err
	}
	return logs.String(), digest, nil
}

func scanBuildOutput(r io.Reader, capture func(string)) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		capture(scanner.Text())
	}
}

func readBuildImageDigest(metadataPath string) (string, error) {
	raw, err := os.ReadFile(metadataPath)
	if err != nil {
		return "", fmt.Errorf("read build metadata: %w", err)
	}
	var metadata map[string]any
	if err := json.Unmarshal(raw, &metadata); err != nil {
		return "", fmt.Errorf("parse build metadata: %w", err)
	}
	if digest, ok := metadata["containerimage.digest"].(string); ok && strings.HasPrefix(digest, "sha256:") {
		return digest, nil
	}
	return "", fmt.Errorf("build metadata missing containerimage.digest")
}

func templateDockerfile(baseImage string, commands []string) string {
	var b strings.Builder
	b.WriteString("FROM ")
	b.WriteString(strings.TrimSpace(baseImage))
	b.WriteString("\n\n")
	if len(commands) == 0 {
		return b.String()
	}
	b.WriteString("SHELL [\"/bin/bash\", \"-o\", \"pipefail\", \"-c\"]\n")
	b.WriteString("RUN <<'HIVY_TEMPLATE_SCRIPT'\n")
	b.WriteString("set -eux\n")
	for _, cmd := range commands {
		if strings.TrimSpace(cmd) == "" {
			continue
		}
		b.WriteString(cmd)
		b.WriteByte('\n')
	}
	b.WriteString("HIVY_TEMPLATE_SCRIPT\n")
	return b.String()
}

func templateMutableImageRef(registry, orgID, templateID, buildID string) string {
	return strings.TrimRight(registry, "/") + "/images/" + sanitizeImagePathPart(orgID) + "/" + sanitizeImagePathPart(templateID) + ":" + sanitizeImageTag(buildID)
}

func templateDigestImageRef(mutableRef, digest string) string {
	return imageRepositoryWithoutTag(mutableRef) + "@" + digest
}

func imageRepositoryWithoutTag(ref string) string {
	ref = strings.SplitN(strings.TrimSpace(ref), "@", 2)[0]
	lastSlash := strings.LastIndex(ref, "/")
	lastColon := strings.LastIndex(ref, ":")
	if lastColon > lastSlash {
		return ref[:lastColon]
	}
	return ref
}

func sanitizeImagePathPart(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastSeparator := false
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastSeparator = false
		case r == '.', r == '_', r == '-':
			if !lastSeparator {
				b.WriteRune(r)
				lastSeparator = true
			}
		default:
			if !lastSeparator {
				b.WriteByte('-')
				lastSeparator = true
			}
		}
	}
	out := strings.Trim(b.String(), "._-")
	if out == "" {
		return "unknown"
	}
	return out
}

func sanitizeImageTag(value string) string {
	value = sanitizeImagePathPart(value)
	return strings.ReplaceAll(value, "/", "-")
}
