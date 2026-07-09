package bootstrap

import (
	"testing"

	"github.com/usehivy/hivy/internal/config"
)

func TestBuildWebProviderNoneConfigured(t *testing.T) {
	if p := buildWebProvider(&config.Config{}); p != nil {
		t.Fatalf("expected nil provider, got %v", p.Name())
	}
}

func TestBuildWebProviderSpiderOnly(t *testing.T) {
	p := buildWebProvider(&config.Config{SpiderAPIKey: "k"})
	if p == nil {
		t.Fatal("expected provider, got nil")
	}
	if got, want := p.Name(), "router(spider)"; got != want {
		t.Fatalf("Name() = %q, want %q", got, want)
	}
}

func TestBuildWebProviderFirecrawlOnly(t *testing.T) {
	p := buildWebProvider(&config.Config{FirecrawlAPIKey: "k"})
	if p == nil {
		t.Fatal("expected provider, got nil")
	}
	if got, want := p.Name(), "router(firecrawl)"; got != want {
		t.Fatalf("Name() = %q, want %q", got, want)
	}
}

func TestBuildWebProviderBothSpiderFirst(t *testing.T) {
	p := buildWebProvider(&config.Config{SpiderAPIKey: "k", FirecrawlAPIKey: "k"})
	if p == nil {
		t.Fatal("expected provider, got nil")
	}
	if got, want := p.Name(), "router(spider,firecrawl)"; got != want {
		t.Fatalf("Name() = %q, want %q", got, want)
	}
}

func TestBuildWebProviderSerperOnly(t *testing.T) {
	p := buildWebProvider(&config.Config{SerperAPIKey: "k"})
	if p == nil {
		t.Fatal("expected provider, got nil")
	}
	if got, want := p.Name(), "router(serper)"; got != want {
		t.Fatalf("Name() = %q, want %q", got, want)
	}
}

func TestBuildWebProviderAllThree(t *testing.T) {
	p := buildWebProvider(&config.Config{SpiderAPIKey: "k", FirecrawlAPIKey: "k", SerperAPIKey: "k"})
	if p == nil {
		t.Fatal("expected provider, got nil")
	}
	if got, want := p.Name(), "router(spider,firecrawl,serper)"; got != want {
		t.Fatalf("Name() = %q, want %q", got, want)
	}
}
