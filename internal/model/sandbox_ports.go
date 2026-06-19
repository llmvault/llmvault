package model

import (
	"fmt"
	"sort"

	"github.com/lib/pq"
)

const (
	SandboxRuntimeReservedPort = 7080
	MaxSandboxExposedPorts     = 20
)

var defaultSandboxExposedPorts = []int{3000, 5173, 8000, 8080}

func DefaultSandboxExposedPorts() []int {
	return append([]int(nil), defaultSandboxExposedPorts...)
}

func NormalizeSandboxExposedPorts(ports []int) ([]int, error) {
	if len(ports) == 0 {
		return DefaultSandboxExposedPorts(), nil
	}
	return normalizeSandboxExposedPorts(ports)
}

func NormalizeExplicitSandboxExposedPorts(ports []int) ([]int, error) {
	if len(ports) == 0 {
		return nil, fmt.Errorf("sandbox_exposed_ports must contain at least one port")
	}
	return normalizeSandboxExposedPorts(ports)
}

func SandboxExposedPortsInt64Array(ports []int) pq.Int64Array {
	out := make(pq.Int64Array, 0, len(ports))
	for _, port := range ports {
		out = append(out, int64(port))
	}
	return out
}

func SandboxExposedPortsFromInt64Array(ports pq.Int64Array) []int {
	out := make([]int, 0, len(ports))
	for _, port := range ports {
		out = append(out, int(port))
	}
	return out
}

func normalizeSandboxExposedPorts(ports []int) ([]int, error) {
	seen := map[int]struct{}{}
	out := make([]int, 0, len(ports))
	for _, port := range ports {
		if port <= 0 || port > 65535 {
			return nil, fmt.Errorf("sandbox_exposed_ports must contain ports between 1 and 65535")
		}
		if port == SandboxRuntimeReservedPort {
			return nil, fmt.Errorf("sandbox_exposed_ports cannot include reserved runtime port %d", SandboxRuntimeReservedPort)
		}
		if _, ok := seen[port]; ok {
			continue
		}
		seen[port] = struct{}{}
		out = append(out, port)
	}
	if len(out) > MaxSandboxExposedPorts {
		return nil, fmt.Errorf("sandbox_exposed_ports cannot contain more than %d ports", MaxSandboxExposedPorts)
	}
	sort.Ints(out)
	return out, nil
}
