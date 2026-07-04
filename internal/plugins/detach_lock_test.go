package plugins

import (
	"testing"

	"github.com/usehivy/hivy/internal/model"
)

// PluginDetachLock is the single policy every plugin-removal path consults. These
// cases pin the rules and their precedence so the UI disable endpoint and the MCP
// update_agent reconcile can never drift apart.
func TestPluginDetachLock(t *testing.T) {
	plugin := func(slug, manifest string) model.Plugin {
		return model.Plugin{Slug: slug, Manifest: model.RawJSON(manifest)}
	}

	cases := []struct {
		name           string
		plugin         model.Plugin
		agentIsDefault bool
		catalogReq     []string
		wantLocked     bool
		wantReason     string
	}{
		{
			name:   "ordinary plugin on ordinary agent is removable",
			plugin: plugin("github", `{}`),
		},
		{
			name:       "auto_install is locked everywhere",
			plugin:     plugin("runtime", `{"auto_install":true}`),
			wantLocked: true,
			wantReason: detachReasonAutoInstall,
		},
		{
			name:       "manifest locked is locked",
			plugin:     plugin("sheets", `{"locked":true}`),
			wantLocked: true,
			wantReason: detachReasonLocked,
		},
		{
			name:           "default_agent_install is locked ON the default agent",
			plugin:         plugin("agent-builder", `{"default_agent_install":true}`),
			agentIsDefault: true,
			wantLocked:     true,
			wantReason:     detachReasonDefaultAgent,
		},
		{
			name:           "default_agent_install is removable on a NON-default agent",
			plugin:         plugin("agent-builder", `{"default_agent_install":true}`),
			agentIsDefault: false,
			wantLocked:     false,
		},
		{
			name:       "catalog-required plugin is locked on the requiring agent",
			plugin:     plugin("github", `{}`),
			catalogReq: []string{"linear", "github"},
			wantLocked: true,
			wantReason: detachReasonCatalog,
		},
		{
			name:       "not-required plugin with a catalog is removable",
			plugin:     plugin("slack", `{}`),
			catalogReq: []string{"github"},
			wantLocked: false,
		},
		{
			name:           "auto_install takes precedence over default-agent and catalog",
			plugin:         plugin("runtime", `{"auto_install":true,"default_agent_install":true}`),
			agentIsDefault: true,
			catalogReq:     []string{"runtime"},
			wantLocked:     true,
			wantReason:     detachReasonAutoInstall,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			locked, reason := PluginDetachLock(tc.plugin, tc.agentIsDefault, tc.catalogReq)
			if locked != tc.wantLocked {
				t.Fatalf("locked = %v, want %v (reason %q)", locked, tc.wantLocked, reason)
			}
			if locked && reason != tc.wantReason {
				t.Fatalf("reason = %q, want %q", reason, tc.wantReason)
			}
			if !locked && reason != "" {
				t.Fatalf("reason = %q, want empty when not locked", reason)
			}
		})
	}
}
