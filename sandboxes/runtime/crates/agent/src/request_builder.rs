use serde_json::{json, Value};

use crate::primitives::{
    AgentMessage, AgentMessageRole, CacheControlPolicy, MessagePart, ModelRequest, ToolCall,
};

pub fn build_openai_compatible_request(request: &ModelRequest) -> Value {
    let tools = request
        .tools
        .clone()
        .into_iter()
        .map(|tool| request.profile.sanitize_tool_definition(tool))
        .collect::<Vec<_>>();
    let messages = transform_messages_for_profile(&request.messages);

    let mut body = json!({
        "model": request.model,
        "messages": messages.iter().enumerate().map(|(i, m)| {
            let policy = if i == 0 && m.role == AgentMessageRole::System {
                request.cache_policy
            } else {
                CacheControlPolicy::Disabled
            };
            message_to_json(m, policy)
        }).collect::<Vec<_>>(),
        "stream": true,
        "stream_options": {"include_usage": true},
    });

    if !tools.is_empty() {
        body["tools"] = Value::Array(
            tools
                .into_iter()
                .map(|tool| {
                    json!({
                        "type": "function",
                        "function": {
                            "name": tool.name,
                            "description": tool.description,
                            "parameters": tool.parameters,
                        }
                    })
                })
                .collect(),
        );
        if request.profile.supports_parallel_tool_calls {
            body["parallel_tool_calls"] = Value::Bool(true);
        }
    }

    if let Some(temperature) = request.temperature.or(request.profile.default_temperature) {
        body["temperature"] = json!(temperature);
    }
    if let Some(max_tokens) = request
        .max_output_tokens
        .or(request.profile.default_max_output_tokens)
    {
        body["max_completion_tokens"] = json!(max_tokens);
    }
    if request.profile.supports_generic_reasoning_effort {
        if let Some(reasoning_effort) = &request.reasoning_effort {
            body["reasoning_effort"] = json!(reasoning_effort);
        }
    }

    request
        .profile
        .apply_provider_options(&mut body, &request.provider_options);

    body
}

fn transform_messages_for_profile(messages: &[AgentMessage]) -> Vec<AgentMessage> {
    let mut output = Vec::with_capacity(messages.len());
    let mut pending_system = String::new();

    for message in messages {
        if message.role == AgentMessageRole::System {
            let text = message_text(message);
            if !text.trim().is_empty() {
                if !pending_system.is_empty() {
                    pending_system.push_str("\n\n");
                }
                pending_system.push_str(text.trim());
            }
            continue;
        }

        flush_system(&mut output, &mut pending_system);
        output.push(message.clone());
    }

    flush_system(&mut output, &mut pending_system);
    output
}

fn flush_system(output: &mut Vec<AgentMessage>, pending_system: &mut String) {
    if pending_system.trim().is_empty() {
        return;
    }
    output.push(AgentMessage::system(std::mem::take(pending_system)));
}

fn message_text(message: &AgentMessage) -> String {
    message
        .parts
        .iter()
        .map(|part| match part {
            MessagePart::Text { text } => text.as_str(),
        })
        .collect::<Vec<_>>()
        .join("\n")
}

fn parts_to_tool_content(parts: &[MessagePart]) -> Value {
    let text = parts
        .iter()
        .map(|part| match part {
            MessagePart::Text { text } => text.as_str(),
        })
        .collect::<Vec<_>>()
        .join("\n");
    Value::String(text)
}

fn assistant_content_value(message: &AgentMessage, cache_policy: CacheControlPolicy) -> Value {
    if message.parts.is_empty() {
        Value::String(String::new())
    } else {
        parts_to_content(&message.parts, cache_policy)
    }
}

fn message_to_json(message: &AgentMessage, cache_policy: CacheControlPolicy) -> Value {
    match message.role {
        AgentMessageRole::System => json!({
            "role": "system",
            "content": parts_to_content(&message.parts, cache_policy),
        }),
        AgentMessageRole::User => json!({
            "role": "user",
            "content": parts_to_content(&message.parts, cache_policy),
        }),
        AgentMessageRole::Assistant => {
            let mut value = json!({"role": "assistant"});
            if !message.parts.is_empty() || message.tool_calls.is_empty() {
                value["content"] = assistant_content_value(message, cache_policy);
            }
            if !message.tool_calls.is_empty() {
                value["tool_calls"] =
                    Value::Array(message.tool_calls.iter().map(tool_call_to_json).collect());
            }
            value
        }
        AgentMessageRole::Tool => json!({
            "role": "tool",
            "tool_call_id": message.tool_call_id.clone().unwrap_or_else(|| "unknown".to_string()),
            "content": parts_to_tool_content(&message.parts),
        }),
    }
}

fn parts_to_content(parts: &[MessagePart], cache_policy: CacheControlPolicy) -> Value {
    Value::Array(
        parts
            .iter()
            .map(|part| match part {
                MessagePart::Text { text } => {
                    let mut value = json!({
                        "type": "text",
                        "text": text,
                    });
                    if cache_policy == CacheControlPolicy::Ephemeral {
                        value["cache_control"] = json!({"type": "ephemeral"});
                    }
                    value
                }
            })
            .collect(),
    )
}

fn tool_call_to_json(call: &ToolCall) -> Value {
    json!({
        "id": call.id,
        "type": "function",
        "function": {
            "name": call.name,
            "arguments": serde_json::to_string(&call.arguments).unwrap_or_else(|_| "{}".to_string()),
        }
    })
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::model_profile::ModelProfile;
    use crate::primitives::{AgentMessage, CacheControlPolicy, ModelRequest};
    use domain::ModelCapabilities;
    use std::collections::HashMap;

    fn openai_compatible_profile() -> ModelProfile {
        ModelProfile::detect(None, Some("atlascloud"), None, None, "test", None)
    }

    fn request_with_profile(profile: ModelProfile) -> ModelRequest {
        ModelRequest {
            model: "test".into(),
            messages: vec![AgentMessage::user("hi")],
            tools: vec![],
            temperature: None,
            max_output_tokens: None,
            reasoning_effort: None,
            cache_policy: CacheControlPolicy::Disabled,
            profile,
            provider_options: HashMap::new(),
        }
    }

    #[test]
    fn only_serializes_first_system_message_as_cacheable_content_block() {
        let req = ModelRequest {
            model: "test".into(),
            messages: vec![
                AgentMessage::system("sys"),
                AgentMessage::user("hi"),
                AgentMessage::assistant("hello"),
            ],
            tools: vec![],
            temperature: None,
            max_output_tokens: None,
            reasoning_effort: None,
            cache_policy: CacheControlPolicy::Ephemeral,
            profile: openai_compatible_profile(),
            provider_options: HashMap::new(),
        };
        let body = build_openai_compatible_request(&req);
        assert_eq!(body["messages"][0]["role"], "system");
        assert_eq!(body["messages"][1]["role"], "user");
        assert_eq!(body["messages"][2]["role"], "assistant");
        assert_eq!(
            body["messages"][0]["content"][0]["cache_control"]["type"],
            "ephemeral"
        );
        assert!(
            body["messages"][1]["content"][0]
                .as_object()
                .and_then(|o| o.get("cache_control"))
                .is_none(),
            "user messages must not have cache_control"
        );
        assert!(
            body["messages"][2]["content"][0]
                .as_object()
                .and_then(|o| o.get("cache_control"))
                .is_none(),
            "assistant messages must not have cache_control"
        );
    }

    #[test]
    fn preserves_tool_order_for_loaded_schema_appends() {
        let req = ModelRequest {
            model: "test".into(),
            messages: vec![AgentMessage::user("hi")],
            tools: vec![
                tools::ToolDefinition {
                    name: "z".into(),
                    description: "".into(),
                    parameters: json!({}),
                },
                tools::ToolDefinition {
                    name: "a".into(),
                    description: "".into(),
                    parameters: json!({}),
                },
            ],
            temperature: None,
            max_output_tokens: None,
            reasoning_effort: None,
            cache_policy: CacheControlPolicy::Disabled,
            profile: openai_compatible_profile(),
            provider_options: HashMap::new(),
        };
        let body = build_openai_compatible_request(&req);
        assert_eq!(body["tools"][0]["function"]["name"], "z");
        assert_eq!(body["tools"][1]["function"]["name"], "a");
    }

    #[test]
    fn omits_generic_reasoning_for_open_weight_profiles() {
        let caps = ModelCapabilities {
            reasoning: Some(true),
            tool_call: Some(true),
            parallel_tool_calls: Some(true),
            structured_output: Some(true),
        };
        let mut req = request_with_profile(ModelProfile::detect(
            None,
            Some("novita"),
            Some("deepseek-v4-flash"),
            Some("deepseek/deepseek-v4-flash"),
            "deepseek-v4-flash",
            Some(&caps),
        ));
        req.reasoning_effort = Some("high".to_string());

        let body = build_openai_compatible_request(&req);

        assert!(body.get("reasoning_effort").is_none());
    }

    #[test]
    fn emits_generic_reasoning_only_when_profile_supports_it() {
        let caps = ModelCapabilities {
            reasoning: Some(true),
            tool_call: Some(true),
            parallel_tool_calls: Some(true),
            structured_output: Some(true),
        };
        let mut req = request_with_profile(ModelProfile::detect(
            None,
            Some("atlascloud"),
            Some("grok-4.3"),
            Some("x-ai/grok-4.3"),
            "grok-4.3",
            Some(&caps),
        ));
        req.reasoning_effort = Some("high".to_string());

        let body = build_openai_compatible_request(&req);

        assert_eq!(body["reasoning_effort"], "high");
    }

    #[test]
    fn lowers_glm_profile_thinking_without_parallel_tool_calls() {
        let mut req = request_with_profile(ModelProfile::detect(
            None,
            Some("zai"),
            Some("glm-5.2"),
            Some("z-ai/glm-5.2"),
            "glm-5.2",
            None,
        ));
        req.tools = vec![tools::ToolDefinition {
            name: "lookup".into(),
            description: "Lookup".into(),
            parameters: json!({"type":"object","properties":{}}),
        }];

        let body = build_openai_compatible_request(&req);

        assert_eq!(body["thinking"]["type"], "enabled");
        assert_eq!(body["thinking"]["clear_thinking"], false);
        assert!(body.get("parallel_tool_calls").is_none());
    }

    #[test]
    fn omits_invalid_provider_options_instead_of_forwarding_them() {
        let mut req = request_with_profile(openai_compatible_profile());
        req.provider_options
            .insert("provider".into(), json!("atlascloud"));
        req.provider_options.insert("reasoning".into(), json!(true));
        req.provider_options
            .insert("usage".into(), json!({"include": true}));
        req.provider_options
            .insert("unexpected".into(), json!({"leak": true}));

        let body = build_openai_compatible_request(&req);

        assert!(body.get("provider").is_none());
        assert!(body.get("reasoning").is_none());
        assert_eq!(body["usage"]["include"], true);
        assert!(body.get("unexpected").is_none());
    }
}
