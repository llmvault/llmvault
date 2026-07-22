use std::collections::HashMap;

use serde::{Deserialize, Serialize};

use crate::{
    mcp_specs::{McpSpec, ToolFilter},
    model_config::ModelConfig,
    model_config::SafetyConfig,
    outbound::OutboundChannelSpec,
    tool_specs::ToolSpec,
};

#[derive(Debug, Clone, Serialize, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct AgentDefinition {
    pub agent: AgentMeta,
    #[serde(default)]
    pub system_prompt: SystemPromptConfig,
    pub model: ModelConfig,
    #[serde(default)]
    pub limits: Limits,
    #[serde(default)]
    pub context: ContextConfig,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub tools: Option<Vec<ToolSpec>>,
    #[serde(default)]
    pub mcp_servers: Vec<McpSpec>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub mcp_tool_filter: Option<ToolFilter>,
    #[serde(default)]
    pub outbound_channels: Vec<OutboundChannelSpec>,
    #[serde(default)]
    #[cfg_attr(feature = "openapi", schema(no_recursion))]
    pub sub_agents: HashMap<String, AgentDefinition>,
    #[serde(default)]
    pub safety: SafetyConfig,
    /// Skills the runtime loads into the session automatically on the first
    /// turn (before the first model call), so the agent starts with the skill
    /// content already in context and its `.skills/` files materialized. Absent
    /// in old configs (serde default = empty), which deserialize unchanged.
    #[serde(default)]
    pub auto_load_skills: Vec<AutoLoadSkill>,
}

/// A skill to auto-load, optionally with specific linked files to also load.
#[derive(Debug, Clone, Default, Serialize, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct AutoLoadSkill {
    /// Skill slug, passed to `skill_view` as `{"name": ...}`.
    pub name: String,
    /// Relative linked-file paths, each loaded via `skill_view {name, file_path}`.
    #[serde(default)]
    pub files: Vec<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct AgentMeta {
    pub name: String,
    #[serde(default)]
    pub description: String,
}

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct SystemPromptConfig {
    #[serde(default)]
    pub cacheable_segments: Vec<SystemPromptSegment>,
    #[serde(default)]
    pub dynamic_segments: Vec<SystemPromptSegment>,
}

impl SystemPromptConfig {
    pub fn validate(&self) -> Result<(), String> {
        const MAX_SEGMENTS: usize = 64;
        const MAX_TEXT_CHARS: usize = 64 * 1024;
        if self.cacheable_segments.len() > MAX_SEGMENTS {
            return Err("too many cacheable prompt segments".to_string());
        }
        if self.dynamic_segments.len() > MAX_SEGMENTS {
            return Err("too many dynamic prompt segments".to_string());
        }
        for segment in self
            .cacheable_segments
            .iter()
            .chain(self.dynamic_segments.iter())
        {
            segment.validate(MAX_TEXT_CHARS)?;
        }
        for segment in &self.cacheable_segments {
            if !matches!(segment, SystemPromptSegment::StaticText(_)) {
                return Err("cacheable prompt segments must be static_text".to_string());
            }
        }
        Ok(())
    }
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
#[serde(tag = "type", content = "config", rename_all = "snake_case")]
pub enum SystemPromptSegment {
    StaticText(StaticPromptSegment),
    DynamicContext(DynamicContextPromptSegment),
    McpTools(ListPromptSegment),
}

impl SystemPromptSegment {
    fn validate(&self, max_text_chars: usize) -> Result<(), String> {
        match self {
            SystemPromptSegment::StaticText(segment) => {
                validate_prompt_text("static_text.title", &segment.title, max_text_chars)?;
                validate_prompt_text("static_text.content", &segment.content, max_text_chars)?;
            }
            SystemPromptSegment::DynamicContext(segment) => {
                validate_prompt_text("dynamic_context.title", &segment.title, max_text_chars)?;
                validate_prompt_text(
                    "dynamic_context.preamble",
                    &segment.preamble,
                    max_text_chars,
                )?;
                validate_prompt_text(
                    "dynamic_context.item_template",
                    &segment.item_template,
                    max_text_chars,
                )?;
            }
            SystemPromptSegment::McpTools(segment) => {
                validate_prompt_text("list.title", &segment.title, max_text_chars)?;
                validate_prompt_text("list.preamble", &segment.preamble, max_text_chars)?;
                validate_prompt_text("list.item_template", &segment.item_template, max_text_chars)?;
            }
        }
        Ok(())
    }
}

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct StaticPromptSegment {
    #[serde(default)]
    pub title: String,
    #[serde(default)]
    pub content: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct DynamicContextPromptSegment {
    #[serde(default)]
    pub title: String,
    #[serde(default)]
    pub preamble: String,
    #[serde(default = "default_context_item_template")]
    pub item_template: String,
}

impl Default for DynamicContextPromptSegment {
    fn default() -> Self {
        Self {
            title: String::new(),
            preamble: String::new(),
            item_template: default_context_item_template(),
        }
    }
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct ListPromptSegment {
    #[serde(default)]
    pub title: String,
    #[serde(default)]
    pub preamble: String,
    #[serde(default = "default_list_item_template")]
    pub item_template: String,
}

impl Default for ListPromptSegment {
    fn default() -> Self {
        Self {
            title: String::new(),
            preamble: String::new(),
            item_template: default_list_item_template(),
        }
    }
}

fn validate_prompt_text(label: &str, value: &str, max_text_chars: usize) -> Result<(), String> {
    if value.len() > max_text_chars {
        return Err(format!("{label} is too large"));
    }
    Ok(())
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct Limits {
    pub max_turns_per_session: u32,
    pub input_token_budget: u32,
    pub output_token_budget: u32,
    pub tool_call_timeout_seconds: u32,
}

impl Default for Limits {
    fn default() -> Self {
        Self {
            max_turns_per_session: 20_000,
            input_token_budget: 720_000,
            output_token_budget: 32_000,
            tool_call_timeout_seconds: 240,
        }
    }
}

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct ContextConfig {
    #[serde(default)]
    pub max_history_events: Option<u32>,
    #[serde(default)]
    pub compaction: Option<CompactionConfig>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct CompactionConfig {
    pub enabled: bool,
    #[serde(default)]
    pub token_threshold: Option<u32>,
    #[serde(default)]
    pub token_threshold_percentage: Option<f64>,
    #[serde(default)]
    pub turn_threshold: Option<u32>,
    #[serde(default)]
    pub message_threshold: Option<u32>,
    #[serde(default = "default_eviction")]
    pub eviction_window: f64,
    #[serde(default)]
    pub retention_window: u32,
    #[serde(default = "default_overlap")]
    pub overlap_event_count: u32,
    #[serde(default = "default_chars_per_token")]
    pub chars_per_token: u32,
    #[serde(default)]
    pub on_turn_end: Option<bool>,
}

fn default_eviction() -> f64 {
    0.2
}

fn default_overlap() -> u32 {
    10
}
fn default_chars_per_token() -> u32 {
    4
}
fn default_context_item_template() -> String {
    "{content}".to_string()
}
fn default_list_item_template() -> String {
    "- {name}: {description}".to_string()
}

#[cfg(test)]
mod auto_load_skills_tests {
    use super::AgentDefinition;

    const MINIMAL_MODEL: &str = r#""model":{"provider":"openai_compatible","base_url":"http://x","model_id":"m","api_key_env":"K"}"#;

    fn parse(extra: &str) -> AgentDefinition {
        let json = format!(r#"{{"agent":{{"name":"a"}},{MINIMAL_MODEL}{extra}}}"#);
        serde_json::from_str(&json).expect("definition should deserialize")
    }

    #[test]
    fn absent_auto_load_skills_defaults_to_empty() {
        let definition = parse("");
        assert!(definition.auto_load_skills.is_empty());
    }

    #[test]
    fn deserializes_auto_load_skills_with_and_without_files() {
        let definition = parse(
            r#","auto_load_skills":[{"name":"qa-registry"},{"name":"browser","files":["references/commands.md"]}]"#,
        );
        assert_eq!(definition.auto_load_skills.len(), 2);
        assert_eq!(definition.auto_load_skills[0].name, "qa-registry");
        assert!(
            definition.auto_load_skills[0].files.is_empty(),
            "omitted files must default to empty"
        );
        assert_eq!(definition.auto_load_skills[1].name, "browser");
        assert_eq!(
            definition.auto_load_skills[1].files,
            vec!["references/commands.md".to_string()]
        );
    }

    #[test]
    fn auto_load_skills_round_trips_through_serde() {
        let definition =
            parse(r#","auto_load_skills":[{"name":"browser","files":["references/commands.md"]}]"#);
        let serialized = serde_json::to_string(&definition).expect("serialize");
        let reparsed: AgentDefinition = serde_json::from_str(&serialized).expect("reparse");
        assert_eq!(reparsed.auto_load_skills[0].name, "browser");
        assert_eq!(
            reparsed.auto_load_skills[0].files,
            vec!["references/commands.md"]
        );
    }
}

#[cfg(test)]
mod limits_tests {
    use super::Limits;

    #[test]
    fn defaults_use_quadrupled_agent_run_budgets() {
        let limits = Limits::default();
        assert_eq!(limits.max_turns_per_session, 20_000);
        assert_eq!(limits.input_token_budget, 720_000);
        assert_eq!(limits.output_token_budget, 32_000);
        assert_eq!(limits.tool_call_timeout_seconds, 240);
    }
}
