package runner

import (
	"fmt"
	"net"
	"strings"

	microsandbox "github.com/superradcompany/microsandbox/sdk/go"

	"github.com/usehivy/hivy/internal/microsandbox/config"
)

// sandboxNetworkConfig keeps the SDK's public-only policy unless runner-local
// DNS and a private ingress destination are configured together. Production
// sandboxes may then resolve Hivy's public API names to their runner-local
// HAProxy without gaining access to the rest of the private network.
func sandboxNetworkConfig(cfg config.Config) (*microsandbox.NetworkConfig, error) {
	nameserver := strings.TrimSpace(cfg.RunnerDNSNameserver)
	privateCIDR := strings.TrimSpace(cfg.RunnerPrivateEgressCIDR)
	privatePorts := splitNetworkPorts(cfg.RunnerPrivateEgressPorts)
	if nameserver == "" && privateCIDR == "" && len(privatePorts) == 0 {
		return microsandbox.NetworkPolicy.PublicOnly(), nil
	}
	if nameserver == "" || privateCIDR == "" || len(privatePorts) == 0 {
		return nil, fmt.Errorf("runner sandbox private networking requires DNS nameserver, egress CIDR, and egress ports")
	}

	dnsHost, dnsPort, err := net.SplitHostPort(nameserver)
	if err != nil || strings.TrimSpace(dnsHost) == "" || strings.TrimSpace(dnsPort) == "" {
		return nil, fmt.Errorf("invalid runner DNS nameserver %q", nameserver)
	}
	rebindProtection := false
	rules := []microsandbox.PolicyRule{
		{
			Action:      microsandbox.PolicyActionAllow,
			Direction:   microsandbox.PolicyDirectionEgress,
			Destination: "host",
			Protocols: []microsandbox.PolicyProtocol{
				microsandbox.PolicyProtocolTCP,
				microsandbox.PolicyProtocolUDP,
			},
			Port: "53",
		},
		{
			Action:      microsandbox.PolicyActionAllow,
			Direction:   microsandbox.PolicyDirectionEgress,
			Destination: dnsHost,
			Protocols: []microsandbox.PolicyProtocol{
				microsandbox.PolicyProtocolTCP,
				microsandbox.PolicyProtocolUDP,
			},
			Port: dnsPort,
		},
		{
			Action:      microsandbox.PolicyActionAllow,
			Direction:   microsandbox.PolicyDirectionEgress,
			Destination: privateCIDR,
			Protocol:    microsandbox.PolicyProtocolTCP,
			Ports:       privatePorts,
		},
	}
	for _, destination := range []string{"private", "host", "loopback", "link-local", "metadata", "multicast"} {
		rules = append(rules, microsandbox.PolicyRule{
			Action:      microsandbox.PolicyActionDeny,
			Direction:   microsandbox.PolicyDirectionEgress,
			Destination: destination,
		})
	}
	return &microsandbox.NetworkConfig{
		Rules:          rules,
		DefaultEgress:  microsandbox.PolicyActionAllow,
		DefaultIngress: microsandbox.PolicyActionAllow,
		DNS: &microsandbox.DNSConfig{
			Nameservers:      []string{nameserver},
			RebindProtection: &rebindProtection,
		},
	}, nil
}

func splitNetworkPorts(raw string) []string {
	var ports []string
	for _, value := range strings.Split(raw, ",") {
		if value = strings.TrimSpace(value); value != "" {
			ports = append(ports, value)
		}
	}
	return ports
}
