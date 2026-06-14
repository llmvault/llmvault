package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	microsandbox "github.com/superradcompany/microsandbox/sdk/go"
)

type persistedSandboxConfig struct {
	Labels       map[string]string       `json:"labels"`
	Ports        json.RawMessage         `json:"ports"`
	PortBindings []persistedPortBinding  `json:"port_bindings"`
	Network      *persistedNetworkConfig `json:"network"`
}

type persistedNetworkConfig struct {
	Ports        json.RawMessage        `json:"ports"`
	PortBindings []persistedPortBinding `json:"port_bindings"`
}

type persistedPortBinding struct {
	HostPort  uint16 `json:"host_port"`
	GuestPort uint16 `json:"guest_port"`
	Protocol  string `json:"protocol"`
}

func recoverSandboxState(name, status, configJSON string) (sandboxState, bool) {
	var cfg persistedSandboxConfig
	if configJSON != "" {
		if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
			return sandboxState{}, false
		}
	}

	sandboxID := cfg.Labels[sandboxIDLabel]
	if sandboxID == "" && isHivySandboxName(name) {
		sandboxID = name
	}
	if sandboxID == "" {
		return sandboxState{}, false
	}

	ports := publishedPorts(cfg)
	return sandboxState{
		ID:     sandboxID,
		Name:   name,
		Status: status,
		Labels: cloneStringMap(cfg.Labels),
		Ports:  ports,
	}, true
}

func publishedPorts(cfg persistedSandboxConfig) map[int]int {
	ports := map[int]int{}
	addPublishedPorts(ports, cfg.Ports)
	addPublishedPortBindings(ports, cfg.PortBindings)
	if cfg.Network != nil {
		addPublishedPorts(ports, cfg.Network.Ports)
		addPublishedPortBindings(ports, cfg.Network.PortBindings)
	}
	return ports
}

func addPublishedPorts(dst map[int]int, raw json.RawMessage) {
	if len(raw) == 0 || string(raw) == "null" {
		return
	}
	var hostToGuest map[string]uint16
	if err := json.Unmarshal(raw, &hostToGuest); err == nil {
		addPublishedPortMap(dst, hostToGuest)
		return
	}
	var bindings []persistedPortBinding
	if err := json.Unmarshal(raw, &bindings); err == nil {
		addPublishedPortBindings(dst, bindings)
	}
}

func addPublishedPortMap(dst map[int]int, hostToGuest map[string]uint16) {
	for hostRaw, guest := range hostToGuest {
		host, err := strconv.Atoi(hostRaw)
		if err != nil || host <= 0 || guest == 0 {
			continue
		}
		dst[int(guest)] = host
	}
}

func addPublishedPortBindings(dst map[int]int, bindings []persistedPortBinding) {
	for _, binding := range bindings {
		if binding.HostPort == 0 || binding.GuestPort == 0 {
			continue
		}
		if binding.Protocol != "" && !strings.EqualFold(binding.Protocol, "tcp") {
			continue
		}
		dst[int(binding.GuestPort)] = int(binding.HostPort)
	}
}

func hivyLabels(sandboxID string, labels map[string]string) map[string]string {
	out := cloneStringMap(labels)
	out[hivyManagedLabel] = "true"
	if out[sandboxIDLabel] == "" && isHivySandboxName(sandboxID) {
		out[sandboxIDLabel] = sandboxID
	}
	return out
}

func volumeLabels(sandboxID, purpose string) map[string]string {
	labels := map[string]string{
		hivyManagedLabel:   "true",
		volumePurposeLabel: purpose,
	}
	if isHivySandboxName(sandboxID) {
		labels[sandboxIDLabel] = sandboxID
	}
	return labels
}

func workspaceVolumeName(sandboxID string) string {
	return "hivy-" + sandboxID
}

func dockerDataVolumeName(sandboxID string) string {
	return "hivy-docker-" + sandboxID
}

func sandboxVolumeSizesMiB(diskGB int) (uint32, uint32) {
	if diskGB <= 0 {
		diskGB = defaultSandboxDiskGB
	}
	total := uint32(diskGB * 1024)
	dockerData := total / 4
	if dockerData > maxDockerDataVolumeMiB {
		dockerData = maxDockerDataVolumeMiB
	}
	if total > minWorkspaceVolumeMiB && total-dockerData < minWorkspaceVolumeMiB {
		dockerData = total - minWorkspaceVolumeMiB
	}
	if dockerData == 0 {
		return total, 0
	}
	return total - dockerData, dockerData
}

func ensureVolume(ctx context.Context, name string, opts ...microsandbox.VolumeOption) error {
	_, err := microsandbox.CreateVolume(ctx, name, opts...)
	if err == nil || microsandbox.IsKind(err, microsandbox.ErrVolumeAlreadyExists) {
		return nil
	}
	return fmt.Errorf("create microsandbox volume %s: %w", name, err)
}

func isHivySandboxName(name string) bool {
	return strings.HasPrefix(name, "sbx_")
}

func cloneStringMap(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneIntMap(in map[int]int) map[int]int {
	out := make(map[int]int, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func mapValues(in map[int]int) []int {
	out := make([]int, 0, len(in))
	for _, v := range in {
		out = append(out, v)
	}
	return out
}

func uniquePorts(in []int) []int {
	out := make([]int, 0, len(in))
	seen := map[int]struct{}{}
	for _, port := range in {
		if port <= 0 || port > 65535 {
			continue
		}
		if _, ok := seen[port]; ok {
			continue
		}
		seen[port] = struct{}{}
		out = append(out, port)
	}
	return out
}

func (m *MicrosandboxBackend) setSandboxStatus(sandboxID, status string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	state, ok := m.sandboxes[sandboxID]
	if !ok {
		return
	}
	state.Status = status
	m.sandboxes[sandboxID] = state
}
