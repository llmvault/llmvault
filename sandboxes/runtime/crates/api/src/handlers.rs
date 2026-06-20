use axum::{
    extract::{Path, Query, State},
    http::{
        header::{CACHE_CONTROL, CONNECTION},
        HeaderName, HeaderValue, StatusCode,
    },
    response::{IntoResponse, Response},
    Json,
};
use chrono::{DateTime, Utc};
use domain::cron::CronJob;
use domain::{AgentDefinition, QuestionAnswerPayload, SessionId, SessionStatus};
use serde::{Deserialize, Serialize};
use std::collections::HashMap;
use std::fmt::Display;
use std::path::{Path as FsPath, PathBuf};
use std::time::Duration;
use storage::{SessionListCursor, SessionListFilter};
use tools::{BashExecOptions, BashOperations};
use tracing::warn;

use crate::question_manager::{QuestionAnswerError, QuestionAnswerResponse};
use crate::session_stream::{
    stream_response, SessionMessageRequest, SessionMessageResponse, StreamReplayMode,
};
use crate::state::ApiState;

#[derive(Serialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct ConfigResponse {
    applied_at: DateTime<Utc>,
    definition: AgentDefinition,
    env_key_count: usize,
    secret_rotated: bool,
}

#[derive(Serialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct HealthResponse {
    status: &'static str,
    sentry_enabled: bool,
    sentry_dsn_set: bool,
}

#[derive(Serialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct SessionInterruptResponse {
    pub interrupted: bool,
    pub status: &'static str,
}

const MAX_RUNTIME_ENV_KEYS: usize = 128;
const MAX_RUNTIME_ENV_KEY_LENGTH: usize = 128;
const MAX_RUNTIME_ENV_VALUE_LENGTH: usize = 8192;
const MAX_RUNTIME_ENV_PAYLOAD_BYTES: usize = 64 * 1024;
const MAX_CONFIG_SCHEDULES: usize = 128;
const MAX_CONFIG_SCHEDULE_ID_LENGTH: usize = 255;
const MAX_CONFIG_SCHEDULE_TEXT_LENGTH: usize = 64 * 1024;
const MAX_CONTROL_COMMANDS: usize = 20;
const MAX_CONTROL_COMMAND_LENGTH: usize = 8 * 1024;
const MAX_CONTROL_COMMAND_PAYLOAD_BYTES: usize = 128 * 1024;
const MAX_CONTROL_COMMAND_TIMEOUT_SECONDS: u64 = 300;
const MAX_CONTROL_COMMAND_OUTPUT_BYTES: u64 = 64 * 1024;
const INTERNAL_RETRY_MESSAGE: &str =
    "An internal runtime error occurred. Please retry. The Hivy team has been notified of this error.";

#[derive(Debug, Default, Deserialize)]
pub struct SessionStreamQuery {
    replay: Option<String>,
    after_seq: Option<String>,
}

impl SessionStreamQuery {
    fn replay_mode(&self) -> Result<StreamReplayMode, String> {
        let replay = self.replay.as_deref().unwrap_or("all");
        let after_seq = self
            .after_seq
            .as_deref()
            .map(|value| {
                value
                    .parse::<u64>()
                    .map_err(|_| "after_seq must be an unsigned integer".to_string())
            })
            .transpose()?;

        match replay {
            "all" => {}
            "none" => {
                if after_seq.is_some() {
                    return Err("replay=none cannot be combined with after_seq".to_string());
                }
            }
            _ => {
                return Err("replay must be one of: all, none".to_string());
            }
        }

        Ok(match after_seq {
            Some(sequence) => StreamReplayMode::AfterSeq(sequence),
            None if replay == "none" => StreamReplayMode::None,
            None => StreamReplayMode::All,
        })
    }
}

fn capture_internal_error(context: &'static str, error: impl Display) {
    let error = error.to_string();
    warn!(context, error = %error, "runtime API internal error captured");
    sentry::with_scope(
        |scope| {
            scope.set_tag("runtime.api_context", context);
            scope.set_extra("internal_error", error.into());
        },
        || sentry::capture_message("runtime API internal error", sentry::Level::Error),
    );
}

fn internal_error_response(context: &'static str, error: impl Display) -> (StatusCode, String) {
    capture_internal_error(context, error);
    (
        StatusCode::INTERNAL_SERVER_ERROR,
        INTERNAL_RETRY_MESSAGE.to_string(),
    )
}

#[derive(Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct ConfigUpdateRequest {
    #[serde(default)]
    pub runtime_secret: Option<String>,
    #[serde(default)]
    pub runtime_env: HashMap<String, String>,
    pub definition: AgentDefinition,
    #[serde(default)]
    pub schedules: Vec<CronJob>,
}

#[derive(Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct ControlCommandsRequest {
    pub commands: Vec<String>,
    #[serde(default)]
    pub workdir: Option<String>,
    #[serde(default)]
    pub timeout_seconds: Option<u64>,
    #[serde(default = "default_stop_on_error")]
    pub stop_on_error: bool,
}

#[derive(Serialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct ControlCommandsResponse {
    pub ok: bool,
    pub results: Vec<ControlCommandResult>,
}

#[derive(Serialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct ControlCommandResult {
    pub command: String,
    pub exit_code: Option<i32>,
    pub timed_out: bool,
    pub truncated: bool,
    pub output: String,
}

fn default_stop_on_error() -> bool {
    true
}

impl ConfigUpdateRequest {
    fn validate(&self) -> Result<(), String> {
        if self.runtime_env.len() > MAX_RUNTIME_ENV_KEYS {
            return Err(format!(
                "too many environment variables; max {}",
                MAX_RUNTIME_ENV_KEYS
            ));
        }

        for (key, value) in &self.runtime_env {
            if !is_valid_env_key(key) {
                return Err(format!("invalid environment key: {key}"));
            }
            if value.len() > MAX_RUNTIME_ENV_VALUE_LENGTH {
                return Err(format!(
                    "environment value too long for {key}; max {} chars",
                    MAX_RUNTIME_ENV_VALUE_LENGTH
                ));
            }
        }

        let payload_size = self.estimated_payload_bytes();
        if payload_size > MAX_RUNTIME_ENV_PAYLOAD_BYTES {
            return Err("runtime env payload too large".to_string());
        }

        if self.schedules.len() > MAX_CONFIG_SCHEDULES {
            return Err(format!("too many schedules; max {}", MAX_CONFIG_SCHEDULES));
        }
        for schedule in &self.schedules {
            validate_config_schedule(schedule)?;
        }

        Ok(())
    }

    fn estimated_payload_bytes(&self) -> usize {
        std::mem::size_of::<usize>()
            + self
                .runtime_env
                .iter()
                .map(|(key, value)| key.len() + value.len())
                .sum::<usize>()
    }
}

fn validate_config_schedule(schedule: &CronJob) -> Result<(), String> {
    let id = schedule.id.trim();
    if id.is_empty() {
        return Err("schedule id must not be empty".to_string());
    }
    if id.len() > MAX_CONFIG_SCHEDULE_ID_LENGTH {
        return Err(format!(
            "schedule id too long for {id}; max {} chars",
            MAX_CONFIG_SCHEDULE_ID_LENGTH
        ));
    }
    if schedule.task_prompt.trim().is_empty() {
        return Err(format!("schedule {id} task_prompt must not be empty"));
    }
    if schedule.task_prompt.len() > MAX_CONFIG_SCHEDULE_TEXT_LENGTH {
        return Err(format!(
            "schedule {id} task_prompt too long; max {} chars",
            MAX_CONFIG_SCHEDULE_TEXT_LENGTH
        ));
    }
    if schedule.description.len() > MAX_CONFIG_SCHEDULE_TEXT_LENGTH {
        return Err(format!(
            "schedule {id} description too long; max {} chars",
            MAX_CONFIG_SCHEDULE_TEXT_LENGTH
        ));
    }
    if schedule.cron_expression.is_none() && schedule.interval_seconds.is_none() {
        return Err(format!(
            "schedule {id} must include cron_expression or interval_seconds"
        ));
    }
    Ok(())
}

fn is_valid_env_key(key: &str) -> bool {
    if key.is_empty() || key.len() > MAX_RUNTIME_ENV_KEY_LENGTH {
        return false;
    }
    let mut chars = key.chars();
    if let Some(first) = chars.next() {
        if !(first == '_' || first.is_ascii_alphabetic()) {
            return false;
        }
    }
    chars.all(|ch| ch == '_' || ch.is_ascii_alphanumeric())
}

fn redacted_env_keys(entries: &HashMap<String, String>) -> Vec<&str> {
    entries.keys().map(|key| key.as_str()).collect()
}

#[cfg_attr(feature = "openapi", utoipa::path(
    put,
    path = "/config",
    request_body = ConfigUpdateRequest,
    responses(
        (status = 200, description = "Configuration applied", body = ConfigResponse),
        (status = 400, description = "Invalid configuration"),
        (status = 500, description = "Failed to persist or apply configuration")
    ),
    security(("bearer" = []))
))]
pub async fn put_config(
    State(state): State<ApiState>,
    Json(request): Json<ConfigUpdateRequest>,
) -> Result<Json<ConfigResponse>, (StatusCode, String)> {
    if let Err(error) = request.validate() {
        warn!(
            error = %error,
            keys = ?redacted_env_keys(&request.runtime_env),
            key_count = request.runtime_env.len(),
            payload_size = request.estimated_payload_bytes(),
            "config update rejected"
        );
        let status = if error.contains("payload too large") {
            StatusCode::PAYLOAD_TOO_LARGE
        } else {
            StatusCode::BAD_REQUEST
        };
        return Err((status, error));
    }
    let env_key_count = request.runtime_env.len();
    let secret_rotated = request
        .runtime_secret
        .as_ref()
        .is_some_and(|v| !v.trim().is_empty());
    if env_key_count > 0 {
        state.config_store.merge_runtime_env(request.runtime_env);
    }
    if let Some(secret) = request.runtime_secret {
        let secret = secret.trim().to_string();
        if !secret.is_empty() {
            state.config_store.merge_runtime_env(HashMap::from([(
                "HIVY_RUNTIME_SECRET".to_string(),
                secret.clone(),
            )]));
            let mut token = state.bearer_token.write().await;
            *token = secret;
        }
    }
    let definition = request.definition;
    apply_definition(&state, definition.clone()).await?;
    Ok(Json(ConfigResponse {
        applied_at: Utc::now(),
        definition,
        env_key_count,
        secret_rotated,
    }))
}

async fn apply_definition(
    state: &ApiState,
    definition: AgentDefinition,
) -> Result<(), (StatusCode, String)> {
    if let Err(error) = definition.system_prompt.validate() {
        return Err((StatusCode::BAD_REQUEST, error));
    }
    if definition.system_prompt.cacheable_segments.is_empty() {
        return Err((
            StatusCode::BAD_REQUEST,
            "system_prompt.cacheable_segments must not be empty".to_string(),
        ));
    }

    state
        .config_repo
        .upsert(&definition)
        .await
        .map_err(|error| internal_error_response("config.persist", error))?;
    state.skill_writer.sync(&definition.skills);
    for sub_agent in definition.sub_agents.values() {
        state.skill_writer.sync(&sub_agent.skills);
    }
    if let Some(registry) = state.mcp_registry.as_ref() {
        let runtime_env = state.config_store.runtime_env();
        registry
            .reload_from_specs(&definition.mcp_servers, &runtime_env)
            .await;
    }
    if let Some(reloader) = state.outbound_reloader.as_ref() {
        reloader
            .reload_outbound_channels(&definition.outbound_channels)
            .await
            .map_err(|error| internal_error_response("config.reload_outbound_channels", error))?;
    }
    state.config_store.replace(definition);
    state.mark_config_loaded();
    Ok(())
}

#[cfg_attr(feature = "openapi", utoipa::path(
    post,
    path = "/control/commands",
    request_body = ControlCommandsRequest,
    responses(
        (status = 200, description = "Commands executed", body = ControlCommandsResponse),
        (status = 400, description = "Invalid command payload"),
        (status = 413, description = "Payload too large"),
        (status = 500, description = "Command execution failed")
    ),
    security(("bearer" = []))
))]
pub async fn post_control_commands(
    State(state): State<ApiState>,
    Json(request): Json<ControlCommandsRequest>,
) -> Result<Json<ControlCommandsResponse>, (StatusCode, String)> {
    validate_control_commands(&request)?;
    let workdir = resolve_control_workdir(&state.workspace_root, request.workdir.as_deref())?;
    let timeout = request
        .timeout_seconds
        .unwrap_or(120)
        .clamp(1, MAX_CONTROL_COMMAND_TIMEOUT_SECONDS);
    let runtime_env = state.config_store.runtime_env();
    let mut results = Vec::with_capacity(request.commands.len());
    let mut ok = true;

    for command in request.commands {
        let result = state
            .bash
            .exec(
                &command,
                BashExecOptions {
                    workdir: workdir.clone(),
                    env: runtime_env.as_ref().clone(),
                    timeout: Some(Duration::from_secs(timeout)),
                    max_output_bytes: MAX_CONTROL_COMMAND_OUTPUT_BYTES,
                },
            )
            .await
            .map_err(|error| internal_error_response("control_commands.execute", error))?;
        let output = String::from_utf8_lossy(&result.stdout_combined).to_string();
        let command_ok = result.exit_code == Some(0) && !result.timed_out;
        if !command_ok {
            ok = false;
        }
        results.push(ControlCommandResult {
            command,
            exit_code: result.exit_code,
            timed_out: result.timed_out,
            truncated: result.truncated,
            output,
        });
        if !command_ok && request.stop_on_error {
            break;
        }
    }

    Ok(Json(ControlCommandsResponse { ok, results }))
}

fn validate_control_commands(request: &ControlCommandsRequest) -> Result<(), (StatusCode, String)> {
    if request.commands.is_empty() {
        return Err((
            StatusCode::BAD_REQUEST,
            "commands must not be empty".to_string(),
        ));
    }
    if request.commands.len() > MAX_CONTROL_COMMANDS {
        return Err((
            StatusCode::BAD_REQUEST,
            format!("too many commands; max {MAX_CONTROL_COMMANDS}"),
        ));
    }
    let total_bytes = request
        .commands
        .iter()
        .map(|command| command.len())
        .sum::<usize>()
        + request
            .workdir
            .as_ref()
            .map(|value| value.len())
            .unwrap_or(0);
    if total_bytes > MAX_CONTROL_COMMAND_PAYLOAD_BYTES {
        return Err((
            StatusCode::PAYLOAD_TOO_LARGE,
            "command payload too large".to_string(),
        ));
    }
    for command in &request.commands {
        if command.trim().is_empty() {
            return Err((
                StatusCode::BAD_REQUEST,
                "commands must not contain empty entries".to_string(),
            ));
        }
        if command.len() > MAX_CONTROL_COMMAND_LENGTH {
            return Err((
                StatusCode::BAD_REQUEST,
                format!("command too long; max {MAX_CONTROL_COMMAND_LENGTH} bytes"),
            ));
        }
    }
    Ok(())
}

fn resolve_control_workdir(
    workspace_root: &FsPath,
    requested: Option<&str>,
) -> Result<PathBuf, (StatusCode, String)> {
    let root = workspace_root
        .canonicalize()
        .map_err(|error| internal_error_response("control_commands.workspace_root", error))?;
    let path = requested
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .map(PathBuf::from)
        .unwrap_or_else(|| root.clone());
    let candidate = if path.is_absolute() {
        path
    } else {
        root.join(path)
    };
    let canonical = candidate.canonicalize().map_err(|error| {
        (
            StatusCode::BAD_REQUEST,
            format!("workdir must exist under workspace: {error}"),
        )
    })?;
    if !canonical.starts_with(&root) {
        return Err((
            StatusCode::BAD_REQUEST,
            "workdir must be under workspace".to_string(),
        ));
    }
    Ok(canonical)
}

#[cfg_attr(feature = "openapi", utoipa::path(
    get,
    path = "/config",
    responses(
        (status = 200, description = "Current agent configuration", body = AgentDefinition)
    ),
    security(("bearer" = []))
))]
pub async fn get_config(State(state): State<ApiState>) -> Json<AgentDefinition> {
    let snapshot = state.config_store.snapshot();
    Json((*snapshot).clone())
}

#[derive(Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct ListSessionsParams {
    pub cursor: Option<String>,
    pub status: Option<String>,
    pub limit: Option<u32>,
    pub session_id: Option<String>,
    pub q: Option<String>,
}

#[derive(Serialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct ListSessionsResponse {
    pub items: Vec<domain::Session>,
    pub next_cursor: Option<String>,
}

#[cfg_attr(feature = "openapi", utoipa::path(
    get,
    path = "/sessions",
    params(
        ("cursor" = Option<String>, Query, description = "RFC 3339 cursor from the previous page"),
        ("status" = Option<String>, Query, description = "Session status filter: active, completed, or errored"),
        ("session_id" = Option<String>, Query, description = "Exact session ID filter"),
        ("q" = Option<String>, Query, description = "Prefix search over session identifiers"),
        ("limit" = Option<u32>, Query, description = "Maximum sessions to return, clamped from 1 to 200")
    ),
    responses(
        (status = 200, description = "Sessions page", body = ListSessionsResponse),
        (status = 400, description = "Invalid cursor or status"),
        (status = 500, description = "Failed to list sessions")
    ),
    security(("bearer" = []))
))]
pub async fn list_sessions(
    State(state): State<ApiState>,
    Query(params): Query<ListSessionsParams>,
) -> Result<Json<ListSessionsResponse>, (StatusCode, String)> {
    let cursor = params
        .cursor
        .as_deref()
        .map(parse_session_cursor)
        .transpose()
        .map_err(|error| (StatusCode::BAD_REQUEST, error))?;
    let status = params
        .status
        .as_deref()
        .map(parse_status)
        .transpose()
        .map_err(|error| (StatusCode::BAD_REQUEST, error))?;
    let limit = params.limit.unwrap_or(50).clamp(1, 200);

    let mut sessions = state
        .session_repo
        .list(
            SessionListFilter {
                cursor,
                status,
                session_id: clean_optional(params.session_id),
                search: clean_optional(params.q),
            },
            limit + 1,
        )
        .await
        .map_err(|error| internal_error_response("sessions.list", error))?;

    let has_more = sessions.len() > limit as usize;
    if has_more {
        sessions.truncate(limit as usize);
    }
    let next_cursor = if has_more {
        sessions
            .last()
            .map(|session| encode_session_cursor(session.last_activity_at, session.id.as_str()))
    } else {
        None
    };

    Ok(Json(ListSessionsResponse {
        items: sessions,
        next_cursor,
    }))
}

#[derive(Serialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct SessionDetailResponse {
    pub session: domain::Session,
    pub events: Vec<domain::SessionEvent>,
}

#[cfg_attr(feature = "openapi", utoipa::path(
    get,
    path = "/sessions/{session_id}",
    params(("session_id" = String, Path, description = "Session identifier")),
    responses(
        (status = 200, description = "Session details", body = SessionDetailResponse),
        (status = 404, description = "Session not found"),
        (status = 500, description = "Failed to load session details")
    ),
    security(("bearer" = []))
))]
pub async fn get_session_detail(
    State(state): State<ApiState>,
    Path(session_id): Path<String>,
) -> Result<Json<SessionDetailResponse>, StatusCode> {
    let session_id = SessionId::from(session_id);
    let session = state
        .session_repo
        .get(&session_id)
        .await
        .map_err(|error| {
            capture_internal_error("sessions.get", error);
            StatusCode::INTERNAL_SERVER_ERROR
        })?
        .ok_or(StatusCode::NOT_FOUND)?;
    let events = state
        .event_repo
        .list_recent(&session_id, 100)
        .await
        .map_err(|error| {
            capture_internal_error("sessions.events", error);
            StatusCode::INTERNAL_SERVER_ERROR
        })?;
    Ok(Json(SessionDetailResponse { session, events }))
}

#[cfg_attr(feature = "openapi", utoipa::path(
    get,
    path = "/healthz",
    responses(
        (status = 200, description = "Runtime process is alive", body = HealthResponse)
    ),
    security(())
))]
pub async fn healthz(State(state): State<ApiState>) -> impl IntoResponse {
    Json(HealthResponse {
        status: "ok",
        sentry_enabled: state.sentry_enabled,
        sentry_dsn_set: state.sentry_dsn_set,
    })
}

#[cfg_attr(feature = "openapi", utoipa::path(
    get,
    path = "/readyz",
    responses(
        (status = 200, description = "Runtime is ready"),
        (status = 503, description = "Runtime is not ready")
    ),
    security(("bearer" = []))
))]
pub async fn readyz(State(state): State<ApiState>) -> impl IntoResponse {
    if state.is_ready() {
        StatusCode::OK
    } else {
        StatusCode::SERVICE_UNAVAILABLE
    }
}

#[cfg_attr(feature = "openapi", utoipa::path(
    get,
    path = "/infra/readyz",
    responses(
        (status = 200, description = "Runtime is ready for infrastructure routing"),
        (status = 503, description = "Runtime is not ready")
    ),
    security(("bearer" = []))
))]
pub async fn infra_readyz(State(state): State<ApiState>) -> impl IntoResponse {
    if state.is_ready() {
        StatusCode::OK
    } else {
        StatusCode::SERVICE_UNAVAILABLE
    }
}

#[cfg_attr(feature = "openapi", utoipa::path(
    post,
    path = "/sessions/{session_id}/messages",
    params(("session_id" = String, Path, description = "Session identifier")),
    request_body = SessionMessageRequest,
    responses(
        (status = 200, description = "Message accepted and stream created", body = SessionMessageResponse),
        (status = 500, description = "Failed to inject message"),
        (status = 503, description = "session API is not enabled")
    ),
    security(("bearer" = []))
))]
pub async fn post_session_message(
    State(state): State<ApiState>,
    Path(session_id): Path<String>,
    Json(request): Json<SessionMessageRequest>,
) -> Result<Json<SessionMessageResponse>, (StatusCode, String)> {
    let Some(session_stream) = state.session_stream.as_ref() else {
        return Err((
            StatusCode::SERVICE_UNAVAILABLE,
            "session API is not enabled".to_string(),
        ));
    };
    session_stream
        .inject_message(SessionId::from(session_id), request)
        .await
        .map(Json)
        .map_err(|error| internal_error_response("session_stream.inject_message", error))
}

#[cfg_attr(feature = "openapi", utoipa::path(
    post,
    path = "/sessions/{session_id}/interrupt",
    params(("session_id" = String, Path, description = "Session identifier")),
    responses(
        (status = 200, description = "Session interrupt accepted", body = SessionInterruptResponse),
        (status = 503, description = "session interrupt is not enabled")
    ),
    security(("bearer" = []))
))]
pub async fn post_session_interrupt(
    State(state): State<ApiState>,
    Path(session_id): Path<String>,
) -> Result<Json<SessionInterruptResponse>, (StatusCode, String)> {
    let Some(session_stream) = state.session_stream.as_ref() else {
        return Err((
            StatusCode::SERVICE_UNAVAILABLE,
            "session API is not enabled".to_string(),
        ));
    };
    let Some(interrupter) = session_stream.interrupter.as_ref() else {
        return Err((
            StatusCode::SERVICE_UNAVAILABLE,
            "session interrupt is not enabled".to_string(),
        ));
    };
    let interrupted = interrupter
        .interrupt_session(&SessionId::from(session_id))
        .await;
    Ok(Json(SessionInterruptResponse {
        interrupted,
        status: if interrupted { "interrupted" } else { "idle" },
    }))
}

#[cfg_attr(feature = "openapi", utoipa::path(
    post,
    path = "/sessions/{session_id}/questions/{question_request_id}/answer",
    params(
        ("session_id" = String, Path, description = "Session identifier"),
        ("question_request_id" = String, Path, description = "Question request identifier")
    ),
    request_body = QuestionAnswerPayload,
    responses(
        (status = 200, description = "Question answered", body = QuestionAnswerResponse),
        (status = 400, description = "Invalid question answer"),
        (status = 404, description = "Question request not found"),
        (status = 409, description = "Question cannot be answered"),
        (status = 503, description = "question API is not enabled")
    ),
    security(("bearer" = []))
))]
pub async fn post_question_answer(
    State(state): State<ApiState>,
    Path((session_id, question_request_id)): Path<(String, String)>,
    Json(answer): Json<QuestionAnswerPayload>,
) -> Result<Json<QuestionAnswerResponse>, (StatusCode, String)> {
    let Some(manager) = state.question_manager.as_ref() else {
        return Err((
            StatusCode::SERVICE_UNAVAILABLE,
            "question API is not enabled".to_string(),
        ));
    };
    manager
        .answer(&SessionId::from(session_id), &question_request_id, answer)
        .await
        .map(Json)
        .map_err(question_answer_error_response)
}

fn question_answer_error_response(error: QuestionAnswerError) -> (StatusCode, String) {
    match error {
        QuestionAnswerError::NotFound => (StatusCode::NOT_FOUND, error.to_string()),
        QuestionAnswerError::AlreadyAnswered | QuestionAnswerError::NotWaiting => {
            (StatusCode::CONFLICT, error.to_string())
        }
        QuestionAnswerError::InvalidAnswer(_) => (StatusCode::BAD_REQUEST, error.to_string()),
        QuestionAnswerError::Storage(error) => {
            internal_error_response("question_manager.answer", error)
        }
    }
}

#[cfg_attr(feature = "openapi", utoipa::path(
    get,
    path = "/sessions/{session_id}/stream",
    params(
        ("session_id" = String, Path, description = "Session identifier"),
        ("replay" = Option<String>, Query, description = "Replay mode: `all` or `none`"),
        ("after_seq" = Option<u64>, Query, description = "Replay retained events with sequence greater than this value")
    ),
    responses(
        (status = 200, description = "Server-sent event stream", content_type = "text/event-stream"),
        (status = 400, description = "Invalid replay parameters"),
        (status = 503, description = "session API is not enabled")
    ),
    security(("bearer" = []))
))]
pub async fn get_session_live_stream(
    State(state): State<ApiState>,
    Path(session_id): Path<String>,
    Query(query): Query<SessionStreamQuery>,
) -> Result<Response, (StatusCode, String)> {
    let Some(session_stream) = state.session_stream.as_ref() else {
        return Err((
            StatusCode::SERVICE_UNAVAILABLE,
            "session API is not enabled".to_string(),
        ));
    };
    let replay_mode = query.replay_mode().map_err(|error| {
        warn!(error = %error, "stream query rejected");
        (StatusCode::BAD_REQUEST, error)
    })?;
    if let Some(repo_service) = state.repo_service.as_ref() {
        repo_service.register_session(&session_id).await;
    }
    Ok(sse_stream_response(
        crate::session_stream::session_stream_response(
            session_stream.broker.clone(),
            session_id,
            replay_mode,
        )
        .await,
    ))
}

#[cfg_attr(feature = "openapi", utoipa::path(
    get,
    path = "/sessions/{session_id}/streams/{stream_id}",
    params(
        ("session_id" = String, Path, description = "Session identifier"),
        ("stream_id" = String, Path, description = "Turn stream identifier"),
        ("replay" = Option<String>, Query, description = "Replay mode: `all` or `none`"),
        ("after_seq" = Option<u64>, Query, description = "Replay retained events with sequence greater than this value")
    ),
    responses(
        (status = 200, description = "Server-sent event stream", content_type = "text/event-stream"),
        (status = 400, description = "Invalid replay parameters"),
        (status = 404, description = "stream not found"),
        (status = 503, description = "session API is not enabled")
    ),
    security(("bearer" = []))
))]
pub async fn get_session_stream(
    State(state): State<ApiState>,
    Path((session_id, stream_id)): Path<(String, String)>,
    Query(query): Query<SessionStreamQuery>,
) -> Result<Response, (StatusCode, String)> {
    let Some(session_stream) = state.session_stream.as_ref() else {
        return Err((
            StatusCode::SERVICE_UNAVAILABLE,
            "session API is not enabled".to_string(),
        ));
    };
    let replay_mode = query.replay_mode().map_err(|error| {
        warn!(error = %error, "stream query rejected");
        (StatusCode::BAD_REQUEST, error)
    })?;
    stream_response(
        session_stream.broker.clone(),
        session_id,
        stream_id,
        replay_mode,
    )
    .await
    .map(sse_stream_response)
    .ok_or((StatusCode::NOT_FOUND, "stream not found".to_string()))
}

fn sse_stream_response(response: impl IntoResponse) -> Response {
    let mut response = response.into_response();
    let headers = response.headers_mut();
    headers.insert(
        CACHE_CONTROL,
        HeaderValue::from_static("no-cache, no-transform"),
    );
    headers.insert(CONNECTION, HeaderValue::from_static("keep-alive"));
    headers.insert(
        HeaderName::from_static("x-accel-buffering"),
        HeaderValue::from_static("no"),
    );
    headers.insert(
        HeaderName::from_static("access-control-allow-origin"),
        HeaderValue::from_static("*"),
    );
    response
}

fn parse_cursor(raw: &str) -> Result<DateTime<Utc>, String> {
    DateTime::parse_from_rfc3339(raw)
        .map(|dt| dt.with_timezone(&Utc))
        .map_err(|error| format!("invalid cursor `{raw}`: {error}"))
}

fn parse_session_cursor(raw: &str) -> Result<SessionListCursor, String> {
    let (timestamp, id) = match raw.split_once('|') {
        Some((timestamp, id)) => (timestamp, Some(id.trim().to_string())),
        None => (raw, None),
    };
    Ok(SessionListCursor {
        last_activity_at: parse_cursor(timestamp)?,
        id: id.filter(|value| !value.is_empty()),
    })
}

fn encode_session_cursor(last_activity_at: DateTime<Utc>, id: &str) -> String {
    format!("{}|{}", last_activity_at.to_rfc3339(), id)
}

fn clean_optional(value: Option<String>) -> Option<String> {
    value
        .map(|value| value.trim().to_string())
        .filter(|value| !value.is_empty())
}

fn parse_status(raw: &str) -> Result<SessionStatus, String> {
    match raw {
        "active" => Ok(SessionStatus::Active),
        "completed" => Ok(SessionStatus::Completed),
        "errored" => Ok(SessionStatus::Errored),
        other => Err(format!("invalid status `{other}`")),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    use domain::cron::CronJobState;
    use std::collections::HashMap;

    #[test]
    fn stream_query_default_replays_all() {
        assert_eq!(
            SessionStreamQuery::default().replay_mode().unwrap(),
            StreamReplayMode::All
        );
    }

    #[test]
    fn stream_query_accepts_replay_none_and_after_seq() {
        let none = SessionStreamQuery {
            replay: Some("none".to_string()),
            after_seq: None,
        };
        assert_eq!(none.replay_mode().unwrap(), StreamReplayMode::None);

        let after_seq = SessionStreamQuery {
            replay: None,
            after_seq: Some("42".to_string()),
        };
        assert_eq!(
            after_seq.replay_mode().unwrap(),
            StreamReplayMode::AfterSeq(42)
        );
    }

    #[test]
    fn stream_query_rejects_invalid_cursor_options() {
        let invalid_combo = SessionStreamQuery {
            replay: Some("none".to_string()),
            after_seq: Some("42".to_string()),
        };
        assert!(invalid_combo.replay_mode().is_err());

        let invalid_replay = SessionStreamQuery {
            replay: Some("latest".to_string()),
            after_seq: None,
        };
        assert!(invalid_replay.replay_mode().is_err());

        let invalid_sequence = SessionStreamQuery {
            replay: None,
            after_seq: Some("nope".to_string()),
        };
        assert!(invalid_sequence.replay_mode().is_err());
    }

    fn test_definition() -> AgentDefinition {
        AgentDefinition {
            agent: domain::AgentMeta {
                name: "test".to_string(),
                description: String::new(),
            },
            system_prompt: domain::SystemPromptConfig {
                cacheable_segments: vec![domain::SystemPromptSegment::StaticText(
                    domain::StaticPromptSegment {
                        title: String::new(),
                        content: "test".to_string(),
                    },
                )],
                dynamic_segments: Vec::new(),
            },
            model: domain::ModelConfig::OpenaiCompatible {
                base_url: "http://localhost".to_string(),
                model_id: "test".to_string(),
                api_key_env: "HIVY_PROXY_API_KEY".to_string(),
                temperature: None,
                max_output_tokens: None,
                reasoning_effort: None,
                extra_headers: HashMap::new(),
                fallback: None,
            },
            multimodal_model: None,
            limits: Default::default(),
            context: Default::default(),
            tools: Some(Vec::new()),
            mcp_servers: Vec::new(),
            skills: Vec::new(),
            outbound_channels: Vec::new(),
            sub_agents: Default::default(),
            safety: Default::default(),
        }
    }

    #[test]
    fn internal_error_response_does_not_expose_details() {
        let (status, body) =
            internal_error_response("test.context", "database is readonly and token missing");

        assert_eq!(status, StatusCode::INTERNAL_SERVER_ERROR);
        assert_eq!(body, INTERNAL_RETRY_MESSAGE);
        assert!(!body.contains("readonly"));
        assert!(!body.contains("token"));
    }

    #[test]
    fn runtime_env_update_accepts_valid_payload() {
        let update = ConfigUpdateRequest {
            definition: test_definition(),
            runtime_secret: None,
            runtime_env: HashMap::from([
                ("GOOD_KEY".to_string(), "value".to_string()),
                ("ANOTHER_KEY_1".to_string(), "another".to_string()),
            ]),
            schedules: Vec::new(),
        };

        assert!(
            update.validate().is_ok(),
            "expected valid env payload to pass"
        );
    }

    #[test]
    fn runtime_env_update_rejects_invalid_key() {
        let update = ConfigUpdateRequest {
            definition: test_definition(),
            runtime_secret: None,
            runtime_env: HashMap::from([("1BAD_KEY".to_string(), "value".to_string())]),
            schedules: Vec::new(),
        };
        assert_eq!(
            update.validate().unwrap_err(),
            "invalid environment key: 1BAD_KEY"
        );
    }

    #[test]
    fn runtime_env_update_accepts_provider_key_names() {
        let update = ConfigUpdateRequest {
            definition: test_definition(),
            runtime_secret: None,
            runtime_env: HashMap::from([("OPENAI_API_KEY".to_string(), "value".to_string())]),
            schedules: Vec::new(),
        };
        assert!(update.validate().is_ok());
    }

    #[test]
    fn runtime_env_update_rejects_too_many_keys() {
        let mut entries = HashMap::new();
        for i in 0..=MAX_RUNTIME_ENV_KEYS {
            entries.insert(format!("KEY_{i}"), "value".to_string());
        }

        let update = ConfigUpdateRequest {
            definition: test_definition(),
            runtime_secret: None,
            runtime_env: entries,
            schedules: Vec::new(),
        };
        assert_eq!(
            update.validate().unwrap_err(),
            format!(
                "too many environment variables; max {}",
                MAX_RUNTIME_ENV_KEYS
            )
        );
    }

    #[test]
    fn runtime_env_update_rejects_long_environment_value() {
        let update = ConfigUpdateRequest {
            definition: test_definition(),
            runtime_secret: None,
            runtime_env: HashMap::from([("VALUE_TOO_LONG".to_string(), "x".repeat(8193))]),
            schedules: Vec::new(),
        };

        assert_eq!(
            update.validate().unwrap_err(),
            format!(
                "environment value too long for VALUE_TOO_LONG; max {} chars",
                MAX_RUNTIME_ENV_VALUE_LENGTH
            )
        );
    }

    #[test]
    fn runtime_env_update_rejects_oversize_payload() {
        let mut entries = HashMap::new();
        for i in 0..9 {
            entries.insert(format!("OVERSIZE_PAYLOAD_{i}"), "x".repeat(8192));
        }
        let update = ConfigUpdateRequest {
            definition: test_definition(),
            runtime_secret: None,
            runtime_env: entries,
            schedules: Vec::new(),
        };

        assert_eq!(
            update.validate().unwrap_err(),
            "runtime env payload too large"
        );
    }

    #[test]
    fn runtime_env_update_accepts_empty_payload() {
        let update = ConfigUpdateRequest {
            definition: test_definition(),
            runtime_secret: None,
            runtime_env: HashMap::new(),
            schedules: Vec::new(),
        };
        assert!(
            update.validate().is_ok(),
            "empty payload should be accepted as clear overlay"
        );
    }

    #[test]
    fn runtime_config_accepts_valid_schedule_payload() {
        let update = ConfigUpdateRequest {
            definition: test_definition(),
            runtime_secret: None,
            runtime_env: HashMap::new(),
            schedules: vec![CronJob {
                id: "system:service-discovery:railway:conn".to_string(),
                description: "Railway service discovery".to_string(),
                channel: "system".to_string(),
                task_prompt: "Discover Railway services and retain useful memories.".to_string(),
                cron_expression: None,
                interval_seconds: Some(86_400),
                repeat_count: None,
                repeat_completed: 0,
                state: CronJobState::Active,
                next_run_at: Utc::now(),
                last_run_at: None,
                last_status: None,
                last_error: None,
                session_continuation_id: None,
                stream_id: None,
                created_at: Utc::now(),
                created_by_session: "system:service-discovery".to_string(),
            }],
        };

        assert!(update.validate().is_ok());
    }
}
