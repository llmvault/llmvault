package plugins

import (
	"slices"

	"github.com/usehivy/hivy/internal/model"
)

// Detach-lock reason messages. Callers surface these verbatim to users (HTTP 409
// bodies, MCP error text), so keep them stable.
const (
	detachReasonAutoInstall  = "plugin is installed for all agents and cannot be disabled"
	detachReasonLocked       = "plugin is required and cannot be disabled"
	detachReasonDefaultAgent = "plugin is part of the default Hivy agent and cannot be disabled"
	detachReasonCatalog      = "plugin is required for this agent and cannot be disabled"
)

// PluginDetachLock is the single source of truth for whether a plugin may be
// removed from an agent. EVERY plugin-removal path (the UI disable endpoint, the
// MCP update_agent reconcile) must consult it so protection is identical no
// matter how the removal is attempted.
//
// A plugin is locked to the agent when it is any of:
//   - auto-installed org-wide (installed on every agent),
//   - manifest "locked",
//   - a default_agent_install plugin AND the target is the org's default (Hivy)
//     agent, or
//   - required by the agent's catalog entry.
//
// It returns the user-facing reason for the first rule that matches.
// catalogRequired is the agent's catalog required-plugin slugs (nil for agents
// with no catalog); agentIsDefault is the agent's is_default flag.
func PluginDetachLock(plugin model.Plugin, agentIsDefault bool, catalogRequired []string) (locked bool, reason string) {
	if PluginAutoInstall(plugin) {
		return true, detachReasonAutoInstall
	}
	if PluginLocked(plugin) { // manifest "locked" (auto_install already handled above)
		return true, detachReasonLocked
	}
	if agentIsDefault && PluginDefaultAgentInstall(plugin) {
		return true, detachReasonDefaultAgent
	}
	if slices.Contains(catalogRequired, plugin.Slug) {
		return true, detachReasonCatalog
	}
	return false, ""
}
