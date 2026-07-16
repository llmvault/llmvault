package model

// PluginMCPToolDeny stores agent-specific tool opt-outs keyed by plugin UUID.
// Missing plugins and missing tool names are allowed by default; the runtime
// receives these entries as per-server deny filters for generated MCP servers.
type PluginMCPToolDeny map[string][]string
