use std::path::Path;

use domain::{AgentDefinition, SessionId, SystemPromptSegment};
use mcp::McpRegistry;

use crate::history::{
    append_model_message, load_model_history, load_session_context, persist_session_context,
    seed_model_history_from_session_history,
};
use crate::primitives::{AgentMessage, MessagePart};
use crate::{Result, TurnInput};

use super::capture_preloaded_context_error;

pub(super) async fn build_initial_messages(
    snapshot: &AgentDefinition,
    workspace_root: &Path,
    session_id: &SessionId,
    input: TurnInput,
    event_repo: Option<&dyn storage::EventRepo>,
    mcp_registry: Option<&McpRegistry>,
) -> Result<Vec<AgentMessage>> {
    let session_context = if input.session_context.is_empty() {
        match load_session_context(event_repo, session_id).await {
            Ok(context) => context,
            Err(error) => {
                capture_preloaded_context_error(session_id, "load", &error.to_string());
                Vec::new()
            }
        }
    } else {
        if let Err(error) =
            persist_session_context(event_repo, session_id, &input.session_context).await
        {
            capture_preloaded_context_error(session_id, "persist", &error.to_string());
        }
        input.session_context.clone()
    };
    let mut messages = vec![
        AgentMessage::system(render_cacheable_system_prompt(snapshot)),
        AgentMessage::system(
            render_dynamic_system_prompt(snapshot, workspace_root, mcp_registry, &session_context)
                .await,
        ),
    ];
    let mut history = load_model_history(event_repo, session_id, 1000).await?;
    if history.is_empty() && !input.prior_history.is_empty() {
        history =
            seed_model_history_from_session_history(event_repo, session_id, &input.prior_history)
                .await?;
    }
    messages.extend(history);
    let mut user = AgentMessage::user(input.text);
    for image in input.images {
        user.push_part(MessagePart::InlineData {
            mime_type: image.mime_type,
            data: image.data,
        });
    }
    append_model_message(event_repo, session_id, &user).await?;
    messages.push(user);
    Ok(messages)
}

pub(super) fn render_cacheable_system_prompt(snapshot: &AgentDefinition) -> String {
    let mut prompt = String::new();
    for segment in &snapshot.system_prompt.cacheable_segments {
        append_rendered_segment(&mut prompt, render_static_segment(segment));
    }
    prompt
}

pub(super) async fn render_dynamic_system_prompt(
    snapshot: &AgentDefinition,
    workspace_root: &Path,
    mcp_registry: Option<&McpRegistry>,
    session_context: &[String],
) -> String {
    let mut prompt = String::new();
    let skill_store = skills::SkillStore::new(workspace_root);
    let skill_summaries = skill_store.summaries(None);
    let mcp_tools = match mcp_registry {
        Some(registry) => registry.available_tool_names(),
        None => Vec::new(),
    };
    let renders_legacy_context_segment = snapshot
        .system_prompt
        .dynamic_segments
        .iter()
        .any(|segment| matches!(segment, SystemPromptSegment::DynamicContext(_)));
    if !renders_legacy_context_segment {
        append_rendered_segment(&mut prompt, render_session_context(session_context));
    }
    for segment in &snapshot.system_prompt.dynamic_segments {
        let rendered = match segment {
            SystemPromptSegment::StaticText(_) => render_static_segment(segment),
            SystemPromptSegment::DynamicContext(config) => {
                render_dynamic_context_segment(config, session_context)
            }
            SystemPromptSegment::SkillCatalog(config) => {
                render_skill_catalog_segment(config, &skill_summaries)
            }
            SystemPromptSegment::McpTools(config) => render_tool_list_segment(config, &mcp_tools),
        };
        append_rendered_segment(&mut prompt, rendered);
    }

    prompt
}

fn render_session_context(session_context: &[String]) -> Option<String> {
    let sections = session_context
        .iter()
        .map(|context| context.trim())
        .filter(|context| !context.is_empty())
        .collect::<Vec<_>>();
    if sections.is_empty() {
        None
    } else {
        Some(sections.join("\n\n"))
    }
}

fn append_rendered_segment(prompt: &mut String, rendered: Option<String>) {
    let Some(rendered) = rendered else {
        return;
    };
    let rendered = rendered.trim();
    if rendered.is_empty() {
        return;
    }
    if !prompt.is_empty() {
        prompt.push_str("\n\n");
    }
    prompt.push_str(rendered);
}

fn render_static_segment(segment: &SystemPromptSegment) -> Option<String> {
    let SystemPromptSegment::StaticText(config) = segment else {
        return None;
    };
    Some(render_section(&config.title, &config.content))
}

fn render_dynamic_context_segment(
    config: &domain::DynamicContextPromptSegment,
    dynamic_context: &[String],
) -> Option<String> {
    let mut items = Vec::new();
    for context in dynamic_context {
        let content = context.trim();
        if content.is_empty() {
            continue;
        }
        items.push(apply_template(
            &config.item_template,
            &[("content", content)],
        ));
    }
    if items.is_empty() {
        let content = config.preamble.trim();
        if !config.title.trim().is_empty() || !content.is_empty() {
            return Some(render_section(&config.title, content));
        }
        return None;
    }
    render_item_section(&config.title, &config.preamble, &[], &items, &[])
}

fn render_skill_catalog_segment(
    config: &domain::ListPromptSegment,
    skills: &[skills::SkillSummary],
) -> Option<String> {
    let items = skills
        .iter()
        .map(|skill| {
            apply_template(
                &config.item_template,
                &[
                    ("name", skill.name.as_str()),
                    ("description", skill.description.as_str()),
                ],
            )
        })
        .collect::<Vec<_>>();
    render_item_section(&config.title, &config.preamble, &[], &items, &[])
}

fn render_tool_list_segment(
    config: &domain::ListPromptSegment,
    tools: &[String],
) -> Option<String> {
    let items = tools
        .iter()
        .map(|name| apply_template(&config.item_template, &[("name", name.as_str())]))
        .collect::<Vec<_>>();
    render_item_section(&config.title, &config.preamble, &[], &items, &[])
}

fn render_item_section(
    title: &str,
    preamble: &str,
    before_items: &[String],
    items: &[String],
    after_items: &[String],
) -> Option<String> {
    if items.is_empty() {
        return None;
    }
    let mut lines = Vec::new();
    let preamble = preamble.trim();
    if !preamble.is_empty() {
        lines.push(preamble.to_string());
    }
    lines.extend(before_items.iter().filter_map(|line| nonempty_line(line)));
    lines.extend(items.iter().filter_map(|line| nonempty_line(line)));
    lines.extend(after_items.iter().filter_map(|line| nonempty_line(line)));
    if lines.is_empty() {
        return None;
    }
    Some(render_section(title, &lines.join("\n")))
}

fn render_section(title: &str, content: &str) -> String {
    let title = title.trim();
    let content = content.trim();
    if title.is_empty() {
        content.to_string()
    } else if content.is_empty() {
        format!("## {title}")
    } else {
        format!("## {title}\n{content}")
    }
}

fn nonempty_line(value: &str) -> Option<String> {
    let value = value.trim();
    if value.is_empty() {
        None
    } else {
        Some(value.to_string())
    }
}

fn apply_template(template: &str, replacements: &[(&str, &str)]) -> String {
    let mut output = template.to_string();
    for (key, value) in replacements {
        output = output.replace(&format!("{{{key}}}"), value.trim());
    }
    output
}
