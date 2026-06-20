package runner

import (
	"bufio"
	"context"
	"os"
	"runtime"
	"strconv"
	"strings"
)

func (m *MicrosandboxBackend) Connections(_ context.Context, sandboxID string) (*ConnectionsResponse, error) {
	ports := m.sandboxPorts(sandboxID)
	byGuest := make(map[int]int, len(ports))
	byHost := make(map[int]int, len(ports))
	for guest, host := range ports {
		byGuest[guest] = 0
		byHost[host] = 0
	}
	counts := activeTCPConnectionsByPort()
	total := 0
	for guest, host := range ports {
		count := counts[host]
		byGuest[guest] = count
		byHost[host] = count
		total += count
	}
	return &ConnectionsResponse{
		SandboxID:         sandboxID,
		ActiveConnections: total,
		ByGuestPort:       byGuest,
		ByHostPort:        byHost,
	}, nil
}

func activeTCPConnectionsByPort() map[int]int {
	counts := map[int]int{}
	if runtime.GOOS != "linux" {
		return counts
	}
	countTCPFile("/proc/net/tcp", counts)
	countTCPFile("/proc/net/tcp6", counts)
	return counts
}

func countTCPFile(path string, counts map[int]int) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		return
	}
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 4 {
			continue
		}
		state := strings.ToUpper(fields[3])
		if state == "0A" || state == "06" {
			continue
		}
		parts := strings.Split(fields[1], ":")
		if len(parts) != 2 {
			continue
		}
		port64, err := strconv.ParseInt(parts[1], 16, 32)
		if err != nil {
			continue
		}
		counts[int(port64)]++
	}
}
