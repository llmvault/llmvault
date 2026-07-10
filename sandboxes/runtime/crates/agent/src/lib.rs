use std::sync::Arc;

use async_trait::async_trait;
use domain::{
    AgentDefinition, ModelConfig, QuestionAnswerPayload, RequestUserInputPayload, SessionId,
    UpdatePlanPayload, UpdatePlanResult,
};
use futures::stream::BoxStream;
use serde::{Deserialize, Serialize};

pub mod compaction;
pub mod history;
pub mod model_client;
pub mod model_profile;
pub mod primitives;
pub mod request_builder;
pub mod rig_tool_registry;
pub mod runner;
pub mod tool_executor;
pub use runner::RigAgentRunner;

#[derive(Debug, Clone)]
pub struct TurnInput {
    pub text: String,
    pub prior_history: Vec<HistoryEntry>,
    pub session_context: Vec<String>,
    pub model_override: Option<ModelConfig>,
    pub session_stream_id: Option<String>,
    pub trace_id: Option<String>,
    pub turn_id: Option<String>,
    /// Hivy user id of the human behind this turn, injected into agent tool
    /// calls as `_hivy_actor_user_id`. None for automated runs.
    pub actor_user_id: Option<String>,
}

#[derive(Debug, Clone)]
pub struct HistoryEntry {
    pub role: HistoryRole,
    pub speaker_id: String,
    pub speaker_display_name: Option<String>,
    pub text: String,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum HistoryRole {
    User,
    Assistant,
}

impl TurnInput {
    pub fn text(input: impl Into<String>) -> Self {
        Self {
            text: input.into(),
            prior_history: Vec::new(),
            session_context: Vec::new(),
            model_override: None,
            session_stream_id: None,
            trace_id: None,
            turn_id: None,
            actor_user_id: None,
        }
    }

    pub fn with_actor_user(mut self, actor_user_id: Option<String>) -> Self {
        self.actor_user_id = actor_user_id.filter(|id| !id.trim().is_empty());
        self
    }

    pub fn with_history(mut self, history: Vec<HistoryEntry>) -> Self {
        self.prior_history = history;
        self
    }

    pub fn with_session_context(mut self, context: impl Into<String>) -> Self {
        let context = context.into();
        if !context.trim().is_empty() {
            self.session_context.push(context);
        }
        self
    }

    pub fn with_session_stream_id(mut self, stream_id: impl Into<String>) -> Self {
        let stream_id = stream_id.into();
        if !stream_id.trim().is_empty() {
            self.session_stream_id = Some(stream_id);
        }
        self
    }

    pub fn with_model_override(mut self, model: ModelConfig) -> Self {
        self.model_override = Some(model);
        self
    }

    pub fn with_turn_context(
        mut self,
        trace_id: impl Into<String>,
        turn_id: impl Into<String>,
    ) -> Self {
        let trace_id = trace_id.into();
        let turn_id = turn_id.into();
        if !trace_id.trim().is_empty() {
            self.trace_id = Some(trace_id);
        }
        if !turn_id.trim().is_empty() {
            self.turn_id = Some(turn_id);
        }
        self
    }
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(tag = "kind", rename_all = "snake_case")]
pub enum AgentEvent {
    TokenChunk {
        text: String,
    },
    ThinkingChunk {
        text: String,
    },
    ToolCall {
        id: String,
        tool: String,
        args: serde_json::Value,
    },
    ToolResult {
        id: String,
        result: serde_json::Value,
    },
    RunEvent {
        event: String,
        payload: serde_json::Value,
    },
    FinalMessage {
        text: String,
    },
    Error {
        message: String,
    },
}

#[derive(Debug, thiserror::Error)]
pub enum AgentError {
    #[error("model error: {0}")]
    Model(String),
    #[error("limit exceeded: {0}")]
    LimitExceeded(String),
    #[error(transparent)]
    Other(#[from] anyhow::Error),
}

pub type Result<T> = std::result::Result<T, AgentError>;

#[async_trait]
pub trait AgentRunner: Send + Sync + 'static {
    async fn run_turn(
        &self,
        session_id: &SessionId,
        user_input: TurnInput,
        definition_override: Option<Arc<AgentDefinition>>,
    ) -> Result<BoxStream<'static, AgentEvent>>;
}

#[async_trait]
pub trait QuestionRequester: Send + Sync + 'static {
    async fn request_user_input(
        &self,
        session_id: &SessionId,
        request: RequestUserInputPayload,
    ) -> anyhow::Result<(String, QuestionAnswerPayload)>;
}

#[async_trait]
pub trait PlanUpdater: Send + Sync + 'static {
    async fn update_plan(
        &self,
        session_id: &SessionId,
        payload: UpdatePlanPayload,
    ) -> anyhow::Result<UpdatePlanResult>;
}
