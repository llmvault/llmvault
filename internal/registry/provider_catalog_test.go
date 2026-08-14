package registry

import "testing"

func TestDeepSeekProviderCatalog(t *testing.T) {
	provider, ok := Global().GetProvider("deepseek")
	if !ok {
		t.Fatal("deepseek provider not found")
	}
	if provider.API != "https://api.deepseek.com" {
		t.Fatalf("deepseek API = %q", provider.API)
	}

	tests := []struct {
		id          string
		name        string
		releaseDate string
	}{
		{"deepseek-v4-flash", "DeepSeek V4 Flash 0731", "2026-07-31"},
		{"deepseek-v4-pro", "DeepSeek V4 Pro 0813", "2026-08-13"},
	}
	for _, test := range tests {
		model, exists := provider.Models[test.id]
		if !exists {
			t.Fatalf("deepseek model %q not found", test.id)
		}
		if model.Name != test.name || model.ReleaseDate != test.releaseDate {
			t.Fatalf("deepseek model %q = %#v", test.id, model)
		}
	}
}

func TestReveProviderCatalog(t *testing.T) {
	provider, ok := Global().GetProvider("reve")
	if !ok {
		t.Fatal("reve provider not found")
	}
	if provider.Name != "Reve" {
		t.Fatalf("reve name = %q, want Reve", provider.Name)
	}
	if provider.API != "https://api.reve.com" {
		t.Fatalf("reve API = %q, want https://api.reve.com", provider.API)
	}
	if provider.Doc != "https://api.reve.com/console/docs" {
		t.Fatalf("reve docs = %q", provider.Doc)
	}
}

func TestQuiverProviderCatalog(t *testing.T) {
	provider, ok := Global().GetProvider("quiver")
	if !ok {
		t.Fatal("quiver provider not found")
	}
	if provider.Name != "Quiver AI" {
		t.Fatalf("quiver name = %q, want Quiver AI", provider.Name)
	}
	if provider.API != "https://api.quiver.ai" {
		t.Fatalf("quiver API = %q, want https://api.quiver.ai", provider.API)
	}
	model, ok := provider.Models["arrow-1.1"]
	if !ok {
		t.Fatal("quiver arrow-1.1 model not found")
	}
	if !ModelSupportsVectorImageOutput(model) {
		t.Fatalf("quiver arrow-1.1 should support vector output, family=%q", model.Family)
	}
}
