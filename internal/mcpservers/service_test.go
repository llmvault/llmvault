package mcpservers

import (
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

func TestNormalizeServer_RejectsTrustedRuntimeName(t *testing.T) {
	_, err := normalizeServer(CreateServerParams{
		Scope: model.MCPServerScopeOrg, Name: "Hivy", Slug: "hivy",
		URL: "https://mcp.example.com", AuthType: model.MCPAuthTypeNone,
	}, uuid.New(), uuid.New())
	if err == nil {
		t.Fatal("expected reserved Hivy slug to be rejected")
	}
}

func TestSelectTokenEndpointAuthMethod(t *testing.T) {
	tests := []struct {
		name      string
		supported []string
		hasSecret bool
		want      string
		wantError bool
	}{
		{name: "metadata default with secret", hasSecret: true, want: "client_secret_basic"},
		{name: "metadata default public client", want: "none"},
		{name: "prefer basic", supported: []string{"client_secret_post", "client_secret_basic"}, hasSecret: true, want: "client_secret_basic"},
		{name: "post only", supported: []string{"client_secret_post"}, hasSecret: true, want: "client_secret_post"},
		{name: "public none", supported: []string{"none"}, want: "none"},
		{name: "unsupported", supported: []string{"private_key_jwt"}, hasSecret: true, wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := selectTokenEndpointAuthMethod(test.supported, test.hasSecret)
			if test.wantError {
				if err == nil {
					t.Fatalf("method = %q, want error", got)
				}
				return
			}
			if err != nil || got != test.want {
				t.Fatalf("method = %q, err=%v, want %q", got, err, test.want)
			}
		})
	}
}

func TestSameOAuthIssuer(t *testing.T) {
	if !sameOAuthIssuer("https://auth.example.com/issuer", "https://AUTH.example.com:443/issuer") {
		t.Fatal("equivalent issuer URLs did not match")
	}
	if sameOAuthIssuer("https://auth.example.com/issuer", "https://auth.example.com/other") {
		t.Fatal("different issuer paths matched")
	}
}

func TestSafeRedirectAfter(t *testing.T) {
	if got := safeRedirectAfter("/w/settings/mcp"); got != "/w/settings/mcp" {
		t.Fatalf("safe redirect = %q", got)
	}
	for _, unsafe := range []string{"https://evil.example", "//evil.example/path", "/%2f%2fevil.example", `/\\evil.example`, "/path?next=https://evil.example"} {
		if got := safeRedirectAfter(unsafe); got != "" {
			t.Errorf("safeRedirectAfter(%q) = %q, want empty", unsafe, got)
		}
	}
}
