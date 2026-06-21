use std::future::Future;
use std::path::PathBuf;
use std::pin::Pin;
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::Arc;
use std::time::{Duration, Instant};

use anyhow::{anyhow, Result};
use async_trait::async_trait;
use chrono::{DateTime, Utc};
use domain::agent_registry::AgentDefinitionRegistry;
use domain::{
    event_types, validate_request_user_input_payload, validate_update_plan_payload, OutboundEvent,
    RequestUserInputPayload, SessionId, SubagentTask, SubagentTaskConfig, SubagentTaskState,
    ToolSpec, UpdatePlanPayload,
};
use mcp::McpRegistry;
use outbound::OutboundEmitter;
use serde_json::{json, Value};
use storage::{EventRepo, SubagentTaskRepo};
use tools::{JsonTool, ProcessRegistry, ToolDefinition};

use crate::{PlanUpdater, QuestionRequester};

pub type ToolFuture = Pin<Box<dyn Future<Output = Result<Value>> + Send>>;

static SUBAGENT_TASK_SEQUENCE: AtomicU64 = AtomicU64::new(1);
const SUBAGENT_TASK_WAIT_INTERVAL: Duration = Duration::from_millis(250);
const SUBAGENT_TASK_FOREGROUND_TIMEOUT: Duration = Duration::from_secs(15 * 60);

pub struct DynamicTool {
    definition: ToolDefinition,
    executor: Arc<dyn Fn(Value) -> ToolFuture + Send + Sync>,
}

impl DynamicTool {
    pub fn new(
        definition: ToolDefinition,
        executor: impl Fn(Value) -> ToolFuture + Send + Sync + 'static,
    ) -> Self {
        Self {
            definition,
            executor: Arc::new(executor),
        }
    }
}

#[async_trait]
impl JsonTool for DynamicTool {
    fn definition(&self) -> ToolDefinition {
        self.definition.clone()
    }

    async fn call(&self, args: Value) -> Result<Value> {
        (self.executor)(args).await
    }
}

pub struct ToolContext {
    pub subagent_task_repo: Option<Arc<dyn SubagentTaskRepo>>,
    pub event_repo: Option<Arc<dyn EventRepo>>,
    pub process_registry: Option<Arc<ProcessRegistry>>,
    pub question_requester: Option<Arc<dyn QuestionRequester>>,
    pub plan_updater: Option<Arc<dyn PlanUpdater>>,
    pub mcp_registry: Option<Arc<McpRegistry>>,
    pub workspace_root: PathBuf,
    pub outbound_emitter: Option<Arc<OutboundEmitter>>,
    pub agent_registry: Arc<AgentDefinitionRegistry>,
    pub session_stream_id: Option<String>,
}

pub fn build_agent_tools(
    specs: &[ToolSpec],
    session_id: &SessionId,
    ctx: &ToolContext,
) -> Vec<Arc<dyn JsonTool>> {
    let mut tools: Vec<Arc<dyn JsonTool>> = Vec::new();
    let session_is_cron = session_id.as_str().contains("-cron-");

    for spec in specs {
        match spec {
            ToolSpec::CheckBashStatus => {
                if let Some(registry) = &ctx.process_registry {
                    tools.push(check_bash_status_tool(registry.clone()));
                }
            }
            ToolSpec::SearchSessions => {
                if let Some(repo) = &ctx.event_repo {
                    tools.push(search_sessions_tool(repo.clone()));
                }
            }
            ToolSpec::SkillsList => {
                tools.push(skills_list_tool(ctx.workspace_root.clone()));
            }
            ToolSpec::SkillView => {
                tools.push(skill_view_tool(ctx.workspace_root.clone()));
            }
            ToolSpec::SkillManage => {
                tools.push(skill_manage_tool(
                    ctx.workspace_root.clone(),
                    session_id.clone(),
                    ctx.outbound_emitter.clone(),
                ));
            }
            ToolSpec::SubagentTask(config) => {
                if let Some(repo) = &ctx.subagent_task_repo {
                    if !session_is_cron {
                        tools.push(subagent_task_tool(
                            repo.clone(),
                            session_id.clone(),
                            ctx.agent_registry.clone(),
                            config.clone(),
                            ctx.session_stream_id.clone(),
                        ));
                    }
                }
            }
            ToolSpec::RequestUserInput => {
                if let Some(requester) = &ctx.question_requester {
                    if !session_is_cron {
                        tools.push(request_user_input_tool(
                            requester.clone(),
                            session_id.clone(),
                        ));
                    }
                }
            }
            ToolSpec::UpdatePlan => {
                if let Some(updater) = &ctx.plan_updater {
                    if !session_is_cron {
                        tools.push(update_plan_tool(updater.clone(), session_id.clone()));
                    }
                }
            }
            _ => {}
        }
    }

    tools
}

pub async fn emit_tool_invoked(
    _emitter: Option<Arc<OutboundEmitter>>,
    _session_id: &SessionId,
    _tool: &str,
    _args: &Value,
    _result: &Value,
) {
}

pub async fn emit_tool_error(
    _emitter: Option<Arc<OutboundEmitter>>,
    _session_id: &SessionId,
    _tool: &str,
    _args: &Value,
    _error: &str,
) {
}

fn event_source_from_session(session_id: &SessionId) -> &'static str {
    if session_id.as_str().starts_with("subagent-") {
        "subagent"
    } else {
        "session"
    }
}

fn request_user_input_tool(
    requester: Arc<dyn QuestionRequester>,
    session_id: SessionId,
) -> Arc<dyn JsonTool> {
    Arc::new(DynamicTool::new(
        ToolDefinition {
            name: "request_user_input".into(),
            description:
                "Ask the user one to three structured questions and wait until they answer.".into(),
            parameters: json!({
                "type": "object",
                "properties": {
                    "questions": {
                        "type": "array",
                        "minItems": 1,
                        "maxItems": 3,
                        "items": {
                            "type": "object",
                            "properties": {
                                "id": { "type": "string" },
                                "header": {
                                    "type": "string",
                                    "description": "Short UI header, 12 or fewer characters."
                                },
                                "question": { "type": "string" },
                                "options": {
                                    "type": "array",
                                    "minItems": 2,
                                    "maxItems": 3,
                                    "items": {
                                        "type": "object",
                                        "properties": {
                                            "label": { "type": "string" },
                                            "description": { "type": "string" }
                                        },
                                        "required": ["label", "description"]
                                    }
                                }
                            },
                            "required": ["id", "header", "question", "options"]
                        }
                    }
                },
                "required": ["questions"]
            }),
        },
        move |args| {
            let requester = requester.clone();
            let session_id = session_id.clone();
            Box::pin(async move {
                let payload: RequestUserInputPayload = serde_json::from_value(args)
                    .map_err(|error| anyhow!("invalid request_user_input arguments: {error}"))?;
                validate_request_user_input_payload(&payload)
                    .map_err(|error| anyhow!("invalid request_user_input arguments: {error}"))?;
                let (question_request_id, answer) =
                    requester.request_user_input(&session_id, payload).await?;
                Ok(json!({
                    "question_request_id": question_request_id,
                    "answers": answer.answers,
                }))
            })
        },
    ))
}

fn update_plan_tool(updater: Arc<dyn PlanUpdater>, session_id: SessionId) -> Arc<dyn JsonTool> {
    Arc::new(DynamicTool::new(
        ToolDefinition {
            name: "update_plan".into(),
            description: "Replace the current visible task plan with a concise checklist of steps."
                .into(),
            parameters: json!({
                "type": "object",
                "additionalProperties": false,
                "properties": {
                    "explanation": {
                        "type": ["string", "null"],
                        "description": "Optional short reason or context for this plan update."
                    },
                    "plan": {
                        "type": "array",
                        "minItems": 1,
                        "items": {
                            "type": "object",
                            "additionalProperties": false,
                            "properties": {
                                "step": { "type": "string" },
                                "status": {
                                    "type": "string",
                                    "enum": ["pending", "in_progress", "completed"]
                                }
                            },
                            "required": ["step", "status"]
                        }
                    }
                },
                "required": ["plan"]
            }),
        },
        move |args| {
            let updater = updater.clone();
            let session_id = session_id.clone();
            Box::pin(async move {
                let payload: UpdatePlanPayload = serde_json::from_value(args)
                    .map_err(|error| anyhow!("invalid update_plan arguments: {error}"))?;
                validate_update_plan_payload(&payload)
                    .map_err(|error| anyhow!("invalid update_plan arguments: {error}"))?;
                let result = updater.update_plan(&session_id, payload).await?;
                serde_json::to_value(result)
                    .map_err(|error| anyhow!("serialize plan result: {error}"))
            })
        },
    ))
}

fn search_sessions_tool(repo: Arc<dyn EventRepo>) -> Arc<dyn JsonTool> {
    Arc::new(DynamicTool::new(
        ToolDefinition {
            name: "search_sessions".into(),
            description: "Search recent local conversation history from this sandbox. Use it to find prior user messages, agent replies, and compact tool summaries before relying on older context.".into(),
            parameters: json!({
                "type": "object",
                "properties": {
                    "query": {
                        "type": "string",
                        "description": "Search terms for recent conversations"
                    },
                    "session_id": {
                        "type": "string",
                        "description": "Optional exact session id to search within"
                    },
                    "limit": {
                        "type": "integer",
                        "description": "Maximum matches to return, default 8, max 20"
                    }
                },
                "required": ["query"]
            }),
        },
        move |args| {
            let repo = repo.clone();
            Box::pin(async move {
                let query = args
                    .get("query")
                    .and_then(Value::as_str)
                    .map(str::trim)
                    .filter(|value| !value.is_empty())
                    .ok_or_else(|| anyhow!("query required"))?;
                let limit = args
                    .get("limit")
                    .and_then(Value::as_u64)
                    .unwrap_or(8)
                    .clamp(1, 20) as u32;
                let session_id = args
                    .get("session_id")
                    .and_then(Value::as_str)
                    .map(str::trim)
                    .filter(|value| !value.is_empty())
                    .map(SessionId::from);
                let matches = repo
                    .search_sessions(query, session_id.as_ref(), limit)
                    .await?;
                let items: Vec<Value> = matches
                    .into_iter()
                    .map(|item| {
                        json!({
                            "session_id": item.session_id,
                            "event_id": item.event_id,
                            "event_kind": item.kind,
                            "created_at": item.created_at.to_rfc3339(),
                            "score": item.score,
                            "snippet": item.snippet,
                            "text": truncate_search_text(&item.content, 700),
                        })
                    })
                    .collect();
                Ok(json!({ "matches": items }))
            })
        },
    ))
}

fn truncate_search_text(value: &str, max_chars: usize) -> String {
    let mut out = String::new();
    for ch in value.chars().take(max_chars) {
        out.push(ch);
    }
    out
}

fn skills_list_tool(workspace_root: PathBuf) -> Arc<dyn JsonTool> {
    Arc::new(DynamicTool::new(
        ToolDefinition {
            name: "skills_list".into(),
            description:
                "List available skills (name + description). Use skill_view(name) to load full content."
                    .into(),
            parameters: json!({
                "type": "object",
                "properties": {
                    "category": {
                        "type": "string",
                        "description": "Optional category filter to narrow results"
                    }
                },
                "required": []
            }),
        },
        move |args| {
            let workspace_root = workspace_root.clone();
            Box::pin(async move {
                let store = skills::SkillStore::new(workspace_root);
                Ok(store.list(args.get("category").and_then(Value::as_str)))
            })
        },
    ))
}

fn skill_view_tool(workspace_root: PathBuf) -> Arc<dyn JsonTool> {
    Arc::new(DynamicTool::new(
        ToolDefinition {
            name: "skill_view".into(),
            description: "Skills allow loading task workflows plus linked files. Load a skill's full content or access linked files under references/, templates/, scripts/, or assets/.".into(),
            parameters: json!({
                "type": "object",
                "properties": {
                    "name": {
                        "type": "string",
                        "description": "The skill name (use skills_list to see available skills)."
                    },
                    "file_path": {
                        "type": "string",
                        "description": "Optional linked file path within the skill, e.g. references/api.md or scripts/check.sh."
                    }
                },
                "required": ["name"]
            }),
        },
        move |args| {
            let workspace_root = workspace_root.clone();
            Box::pin(async move {
                let name = args
                    .get("name")
                    .and_then(Value::as_str)
                    .ok_or_else(|| anyhow!("name required"))?;
                let store = skills::SkillStore::new(workspace_root);
                store.view(name, args.get("file_path").and_then(Value::as_str))
            })
        },
    ))
}

fn skill_manage_tool(
    workspace_root: PathBuf,
    session_id: SessionId,
    emitter: Option<Arc<OutboundEmitter>>,
) -> Arc<dyn JsonTool> {
    Arc::new(DynamicTool::new(
        ToolDefinition {
            name: "skill_manage".into(),
            description: "Manage filesystem-backed skills in /workspace/.skills. Actions: create, patch, edit, delete, write_file, remove_file. Use only when asked, or after confirming the user wants to save/update durable skill instructions.".into(),
            parameters: json!({
                "type": "object",
                "properties": {
                    "action": {"type": "string", "enum": ["create", "patch", "edit", "delete", "write_file", "remove_file"]},
                    "name": {"type": "string", "description": "Skill name: lowercase, numbers, hyphens/underscores, max 64 chars."},
                    "content": {"type": "string", "description": "Full SKILL.md content. Required for create and edit."},
                    "old_string": {"type": "string", "description": "Text to find for patch."},
                    "new_string": {"type": "string", "description": "Replacement text for patch."},
                    "replace_all": {"type": "boolean", "description": "For patch, replace all matches instead of requiring uniqueness."},
                    "category": {"type": "string", "description": "Optional category for create when content has no frontmatter."},
                    "file_path": {"type": "string", "description": "Supporting file path under references/, templates/, scripts/, or assets/."},
                    "file_content": {"type": "string", "description": "Content for write_file."},
                    "absorbed_into": {"type": "string", "description": "For delete, skill this was merged into, or empty string for pruning."}
                },
                "required": ["action", "name"]
            }),
        },
        move |args| {
            let workspace_root = workspace_root.clone();
            let session_id = session_id.clone();
            let emitter = emitter.clone();
            Box::pin(async move {
                let store = skills::SkillStore::new(workspace_root);
                let action = args
                    .get("action")
                    .and_then(Value::as_str)
                    .unwrap_or_default()
                    .to_string();
                let name = args
                    .get("name")
                    .and_then(Value::as_str)
                    .unwrap_or_default()
                    .to_string();
                let absorbed_into = args
                    .get("absorbed_into")
                    .and_then(Value::as_str)
                    .map(ToString::to_string);
                let result = store.manage(skills::SkillManageArgs {
                    action: action.clone(),
                    name: name.clone(),
                    content: args.get("content").and_then(Value::as_str).map(ToString::to_string),
                    category: args.get("category").and_then(Value::as_str).map(ToString::to_string),
                    file_path: args.get("file_path").and_then(Value::as_str).map(ToString::to_string),
                    file_content: args.get("file_content").and_then(Value::as_str).map(ToString::to_string),
                    old_string: args.get("old_string").and_then(Value::as_str).map(ToString::to_string),
                    new_string: args.get("new_string").and_then(Value::as_str).map(ToString::to_string),
                    replace_all: args.get("replace_all").and_then(Value::as_bool).unwrap_or(false),
                    absorbed_into: absorbed_into.clone(),
                })?;
                emit_skill_synced(emitter, &session_id, &store, &action, &name, absorbed_into).await;
                Ok(result)
            })
        },
    ))
}

async fn emit_skill_synced(
    emitter: Option<Arc<OutboundEmitter>>,
    session_id: &SessionId,
    store: &skills::SkillStore,
    action: &str,
    name: &str,
    absorbed_into: Option<String>,
) {
    let Some(emitter) = emitter else { return };
    let mut payload = json!({
        "session_id": session_id.as_str(),
        "source": event_source_from_session(session_id),
        "action": action,
        "name": name,
    });
    if action == "delete" {
        payload["deleted"] = Value::Bool(true);
        if let Some(absorbed_into) = absorbed_into {
            payload["absorbed_into"] = Value::String(absorbed_into);
        }
    } else {
        match store.sync_snapshot(name) {
            Ok(snapshot) => {
                if let Some(obj) = snapshot.as_object() {
                    for (key, value) in obj {
                        payload[key] = value.clone();
                    }
                }
            }
            Err(error) => {
                tracing::warn!(%error, skill = %name, "skill sync snapshot failed");
                return;
            }
        }
    }
    emitter
        .emit(OutboundEvent::new(event_types::SKILL_SYNCED, payload))
        .await;
}

fn check_bash_status_tool(registry: Arc<ProcessRegistry>) -> Arc<dyn JsonTool> {
    Arc::new(DynamicTool::new(
        ToolDefinition {
            name: "check_bash_status".into(),
            description: "Check the status of a background bash process. Pass cursor from the previous response to receive only new output.".into(),
            parameters: json!({
                "type":"object",
                "properties":{
                    "process_id":{"type":"string"},
                    "cursor":{"type":"integer","description":"Optional output cursor returned by the previous status response."}
                },
                "required":["process_id"]
            }),
        },
        move |args| {
            let registry = registry.clone();
            Box::pin(async move {
                let id = args
                    .get("process_id")
                    .and_then(Value::as_str)
                    .ok_or_else(|| anyhow!("process_id required"))?;
                let cursor = args.get("cursor").and_then(Value::as_u64).map(|value| value as usize);
                let status = registry
                    .status(id, cursor)
                    .ok_or_else(|| anyhow!("process not found"))?;
                let mut result = json!({
                    "process_id": id,
                    "running": status.running,
                    "exit_code": status.exit_code,
                    "output": status.output,
                    "next_cursor": status.next_cursor,
                    "truncated": status.truncated,
                });
                if status.running {
                    result["_hint"] = serde_json::json!(format!(
                        "This process is still running. Check again later with \
                         check_bash_status and pass cursor={} so only new output is returned.",
                        status.next_cursor
                    ));
                }
                Ok(result)
            })
        },
    ))
}

fn build_agent_list_description(
    registry: &AgentDefinitionRegistry,
    allowlist: &[String],
) -> String {
    let agents = if allowlist.is_empty() {
        registry.available_agents()
    } else {
        allowlist.to_vec()
    };
    let mut parts = Vec::new();
    for name in &agents {
        let desc = registry.agent_description(name);
        parts.push(format!("{} - {}", name, desc));
    }
    format!("Sub-agent name. Available: {}", parts.join(", "))
}

fn subagent_task_tool(
    repo: Arc<dyn SubagentTaskRepo>,
    session_id: SessionId,
    agent_registry: Arc<AgentDefinitionRegistry>,
    config: SubagentTaskConfig,
    session_stream_id: Option<String>,
) -> Arc<dyn JsonTool> {
    let agent_desc = build_agent_list_description(&agent_registry, &config.agents);
    let agent_names: Vec<String> = if config.agents.is_empty() {
        agent_registry.available_agents()
    } else {
        config.agents.clone()
    };
    let agent_names_clone = agent_names.clone();
    let config_agents = config.agents.clone();

    Arc::new(DynamicTool::new(
        ToolDefinition {
            name: "subagent_task".into(),
            description: "Run a task with a configured subagent in an isolated foreground session."
                .into(),
            parameters: json!({
                "type": "object",
                "properties": {
                    "goal": {
                        "type": "string",
                        "description": "The task for the subagent."
                    },
                    "agent": {
                        "type": "string",
                        "description": agent_desc
                    }
                },
                "required": ["goal", "agent"]
            }),
        },
        move |args| {
            let repo = repo.clone();
            let session_id = session_id.clone();
            let agent_registry = agent_registry.clone();
            let agent_names = agent_names_clone.clone();
            let config_agents = config_agents.clone();
            let session_stream_id = session_stream_id.clone();
            Box::pin(async move {
                let goal = args
                    .get("goal")
                    .and_then(Value::as_str)
                    .ok_or_else(|| anyhow!("goal required"))?
                    .to_string();
                let agent_name = args
                    .get("agent")
                    .and_then(Value::as_str)
                    .ok_or_else(|| anyhow!("agent required"))?
                    .to_string();

                if !config_agents.is_empty() && !config_agents.contains(&agent_name) {
                    return Err(anyhow!(
                        "agent '{}' not in allowlist. Allowed: {:?}",
                        agent_name,
                        config_agents
                    ));
                }
                if agent_registry.resolve(&agent_name).is_none() {
                    return Err(anyhow!(
                        "unknown agent '{}'. Available: {:?}",
                        agent_name,
                        agent_names
                    ));
                }

                let now = Utc::now();
                let id = next_subagent_task_id(now);
                let child_session_id = SessionId::from(format!("subagent-{}", id));

                let task = SubagentTask {
                    id: id.clone(),
                    parent_session_id: session_id.clone(),
                    child_session_id: child_session_id.clone(),
                    agent_name: agent_name.clone(),
                    goal,
                    stream_id: session_stream_id.clone(),
                    state: SubagentTaskState::Queued,
                    result: None,
                    error: None,
                    created_at: now,
                    started_at: None,
                    completed_at: None,
                    updated_at: now,
                };
                repo.create(&task).await?;
                let completed =
                    wait_for_subagent_task_completion(repo.clone(), &id, &agent_name).await?;
                let result = completed.result.clone().unwrap_or_default();
                Ok(json!({
                    "job_id": id,
                    "state": subagent_task_state_string(completed.state),
                    "session_id": child_session_id.as_str(),
                    "agent": agent_name,
                    "result": result,
                    "output": result,
                    "message": format!("Subagent '{}' completed.", agent_name),
                    "system_reminder": "Subagent tasks run in isolated sessions and may not share this session's local filesystem. Use Drive or another explicit shared location for artifacts that must be inspected or shared."
                }))
            })
        },
    ))
}

fn next_subagent_task_id(now: DateTime<Utc>) -> String {
    let sequence = SUBAGENT_TASK_SEQUENCE.fetch_add(1, Ordering::Relaxed);
    format!("subagent-task-{}-{}", now.timestamp_millis(), sequence)
}

async fn wait_for_subagent_task_completion(
    repo: Arc<dyn SubagentTaskRepo>,
    id: &str,
    agent_name: &str,
) -> Result<SubagentTask> {
    let start = Instant::now();
    loop {
        let task = repo
            .get(id)
            .await?
            .ok_or_else(|| anyhow!("subagent task '{}' not found", id))?;
        match task.state {
            SubagentTaskState::Completed => return Ok(task),
            SubagentTaskState::Failed => {
                let error = task
                    .error
                    .clone()
                    .unwrap_or_else(|| "subagent failed without an error message".to_string());
                return Err(anyhow!("subagent '{}' failed: {}", agent_name, error));
            }
            SubagentTaskState::Queued | SubagentTaskState::Running => {}
        }

        if start.elapsed() >= SUBAGENT_TASK_FOREGROUND_TIMEOUT {
            let message = format!(
                "subagent '{}' timed out after {} seconds",
                agent_name,
                SUBAGENT_TASK_FOREGROUND_TIMEOUT.as_secs()
            );
            let _ = repo
                .complete(
                    id,
                    SubagentTaskState::Failed,
                    Utc::now(),
                    "",
                    Some(&message),
                )
                .await;
            return Err(anyhow!(message));
        }
        tokio::time::sleep(SUBAGENT_TASK_WAIT_INTERVAL).await;
    }
}

fn subagent_task_state_string(state: SubagentTaskState) -> &'static str {
    match state {
        SubagentTaskState::Queued => "queued",
        SubagentTaskState::Running => "running",
        SubagentTaskState::Completed => "completed",
        SubagentTaskState::Failed => "failed",
    }
}

#[cfg(test)]
#[path = "rig_tool_registry_tests.rs"]
mod tests;
