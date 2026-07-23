use std::collections::HashMap;

use domain::{ModelCapabilities, ModelConfig};
use serde_json::{json, Value};
use tools::ToolDefinition;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ModelProfileId {
    DeepSeek,
    Glm,
    Kimi,
    MiniMax,
    MiMo,
    Qwen,
    OpenAICompatible,
}

impl ModelProfileId {
    pub fn as_str(self) -> &'static str {
        match self {
            Self::DeepSeek => "deepseek",
            Self::Glm => "glm",
            Self::Kimi => "kimi",
            Self::MiniMax => "minimax",
            Self::MiMo => "mimo",
            Self::Qwen => "qwen",
            Self::OpenAICompatible => "openai_compatible",
        }
    }
}

#[derive(Debug, Clone)]
pub struct ModelProfile {
    pub id: ModelProfileId,
    pub supports_generic_reasoning_effort: bool,
    pub supports_parallel_tool_calls: bool,
    pub strict_tool_schema: bool,
    pub default_temperature: Option<f32>,
    pub default_max_output_tokens: Option<u32>,
    pub max_tool_calls_per_turn: u32,
    pub max_consecutive_tool_errors: u32,
}

impl ModelProfile {
    pub fn from_config(model: &ModelConfig) -> Self {
        let ModelConfig::OpenaiCompatible {
            model_id,
            canonical_model_id,
            provider_id,
            upstream_model_id,
            model_profile,
            capabilities,
            ..
        } = model;
        Self::detect(
            model_profile.as_deref(),
            provider_id.as_deref(),
            canonical_model_id.as_deref(),
            upstream_model_id.as_deref(),
            model_id,
            capabilities.as_ref(),
        )
    }

    pub fn detect(
        explicit: Option<&str>,
        provider_id: Option<&str>,
        canonical_model_id: Option<&str>,
        upstream_model_id: Option<&str>,
        model_id: &str,
        capabilities: Option<&ModelCapabilities>,
    ) -> Self {
        let haystack = [
            explicit.unwrap_or_default(),
            provider_id.unwrap_or_default(),
            canonical_model_id.unwrap_or_default(),
            upstream_model_id.unwrap_or_default(),
            model_id,
        ]
        .join(" ")
        .to_ascii_lowercase();

        let id = match explicit.map(normalize_profile_name).as_deref() {
            Some("deepseek") => ModelProfileId::DeepSeek,
            Some("glm") | Some("zai") | Some("z_ai") | Some("zhipuai") => ModelProfileId::Glm,
            Some("kimi") => ModelProfileId::Kimi,
            Some("minimax") => ModelProfileId::MiniMax,
            Some("mimo") => ModelProfileId::MiMo,
            Some("qwen") => ModelProfileId::Qwen,
            Some("openrouter_compatible") | Some("openai_compatible") => {
                ModelProfileId::OpenAICompatible
            }
            _ if haystack.contains("deepseek") => ModelProfileId::DeepSeek,
            _ if haystack.contains("glm")
                || haystack.contains("z-ai")
                || haystack.contains("z_ai")
                || haystack.contains("zai")
                || haystack.contains("zhipu") =>
            {
                ModelProfileId::Glm
            }
            _ if haystack.contains("kimi") || haystack.contains("moonshot") => ModelProfileId::Kimi,
            _ if haystack.contains("minimax") => ModelProfileId::MiniMax,
            _ if haystack.contains("mimo") => ModelProfileId::MiMo,
            _ if haystack.contains("qwen") => ModelProfileId::Qwen,
            _ => ModelProfileId::OpenAICompatible,
        };

        let mut profile = match id {
            ModelProfileId::DeepSeek => Self {
                id,
                supports_generic_reasoning_effort: false,
                supports_parallel_tool_calls: false,
                strict_tool_schema: true,
                default_temperature: Some(0.0),
                default_max_output_tokens: Some(8192),
                max_tool_calls_per_turn: 640,
                max_consecutive_tool_errors: 40,
            },
            ModelProfileId::Glm => Self {
                id,
                supports_generic_reasoning_effort: false,
                supports_parallel_tool_calls: false,
                strict_tool_schema: true,
                default_temperature: Some(0.0),
                default_max_output_tokens: Some(8192),
                max_tool_calls_per_turn: 480,
                max_consecutive_tool_errors: 40,
            },
            ModelProfileId::Kimi => Self {
                id,
                supports_generic_reasoning_effort: false,
                supports_parallel_tool_calls: false,
                strict_tool_schema: true,
                default_temperature: Some(0.0),
                default_max_output_tokens: Some(8192),
                max_tool_calls_per_turn: 640,
                max_consecutive_tool_errors: 40,
            },
            ModelProfileId::MiniMax => Self {
                id,
                supports_generic_reasoning_effort: false,
                supports_parallel_tool_calls: false,
                strict_tool_schema: false,
                default_temperature: Some(0.0),
                default_max_output_tokens: Some(16_384),
                max_tool_calls_per_turn: 480,
                max_consecutive_tool_errors: 40,
            },
            ModelProfileId::MiMo => Self {
                id,
                supports_generic_reasoning_effort: false,
                supports_parallel_tool_calls: false,
                strict_tool_schema: false,
                default_temperature: Some(0.0),
                default_max_output_tokens: Some(8192),
                max_tool_calls_per_turn: 400,
                max_consecutive_tool_errors: 40,
            },
            ModelProfileId::Qwen => Self {
                id,
                supports_generic_reasoning_effort: false,
                supports_parallel_tool_calls: false,
                strict_tool_schema: true,
                default_temperature: Some(0.0),
                default_max_output_tokens: Some(8192),
                max_tool_calls_per_turn: 640,
                max_consecutive_tool_errors: 40,
            },
            ModelProfileId::OpenAICompatible => Self {
                id,
                supports_generic_reasoning_effort: false,
                supports_parallel_tool_calls: true,
                strict_tool_schema: false,
                default_temperature: Some(0.0),
                default_max_output_tokens: Some(8192),
                max_tool_calls_per_turn: 800,
                max_consecutive_tool_errors: 40,
            },
        };

        if let Some(capabilities) = capabilities {
            if let Some(parallel) = capabilities.parallel_tool_calls {
                profile.supports_parallel_tool_calls = parallel;
            }
            if let Some(reasoning) = capabilities.reasoning {
                profile.supports_generic_reasoning_effort =
                    reasoning && matches!(profile.id, ModelProfileId::OpenAICompatible);
            }
        }

        profile
    }

    pub fn apply_provider_options(
        &self,
        body: &mut Value,
        provider_options: &HashMap<String, Value>,
    ) {
        match self.id {
            ModelProfileId::Glm => {
                body["thinking"] = json!({
                    "type": "enabled",
                    "clear_thinking": false,
                });
            }
            ModelProfileId::OpenAICompatible => {
                if let Some(provider) = provider_options.get("provider") {
                    if provider.is_object() {
                        body["provider"] = provider.clone();
                    }
                }
                if let Some(reasoning) = provider_options.get("reasoning") {
                    if reasoning.is_object() {
                        body["reasoning"] = reasoning.clone();
                    }
                }
                if let Some(usage) = provider_options.get("usage") {
                    if usage.is_object() {
                        body["usage"] = usage.clone();
                    }
                }
            }
            ModelProfileId::DeepSeek
            | ModelProfileId::Kimi
            | ModelProfileId::MiniMax
            | ModelProfileId::MiMo
            | ModelProfileId::Qwen => {}
        }
    }

    pub fn sanitize_tool_definition(&self, tool: ToolDefinition) -> ToolDefinition {
        ToolDefinition {
            parameters: sanitize_schema(tool.parameters, self.strict_tool_schema),
            ..tool
        }
    }
}

fn normalize_profile_name(value: &str) -> String {
    value.trim().to_ascii_lowercase().replace(['-', '/'], "_")
}

pub fn sanitize_schema(value: Value, strict: bool) -> Value {
    let root = value.clone();
    sanitize_schema_inner(value, &root, strict)
}

fn sanitize_schema_inner(value: Value, root: &Value, strict: bool) -> Value {
    match value {
        Value::Object(mut map) => {
            for key in [
                "$schema",
                "$id",
                "$defs",
                "definitions",
                "examples",
                "default",
                "deprecated",
                "readOnly",
                "writeOnly",
                "unevaluatedProperties",
            ] {
                map.remove(key);
            }

            for keyword in ["oneOf", "anyOf", "allOf"] {
                if let Some(Value::Array(items)) = map.remove(keyword) {
                    if let Some(first) = items
                        .into_iter()
                        .find(|item| item.get("type").and_then(Value::as_str) != Some("null"))
                    {
                        if let Some(object) = first.as_object() {
                            for (key, value) in object {
                                map.entry(key.clone()).or_insert_with(|| value.clone());
                            }
                        }
                    }
                }
            }

            // A nullable schema can flatten from `anyOf: [$ref, null]` into a
            // direct `$ref`, so resolve references only after that rewrite.
            if let Some(reference) = map.get("$ref").and_then(Value::as_str) {
                if let Some(resolved) = resolve_local_ref(root, reference) {
                    return sanitize_schema_inner(resolved.clone(), root, strict);
                }
            }

            if let Some(Value::Array(types)) = map.get_mut("type") {
                let non_null = types
                    .iter()
                    .filter(|item| item.as_str() != Some("null"))
                    .cloned()
                    .collect::<Vec<_>>();
                if non_null.len() == 1 {
                    map.insert("type".to_string(), non_null[0].clone());
                }
            }

            let keys = map.keys().cloned().collect::<Vec<_>>();
            for key in keys {
                if let Some(child) = map.remove(&key) {
                    map.insert(key, sanitize_schema_inner(child, root, strict));
                }
            }

            if strict
                && map.get("type").and_then(Value::as_str) == Some("object")
                && !map.contains_key("additionalProperties")
            {
                map.insert("additionalProperties".to_string(), Value::Bool(false));
            }

            Value::Object(map)
        }
        Value::Array(items) => Value::Array(
            items
                .into_iter()
                .map(|item| sanitize_schema_inner(item, root, strict))
                .collect(),
        ),
        other => other,
    }
}

fn resolve_local_ref<'a>(root: &'a Value, reference: &str) -> Option<&'a Value> {
    let pointer = reference.strip_prefix('#')?;
    root.pointer(pointer)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn model_profiles_use_quadrupled_operational_limits() {
        let cases = [
            ("deepseek", 640),
            ("glm", 480),
            ("kimi", 640),
            ("minimax", 480),
            ("mimo", 400),
            ("qwen", 640),
            ("openai_compatible", 800),
        ];
        for (profile_name, expected_tool_calls) in cases {
            let profile = ModelProfile::detect(Some(profile_name), None, None, None, "model", None);
            assert_eq!(
                profile.max_tool_calls_per_turn, expected_tool_calls,
                "profile {profile_name}"
            );
            assert_eq!(
                profile.max_consecutive_tool_errors, 40,
                "profile {profile_name}"
            );
        }
    }

    #[test]
    fn detects_open_weight_profiles_from_model_route_metadata() {
        assert_eq!(
            ModelProfile::detect(
                None,
                Some("novita"),
                Some("glm-5.2"),
                Some("z-ai/glm-5.2"),
                "glm-5.2",
                None
            )
            .id,
            ModelProfileId::Glm
        );
        assert_eq!(
            ModelProfile::detect(
                None,
                Some("xiaomi"),
                Some("mimo-v2.5-pro"),
                Some("xiaomi/mimo-v2.5-pro"),
                "mimo-v2.5-pro",
                None
            )
            .id,
            ModelProfileId::MiMo
        );
        assert_eq!(
            ModelProfile::detect(
                None,
                Some("atlascloud"),
                Some("qwen3.7-plus"),
                Some("qwen/qwen3.7-plus"),
                "qwen3.7-plus",
                None
            )
            .id,
            ModelProfileId::Qwen
        );
    }

    #[test]
    fn inlines_nullable_ref_tool_schema_after_flattening() {
        let schema = json!({
            "type": "object",
            "$defs": {
                "item": {
                    "type": ["string", "null"],
                    "default": "x",
                    "examples": ["x"]
                }
            },
            "definitions": {
                "grep_search_mode": {
                    "type": "string",
                    "enum": ["plain", "regex", "fuzzy", "auto"]
                }
            },
            "properties": {
                "name": { "$ref": "#/$defs/item" },
                "choice": {
                    "anyOf": [
                        {"type": "null"},
                        {"type": "string", "enum": ["a", "b"]}
                    ]
                },
                "mode": {
                    "anyOf": [
                        {"$ref": "#/definitions/grep_search_mode"},
                        {"type": "null"}
                    ]
                }
            },
            "required": ["name"]
        });

        let sanitized = sanitize_schema(schema, true);

        assert!(sanitized.get("$defs").is_none());
        assert_eq!(sanitized["properties"]["name"]["type"], "string");
        assert!(sanitized["properties"]["name"].get("default").is_none());
        assert_eq!(sanitized["properties"]["choice"]["type"], "string");
        assert_eq!(sanitized["properties"]["mode"]["type"], "string");
        assert_eq!(
            sanitized["properties"]["mode"]["enum"],
            json!(["plain", "regex", "fuzzy", "auto"])
        );
        assert!(sanitized["properties"]["mode"].get("$ref").is_none());
        assert_eq!(sanitized["additionalProperties"], false);
        assert_eq!(sanitized["required"][0], "name");
    }
}
