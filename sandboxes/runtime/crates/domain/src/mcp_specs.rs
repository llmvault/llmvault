use serde::{Deserialize, Serialize};
use std::collections::HashMap;

#[derive(Debug, Clone, Serialize, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
#[serde(tag = "transport", rename_all = "snake_case")]
pub enum McpSpec {
    Stdio {
        name: String,
        command: String,
        #[serde(default)]
        args: Vec<String>,
        #[serde(default)]
        env: HashMap<String, String>,
        #[serde(default)]
        tool_filter: Option<ToolFilter>,
        #[serde(default)]
        tool_name_prefix: Option<String>,
        #[serde(default)]
        startup_timeout_seconds: Option<u32>,
    },
    Http {
        name: String,
        url: String,
        #[serde(default)]
        headers: HashMap<String, String>,
        #[serde(default)]
        tool_filter: Option<ToolFilter>,
        #[serde(default)]
        tool_name_prefix: Option<String>,
    },
    /// Legacy MCP HTTP+SSE transport (protocol version 2024-11-05). The
    /// configured URL is the long-lived GET event stream; the server announces
    /// the per-session POST endpoint with an `event: endpoint` frame.
    Sse {
        name: String,
        url: String,
        #[serde(default)]
        headers: HashMap<String, String>,
        #[serde(default)]
        tool_filter: Option<ToolFilter>,
        #[serde(default)]
        tool_name_prefix: Option<String>,
    },
    StreamableHttp {
        name: String,
        url: String,
        #[serde(default)]
        headers: HashMap<String, String>,
        #[serde(default)]
        tool_filter: Option<ToolFilter>,
        #[serde(default)]
        tool_name_prefix: Option<String>,
    },
}

impl McpSpec {
    pub fn name(&self) -> &str {
        match self {
            Self::Stdio { name, .. }
            | Self::Http { name, .. }
            | Self::Sse { name, .. }
            | Self::StreamableHttp { name, .. } => name,
        }
    }

    pub fn tool_name_prefix(&self) -> Option<&str> {
        match self {
            Self::Stdio {
                tool_name_prefix, ..
            }
            | Self::Http {
                tool_name_prefix, ..
            }
            | Self::Sse {
                tool_name_prefix, ..
            }
            | Self::StreamableHttp {
                tool_name_prefix, ..
            } => tool_name_prefix.as_deref(),
        }
    }
}

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct ToolFilter {
    #[serde(default)]
    pub allow: Option<Vec<String>>,
    #[serde(default)]
    pub deny: Option<Vec<String>>,
}

#[cfg(test)]
mod tests {
    use super::McpSpec;

    #[test]
    fn deserializes_legacy_sse_transport_from_control_plane() {
        let spec: McpSpec = serde_json::from_value(serde_json::json!({
            "transport": "sse",
            "name": "legacy",
            "url": "http://127.0.0.1:3000/sse",
            "headers": {"Authorization": "Bearer token"}
        }))
        .expect("deserialize legacy SSE spec");

        assert!(matches!(spec, McpSpec::Sse { name, .. } if name == "legacy"));
    }

    #[test]
    fn deserializes_model_facing_tool_name_prefix() {
        let spec: McpSpec = serde_json::from_value(serde_json::json!({
            "transport": "streamable_http",
            "name": "database-postgres",
            "url": "https://mcp.example.test/database/postgres",
            "tool_name_prefix": "postgres_primary"
        }))
        .expect("deserialize tool name prefix");

        assert_eq!(spec.tool_name_prefix(), Some("postgres_primary"));
    }
}
