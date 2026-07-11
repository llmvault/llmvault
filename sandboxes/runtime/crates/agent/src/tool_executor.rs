use std::sync::Arc;
use std::time::Duration;

use tools::{JsonTool, ToolDefinition};

use crate::primitives::ToolCall;
use crate::rig_tool_registry::SUBAGENT_TOOL_CALL_ID_ARG;

const SUBAGENT_TASK_TIMEOUT: Duration = Duration::from_secs(16 * 60);

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
    // A failure from a tool that declared errors_are_safe(): the message is a
    // benign, model- and user-facing usage error and must not be redacted.
    SafeTool(anyhow::Error),
}

impl ToolExecutionError {
    pub fn raw_message(&self) -> String {
        match self {
            Self::NotFound(message) | Self::MissingRequired(message) => message.clone(),
            Self::TimedOut { tool, seconds } => {
                format!("tool `{tool}` timed out after {seconds}s")
            }
            Self::Tool(error) | Self::SafeTool(error) => error.to_string(),
        }
    }

    pub fn is_safe_argument_error(&self) -> bool {
        matches!(self, Self::MissingRequired(_))
            || matches!(self, Self::SafeTool(_))
            || matches!(self, Self::Tool(error) if is_safe_tool_argument_error(&error.to_string()))
    }
}

fn classify_tool_error(errors_are_safe: bool, error: anyhow::Error) -> ToolExecutionError {
    if errors_are_safe {
        ToolExecutionError::SafeTool(error)
    } else {
        ToolExecutionError::Tool(error)
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
        let errors_are_safe = tool.errors_are_safe();
        let mut arguments = call.arguments.clone();
        if call.name == "subagent_task" {
            if let Some(object) = arguments.as_object_mut() {
                object.insert(
                    SUBAGENT_TOOL_CALL_ID_ARG.to_string(),
                    serde_json::Value::String(call.id.clone()),
                );
            }
        }
        let execution = tool.call(arguments);
        let Some(timeout) = tool_timeout(&call.name, self.timeout) else {
            return execution
                .await
                .map_err(|error| classify_tool_error(errors_are_safe, error));
        };
        match tokio::time::timeout(timeout, execution).await {
            Ok(result) => result.map_err(|error| classify_tool_error(errors_are_safe, error)),
            Err(_) => Err(ToolExecutionError::TimedOut {
                tool: call.name.clone(),
                seconds: timeout.as_secs(),
            }),
        }
    }
}

fn tool_timeout(tool_name: &str, default_timeout: Duration) -> Option<Duration> {
    match tool_name {
        "request_user_input" => None,
        "subagent_task" => Some(SUBAGENT_TASK_TIMEOUT.max(default_timeout)),
        _ => Some(default_timeout),
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

#[cfg(test)]
mod tests {
    use super::*;

    use serde_json::{json, Value};

    struct SleepTool {
        name: &'static str,
        delay: Duration,
    }

    #[async_trait::async_trait]
    impl JsonTool for SleepTool {
        fn definition(&self) -> ToolDefinition {
            ToolDefinition {
                name: self.name.to_string(),
                description: "Sleeps before returning.".to_string(),
                parameters: json!({"type": "object"}),
            }
        }

        async fn call(&self, _args: Value) -> anyhow::Result<Value> {
            tokio::time::sleep(self.delay).await;
            Ok(json!({"ok": true}))
        }
    }

    fn executor_with(tool: SleepTool) -> ToolExecutor {
        ToolExecutor::new(vec![Arc::new(tool)], 1)
    }

    fn tool_call(name: &str) -> ToolCall {
        ToolCall {
            id: "call-1".to_string(),
            name: name.to_string(),
            arguments: json!({}),
        }
    }

    struct EchoArgumentsTool;

    #[async_trait::async_trait]
    impl JsonTool for EchoArgumentsTool {
        fn definition(&self) -> ToolDefinition {
            ToolDefinition {
                name: "subagent_task".to_string(),
                description: "Returns its arguments.".to_string(),
                parameters: json!({"type": "object"}),
            }
        }

        async fn call(&self, args: Value) -> anyhow::Result<Value> {
            Ok(args)
        }
    }

    #[tokio::test]
    async fn subagent_task_receives_its_unique_tool_call_id() {
        let executor = ToolExecutor::new(vec![Arc::new(EchoArgumentsTool)], 1);

        let result = executor
            .execute(&tool_call("subagent_task"))
            .await
            .expect("subagent tool should receive its call id");

        assert_eq!(result[SUBAGENT_TOOL_CALL_ID_ARG], "call-1");
    }

    #[tokio::test]
    async fn request_user_input_is_not_limited_by_default_tool_timeout() {
        let executor = executor_with(SleepTool {
            name: "request_user_input",
            delay: Duration::from_millis(1100),
        });

        let result = tokio::time::timeout(
            Duration::from_secs(3),
            executor.execute(&tool_call("request_user_input")),
        )
        .await
        .expect("request_user_input should keep waiting past the tool timeout")
        .expect("request_user_input should complete");

        assert_eq!(result, json!({"ok": true}));
    }

    #[tokio::test]
    async fn other_tools_still_use_default_timeout() {
        let executor = executor_with(SleepTool {
            name: "slow_tool",
            delay: Duration::from_millis(1100),
        });

        let err = executor
            .execute(&tool_call("slow_tool"))
            .await
            .expect_err("slow_tool should time out");

        assert!(matches!(
            err,
            ToolExecutionError::TimedOut { tool, seconds: 1 } if tool == "slow_tool"
        ));
    }

    struct FailingTool {
        name: &'static str,
        safe: bool,
    }

    #[async_trait::async_trait]
    impl JsonTool for FailingTool {
        fn definition(&self) -> ToolDefinition {
            ToolDefinition {
                name: self.name.to_string(),
                description: "Always fails.".to_string(),
                parameters: json!({"type": "object"}),
            }
        }

        async fn call(&self, _args: Value) -> anyhow::Result<Value> {
            Err(anyhow::anyhow!("old_text not found in file"))
        }

        fn errors_are_safe(&self) -> bool {
            self.safe
        }
    }

    #[tokio::test]
    async fn safe_tool_errors_are_classified_safe() {
        let executor = ToolExecutor::new(
            vec![Arc::new(FailingTool {
                name: "edit_file",
                safe: true,
            })],
            1,
        );

        let err = executor
            .execute(&tool_call("edit_file"))
            .await
            .expect_err("tool should error");

        assert!(matches!(err, ToolExecutionError::SafeTool(_)));
        assert!(err.is_safe_argument_error());
        assert_eq!(err.raw_message(), "old_text not found in file");
    }

    #[tokio::test]
    async fn unsafe_tool_errors_stay_redactable() {
        let executor = ToolExecutor::new(
            vec![Arc::new(FailingTool {
                name: "bash",
                safe: false,
            })],
            1,
        );

        let err = executor
            .execute(&tool_call("bash"))
            .await
            .expect_err("tool should error");

        assert!(matches!(err, ToolExecutionError::Tool(_)));
        assert!(!err.is_safe_argument_error());
    }
}
