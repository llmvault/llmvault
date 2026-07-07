package integrations

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/usehivy/hivy/internal/nango"
)

func TestLoadManifests_MissingDirectoryFails(t *testing.T) {
	_, err := loadManifests(filepath.Join(t.TempDir(), "missing"))
	if err == nil {
		t.Fatal("expected missing global integrations directory to fail")
	}
	if !strings.Contains(err.Error(), "global integrations dir") {
		t.Fatalf("error = %v, want global integrations dir context", err)
	}
}

func TestGlobalIntegrationManifestsUseAdminCredentialFields(t *testing.T) {
	manifests, err := loadManifests("global/integrations")
	if err != nil {
		t.Fatalf("load global manifests: %v", err)
	}
	if len(manifests) == 0 {
		t.Fatal("expected global integration manifests")
	}

	for _, manifest := range manifests {
		rawCreds, _ := manifest.raw["credentials"].(map[string]any)
		for key := range rawCreds {
			if strings.HasSuffix(key, "_env") {
				t.Fatalf("%s still declares env-backed credential field %q", manifest.SourcePath, key)
			}
		}
		assertManifestAdminFields(t, manifest)
	}
}

func TestAdminCredentialFieldsFromManifest(t *testing.T) {
	manifest := Manifest{
		ID:          "linear",
		Credentials: &CredentialsManifest{Type: "OAUTH2", ClientID: true, ClientSecret: true, WebhookSecret: true},
	}

	fields := adminCredentialFields(manifest, nango.Provider{Name: "linear", AuthMode: "OAUTH2"})
	if len(fields) != 3 {
		t.Fatalf("fields = %d, want 3", len(fields))
	}
	if fields[0].Name != "client_id" || fields[1].Name != "client_secret" || fields[2].Name != "webhook_secret" {
		t.Fatalf("unexpected fields: %+v", fields)
	}
	if fields[1].Secret != true || fields[2].Secret != true {
		t.Fatalf("expected secret credential fields, got %+v", fields)
	}
}

func TestGlobalGitHubManifestsCarryBotHandles(t *testing.T) {
	manifests, err := loadManifests("global/integrations")
	if err != nil {
		t.Fatalf("load global manifests: %v", err)
	}
	want := map[string]string{
		"github-app":              "usehivy",
		"github-app-code-reviews": "usehivy-reviews",
	}
	got := map[string]string{}
	for _, manifest := range manifests {
		if _, ok := want[manifest.ID]; ok {
			got[manifest.ID] = manifest.BotHandle
		}
		// Legacy providers must no longer ship as manifests.
		switch manifest.ID {
		case "github-app-oauth", "github-pat", "github":
			t.Fatalf("legacy provider manifest %q still present at %s", manifest.ID, manifest.SourcePath)
		}
	}
	for id, handle := range want {
		if got[id] != handle {
			t.Fatalf("manifest %q bot_handle = %q, want %q", id, got[id], handle)
		}
	}
}

func assertManifestAdminFields(t *testing.T, manifest Manifest) {
	t.Helper()
	switch manifest.ID {
	case "github-app", "github-app-code-reviews":
		if manifest.Credentials == nil {
			t.Fatalf("%s: github app requires credentials", manifest.SourcePath)
		}
		creds := manifest.Credentials
		if creds.Type != "APP" || !creds.AppID || !creds.AppLinkField || !creds.PrivateKey {
			t.Fatalf("%s: github app requires app_id, app_link_field, and private_key admin fields", manifest.SourcePath)
		}
	case "linear", "notion":
		if manifest.Credentials == nil {
			t.Fatalf("%s: OAuth provider requires credentials", manifest.SourcePath)
		}
		creds := manifest.Credentials
		if creds.Type != "OAUTH2" || !creds.ClientID || !creds.ClientSecret || !creds.WebhookSecret {
			t.Fatalf("%s: OAuth provider requires client_id, client_secret, and webhook_secret admin fields", manifest.SourcePath)
		}
	case "railway", "slack":
		if manifest.Credentials == nil {
			t.Fatalf("%s: OAuth provider requires credentials", manifest.SourcePath)
		}
		creds := manifest.Credentials
		if creds.Type != "OAUTH2" || !creds.ClientID || !creds.ClientSecret {
			t.Fatalf("%s: OAuth provider requires client_id and client_secret admin fields", manifest.SourcePath)
		}
	}
}
