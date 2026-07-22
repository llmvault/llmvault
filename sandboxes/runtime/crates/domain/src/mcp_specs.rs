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
        tool_input_bindings: Vec<ToolInputBinding>,
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
        #[serde(default)]
        tool_input_bindings: Vec<ToolInputBinding>,
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
        #[serde(default)]
        tool_input_bindings: Vec<ToolInputBinding>,
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
        #[serde(default)]
        tool_input_bindings: Vec<ToolInputBinding>,
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

    pub fn tool_input_bindings(&self) -> &[ToolInputBinding] {
        match self {
            Self::Stdio {
                tool_input_bindings,
                ..
            }
            | Self::Http {
                tool_input_bindings,
                ..
            }
            | Self::Sse {
                tool_input_bindings,
                ..
            }
            | Self::StreamableHttp {
                tool_input_bindings,
                ..
            } => tool_input_bindings,
        }
    }
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct ToolInputBinding {
    pub tool: String,
    pub kind: ToolInputBindingKind,
    pub path_argument: String,
    pub content_argument: String,
    #[serde(default)]
    pub allowed_extensions: Vec<String>,
    pub max_bytes: u64,
    pub encoding: String,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
#[serde(rename_all = "snake_case")]
pub enum ToolInputBindingKind {
    WorkspaceTextFile,
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
    use super::{McpSpec, ToolInputBindingKind};

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

    #[test]
    fn deserializes_workspace_text_file_binding_without_allowed_roots() {
        let spec: McpSpec = serde_json::from_value(serde_json::json!({
            "transport": "streamable_http",
            "name": "hivy",
            "url": "https://mcp.example.test",
            "tool_input_bindings": [{
                "tool": "send_email",
                "kind": "workspace_text_file",
                "path_argument": "markdown_file_path",
                "content_argument": "markdown",
                "allowed_extensions": [".md", ".markdown"],
                "max_bytes": 1048576,
                "encoding": "utf-8"
            }]
        }))
        .expect("deserialize workspace text file binding");

        let binding = &spec.tool_input_bindings()[0];
        assert_eq!(binding.kind, ToolInputBindingKind::WorkspaceTextFile);
        assert_eq!(binding.max_bytes, 1_048_576);
    }
}
