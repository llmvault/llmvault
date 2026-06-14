use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};
use std::collections::HashMap;

#[derive(Debug, Clone, Serialize, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct OutboundEvent {
    pub event_type: String,
    pub payload: serde_json::Value,
    pub at: DateTime<Utc>,
}

impl OutboundEvent {
    pub fn new(event_type: impl Into<String>, payload: serde_json::Value) -> Self {
        Self {
            event_type: event_type.into(),
            payload,
            at: Utc::now(),
        }
    }
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct OutboundChannelSpec {
    pub name: String,
    #[serde(flatten)]
    pub kind: OutboundChannelKind,
    #[serde(default)]
    pub event_filter: Option<Vec<String>>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
#[serde(tag = "type", rename_all = "snake_case")]
pub enum OutboundChannelKind {
    Webhook {
        url: String,
        secret_env: String,
        #[serde(default)]
        extra_headers: HashMap<String, String>,
    },
}

pub mod event_types {
    pub const TURN_STARTED: &str = "turn_started";
    pub const TOKEN: &str = "token";
    pub const THINKING: &str = "thinking";
    pub const TOOL_RESULT: &str = "tool_result";
    pub const FINAL: &str = "final";
    pub const TURN_COMPLETED: &str = "turn_completed";
    pub const TURN_FAILED: &str = "turn_failed";
    pub const QUESTION_REQUESTED: &str = "question_requested";
    pub const QUESTION_ANSWERED: &str = "question_answered";
    pub const PLAN_UPDATED: &str = "plan_updated";
    pub const SUBAGENT_STARTED: &str = "subagent_started";
    pub const SUBAGENT_COMPLETED: &str = "subagent_completed";
    pub const SUBAGENT_ERRORED: &str = "subagent_errored";
    pub const MODEL_USAGE: &str = "model_usage";
    pub const ERROR: &str = "error";
    pub const SESSION_WAITING: &str = "session_waiting";

    pub const USER_MESSAGE_RECEIVED: &str = "user.message.received";
    pub const SESSION_CREATED: &str = "session.created";
    pub const SESSION_COMPLETED: &str = "session.completed";
    pub const CONFIG_APPLIED: &str = "config.applied";
    pub const SKILL_SYNCED: &str = "skill.synced";
    pub const SCHEDULE_CREATED: &str = "schedule.created";
    pub const SCHEDULE_UPDATED: &str = "schedule.updated";
    pub const SCHEDULE_PAUSED: &str = "schedule.paused";
    pub const SCHEDULE_RESUMED: &str = "schedule.resumed";
    pub const SCHEDULE_CANCELLED: &str = "schedule.cancelled";
    pub const SCHEDULE_RUN_STARTED: &str = "schedule.run_started";
    pub const SCHEDULE_RUN_COMPLETED: &str = "schedule.run_completed";
    pub const SCHEDULE_RUN_FAILED: &str = "schedule.run_failed";
}

pub const DATABASE_CHANNEL_NAME: &str = "database";
