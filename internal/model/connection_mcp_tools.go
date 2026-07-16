package model

// ConnectionMCPToolDeny stores agent-specific tool opt-outs keyed by concrete
// connection UUID. Missing connections and tool names are allowed by default.
type ConnectionMCPToolDeny map[string][]string
