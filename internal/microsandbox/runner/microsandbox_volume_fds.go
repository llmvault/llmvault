package runner

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

func countSandboxVolumeFDs(sandboxID string) int {
	if runtime.GOOS != "linux" {
		return 0
	}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0
	}
	volumePaths := []string{
		microsandboxVolumePath(workspaceVolumeName(sandboxID)),
		microsandboxVolumePath(dockerDataVolumeName(sandboxID)),
	}
	count := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, err := strconv.Atoi(entry.Name()); err != nil {
			continue
		}
		fdDir := filepath.Join("/proc", entry.Name(), "fd")
		fds, err := os.ReadDir(fdDir)
		if err != nil {
			continue
		}
		for _, fd := range fds {
			target, err := os.Readlink(filepath.Join(fdDir, fd.Name()))
			if err != nil {
				continue
			}
			if pathReferencesAnyVolume(target, volumePaths) {
				count++
			}
		}
	}
	return count
}

func pathReferencesAnyVolume(target string, volumePaths []string) bool {
	for _, volumePath := range volumePaths {
		if target == volumePath || strings.HasPrefix(target, volumePath+string(os.PathSeparator)) {
			return true
		}
	}
	return false
}

func microsandboxVolumePath(volumeName string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		home = "/root"
	}
	return filepath.Join(home, ".microsandbox", "volumes", volumeName)
}
