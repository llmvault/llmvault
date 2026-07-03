package main

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var envNameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// writeEnvFile atomically rewrites the app env file (0600) in the
// systemd-EnvironmentFile-compatible KEY="value" format.
func writeEnvFile(path string, vars map[string]string) error {
	names := make([]string, 0, len(vars))
	for name := range vars {
		if !envNameRe.MatchString(name) {
			return fmt.Errorf("invalid env var name %q", name)
		}
		if strings.ContainsAny(vars[name], "\n\r") {
			return fmt.Errorf("env var %q contains a newline", name)
		}
		names = append(names, name)
	}
	sort.Strings(names)

	var b strings.Builder
	for _, name := range names {
		value := strings.ReplaceAll(vars[name], `\`, `\\`)
		value = strings.ReplaceAll(value, `"`, `\"`)
		fmt.Fprintf(&b, "%s=\"%s\"\n", name, value)
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), ".env-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}
	if err := tmp.Chmod(0o600); err != nil {
		cleanup()
		return err
	}
	if _, err := tmp.WriteString(b.String()); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}

// parseEnvFile reads the env file into KEY=value pairs. A missing file is an
// empty environment (matches EnvironmentFile=- semantics).
func parseEnvFile(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var vars []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, raw, ok := strings.Cut(line, "=")
		if !ok || !envNameRe.MatchString(name) {
			continue
		}
		vars = append(vars, name+"="+unquoteEnvValue(raw))
	}
	return vars, nil
}

func unquoteEnvValue(raw string) string {
	if len(raw) >= 2 && strings.HasPrefix(raw, `"`) && strings.HasSuffix(raw, `"`) {
		inner := raw[1 : len(raw)-1]
		inner = strings.ReplaceAll(inner, `\"`, `"`)
		inner = strings.ReplaceAll(inner, `\\`, `\`)
		return inner
	}
	return raw
}

type envRequest struct {
	Vars map[string]string `json:"vars"`
}

// handleEnv rewrites the env file and restarts the app.
//
//	POST /env {"vars": {"KEY": "value"}}
func (s *server) handleEnv(w http.ResponseWriter, r *http.Request) {
	var req envRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Vars == nil {
		writeError(w, http.StatusBadRequest, "vars is required")
		return
	}

	s.deployMu.Lock()
	defer s.deployMu.Unlock()

	if err := writeEnvFile(s.cfg.envFile(), req.Vars); err != nil {
		writeError(w, http.StatusBadRequest, "write env file: "+err.Error())
		return
	}
	if err := s.proc.Restart(r.Context()); err != nil {
		s.log.ErrorContext(r.Context(), "restart after env update failed", "error", err)
		writeError(w, http.StatusInternalServerError, "env written but restart failed: "+err.Error())
		return
	}
	s.log.InfoContext(r.Context(), "env file updated", "vars", len(req.Vars))
	writeJSON(w, http.StatusOK, map[string]any{"updated": len(req.Vars), "restarted": true})
}
