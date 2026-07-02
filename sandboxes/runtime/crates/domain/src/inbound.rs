use std::sync::Arc;

use crate::{session::SessionId, AgentDefinition, ConfigStore, ModelConfig};
use serde::{Deserialize, Serialize};

#[derive(Debug, Clone)]
pub struct InboundEvent {
    pub envelope_id: String,
    pub session_id: SessionId,
    pub user: String,
    pub user_display_name: Option<String>,
    /// Hivy user id of the human on whose behalf this turn runs, when known.
    /// Injected into agent tool calls as `_hivy_actor_user_id` so tools can
    /// authorize on the requesting user. None for automated (trigger/cron/
    /// system) runs that have no human actor.
    pub actor_user_id: Option<String>,
    pub text: String,
    pub attachments: Vec<Attachment>,
    pub session_context: Vec<String>,
    pub model_definition: Option<ModelConfig>,
    pub raw: serde_json::Value,
    pub is_direct_message: bool,
    pub is_directly_addressed: bool,
    pub link_previews: Vec<LinkPreview>,
    /// When set, the handler uses this definition instead of `config.snapshot()`.
    /// Set by the scheduler for sub-agent delegation.
    pub agent_definition: Option<Arc<AgentDefinition>>,
}

impl InboundEvent {
    pub fn effective_definition(&self, config: &ConfigStore) -> Arc<AgentDefinition> {
        self.agent_definition
            .clone()
            .unwrap_or_else(|| config.snapshot())
    }
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct Attachment {
    pub url: String,
    pub mime_type: String,
    pub name: String,
    #[serde(default)]
    pub size_bytes: Option<u64>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct LinkPreview {
    pub url: String,
    pub title: Option<String>,
    pub description: Option<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct HistoryMessage {
    pub user: String,
    pub user_display_name: Option<String>,
    pub text: String,
    pub ts: String,
    pub is_bot: bool,
}
