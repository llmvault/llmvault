package handler

import (
	"errors"
	"fmt"
	"path"
	"strings"
)

func bearerFromHeader(h string) string {
	if h == "" {
		return ""
	}
	parts := strings.SplitN(h, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

// splitAssetPath splits a "<folder>/<filename>" tail into a sanitized folder
// (without leading or trailing slash) and a sanitized filename. Empty path,
// missing filename, traversal segments and absolute paths are rejected.
func splitAssetPath(raw string) (folder, filename string, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", errors.New("filename is required")
	}
	if strings.HasPrefix(raw, "/") {
		return "", "", errors.New("path must be relative")
	}
	if strings.ContainsRune(raw, 0) {
		return "", "", errors.New("invalid path")
	}

	for seg := range strings.SplitSeq(raw, "/") {
		if seg == "." || seg == ".." {
			return "", "", fmt.Errorf("invalid path segment %q", seg)
		}
	}

	dir, file := path.Split(raw)
	dir = strings.Trim(dir, "/")
	if file == "" {
		return "", "", errors.New("filename is required")
	}

	return dir, file, nil
}

// validateFolder runs the same rules as splitAssetPath but for a bare folder
// (no filename component). Empty input means "drive root" and is allowed.
func validateFolder(raw string) (string, error) {
	raw = strings.Trim(strings.TrimSpace(raw), "/")
	if raw == "" {
		return "", nil
	}
	if strings.ContainsRune(raw, 0) {
		return "", errors.New("invalid path")
	}
	for seg := range strings.SplitSeq(raw, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return "", fmt.Errorf("invalid path segment %q", seg)
		}
	}
	return raw, nil
}
