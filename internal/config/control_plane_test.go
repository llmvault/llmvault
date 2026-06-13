package config

import "testing"

func TestRuntimeControlPlaneBaseURLPrefersAPIWebhookBaseURL(t *testing.T) {
	cfg := &Config{
		APIWebhookBaseURL: "http://host.docker.internal:8080/",
	}
	if got := cfg.RuntimeControlPlaneBaseURL(); got != "http://host.docker.internal:8080" {
		t.Fatalf("RuntimeControlPlaneBaseURL = %q", got)
	}
}

func TestRuntimeControlPlaneBaseURLFallsBackToDefault(t *testing.T) {
	cfg := &Config{}
	if got := cfg.RuntimeControlPlaneBaseURL(); got != defaultRuntimeControlPlaneBaseURL {
		t.Fatalf("RuntimeControlPlaneBaseURL = %q", got)
	}
}
