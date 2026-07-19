package main

import "testing"

func TestRailwayIsConfiguredForMCP(t *testing.T) {
	t.Parallel()

	for _, service := range AllServices() {
		if service.Name != "railway" {
			continue
		}
		if service.PushToMCP == nil || !*service.PushToMCP {
			t.Fatal("Railway must be explicitly enabled for MCP when regenerated")
		}
		return
	}
	t.Fatal("Railway GraphQL service configuration is missing")
}
