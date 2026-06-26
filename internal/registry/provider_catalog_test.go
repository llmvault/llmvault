package registry

import "testing"

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
