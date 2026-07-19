package runner

import (
	"testing"

	microsandbox "github.com/superradcompany/microsandbox/sdk/go"

	"github.com/usehivy/hivy/internal/microsandbox/config"
)

func TestSandboxNetworkConfigUsesRunnerDNSAndRestrictsPrivateEgress(t *testing.T) {
	network, err := sandboxNetworkConfig(config.Config{
		RunnerDNSNameserver:      "10.80.1.2:53",
		RunnerPrivateEgressCIDR:  "10.80.1.4/32",
		RunnerPrivateEgressPorts: "443",
	})
	if err != nil {
		t.Fatalf("sandboxNetworkConfig: %v", err)
	}
	if network.DNS == nil || len(network.DNS.Nameservers) != 1 || network.DNS.Nameservers[0] != "10.80.1.2:53" {
		t.Fatalf("DNS configuration = %#v", network.DNS)
	}
	if network.DNS.RebindProtection == nil || *network.DNS.RebindProtection {
		t.Fatalf("DNS rebind protection must be disabled for controlled split-horizon DNS")
	}
	if len(network.Rules) < 3 {
		t.Fatalf("network rules = %#v", network.Rules)
	}
	dnsProxy := network.Rules[0]
	if dnsProxy.Action != microsandbox.PolicyActionAllow || dnsProxy.Destination != "host" || dnsProxy.Port != "53" {
		t.Fatalf("DNS proxy allow rule = %#v", dnsProxy)
	}
	dns := network.Rules[1]
	if dns.Action != microsandbox.PolicyActionAllow || dns.Destination != "10.80.1.2" || dns.Port != "53" {
		t.Fatalf("DNS allow rule = %#v", dns)
	}
	ingress := network.Rules[2]
	if ingress.Action != microsandbox.PolicyActionAllow || ingress.Destination != "10.80.1.4/32" || len(ingress.Ports) != 1 || ingress.Ports[0] != "443" {
		t.Fatalf("private ingress allow rule = %#v", ingress)
	}
}

func TestSandboxNetworkConfigDefaultsToPublicOnly(t *testing.T) {
	network, err := sandboxNetworkConfig(config.Config{})
	if err != nil {
		t.Fatalf("sandboxNetworkConfig: %v", err)
	}
	if network.Policy != microsandbox.NetworkPolicyPresetPublicOnly {
		t.Fatalf("network policy = %q, want public-only", network.Policy)
	}
}

func TestSandboxNetworkConfigRejectsPartialPrivateConfiguration(t *testing.T) {
	if _, err := sandboxNetworkConfig(config.Config{RunnerDNSNameserver: "10.80.1.2:53"}); err == nil {
		t.Fatal("sandboxNetworkConfig accepted incomplete private networking")
	}
}
