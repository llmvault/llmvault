use std::sync::Arc;
use std::time::Duration;

use tools::{JsonTool, ToolDefinition};

use crate::primitives::ToolCall;

pub struct ToolExecutor {
    tools: Vec<Arc<dyn JsonTool>>,
    timeout: Duration,
}

#[derive(Debug)]
pub enum ToolExecutionError {
    NotFound(String),
    MissingRequired(String),
    TimedOut { tool: String, seconds: u64 },
    Tool(anyhow::Error),
}

impl ToolExecutionError {
    pub fn raw_message(&self) -> String {
        match self {
            Self::NotFound(message) | Self::MissingRequired(message) => message.clone(),
            Self::TimedOut { tool, seconds } => {
                format!("tool `{tool}` timed out after {seconds}s")
            }
            Self::Tool(error) => error.to_string(),
        }
    }

    pub fn is_safe_argument_error(&self) -> bool {
        matches!(self, Self::MissingRequired(_))
            || matches!(self, Self::Tool(error) if is_safe_tool_argument_error(&error.to_string()))
    }
}

impl ToolExecutor {
    pub fn new(tools: Vec<Arc<dyn JsonTool>>, timeout_seconds: u32) -> Self {
        Self {
            tools,
            timeout: Duration::from_secs(timeout_seconds.max(1) as u64),
        }
    }

    pub async fn execute(
        &self,
        call: &ToolCall,
    ) -> std::result::Result<serde_json::Value, ToolExecutionError> {
        let Some(tool) = self
            .tools
            .iter()
            .find(|tool| tool.definition().name == call.name)
            .cloned()
        else {
            return Err(ToolExecutionError::NotFound(format!(
                "tool '{}' not found",
                call.name
            )));
        };
        if let Some(message) =
            missing_required_argument_message(&tool.definition(), &call.arguments)
        {
            return Err(ToolExecutionError::MissingRequired(message));
        }
        match tokio::time::timeout(self.timeout, tool.call(call.arguments.clone())).await {
            Ok(Ok(value)) => Ok(value),
            Ok(Err(error)) => Err(ToolExecutionError::Tool(error)),
            Err(_) => Err(ToolExecutionError::TimedOut {
                tool: call.name.clone(),
                seconds: self.timeout.as_secs(),
            }),
        }
    }
}

pub(crate) fn missing_required_argument_message(
    definition: &ToolDefinition,
    arguments: &serde_json::Value,
) -> Option<String> {
    let required = definition
        .parameters
        .get("required")
        .and_then(serde_json::Value::as_array)?;
    if required.is_empty() {
        return None;
    }
    let required: Vec<&str> = required
        .iter()
        .filter_map(serde_json::Value::as_str)
        .collect();
    if required.is_empty() {
        return None;
    }
    let Some(object) = arguments.as_object() else {
        return Some(format!(
            "The model produced an invalid `{}` tool call: arguments must be a JSON object with required argument(s): {}.",
            definition.name,
            required.join(", ")
        ));
    };
    let missing: Vec<&str> = required
        .iter()
        .copied()
        .filter(|field| object.get(*field).map_or(true, serde_json::Value::is_null))
        .collect();
    if missing.is_empty() {
        None
    } else {
        Some(format!(
            "The model produced an invalid `{}` tool call: missing required argument(s): {}.",
            definition.name,
            missing.join(", ")
        ))
    }
}

pub(crate) fn is_safe_tool_argument_error(error: &str) -> bool {
    error.starts_with("invalid ") && error.contains(" arguments")
}
