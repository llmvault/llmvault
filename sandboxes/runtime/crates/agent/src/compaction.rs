use std::collections::HashSet;

use domain::CompactionConfig;

use crate::primitives::{AgentMessage, AgentMessageRole, MessagePart};

const DEFAULT_CONTEXT_WINDOW: u64 = 128_000;
const DEFAULT_TOKEN_THRESHOLD: u64 = 100_000;

/// Truncate `text` to at most `max_bytes` bytes without splitting a UTF-8
/// character, appending `"..."` when truncation occurs.
///
/// Slicing a `&str` at an arbitrary byte offset panics when the offset lands in
/// the middle of a multi-byte UTF-8 sequence (emoji, CJK, accented text). This
/// helper backs the cut point down to the nearest char boundary so summary
/// building can never panic and kill the turn task.
fn truncate_on_char_boundary(text: &str, max_bytes: usize) -> String {
    if text.len() <= max_bytes {
        return text.to_string();
    }
    let mut boundary = max_bytes;
    while boundary > 0 && !text.is_char_boundary(boundary) {
        boundary -= 1;
    }
    format!("{}...", &text[..boundary])
}

#[derive(Debug, Clone)]
pub struct CompactContext {
    pub estimated_tokens: u64,
    pub context_window: u64,
    pub user_turns: u32,
    pub message_count: u32,
    pub last_message_is_user: bool,
}

impl CompactContext {
    pub fn from_messages(messages: &[AgentMessage]) -> Self {
        let estimated_tokens = estimate_tokens(messages);
        let mut user_turns = 0u32;
        let last_message_is_user =
            matches!(messages.last(), Some(m) if m.role == AgentMessageRole::User);

        for msg in messages {
            if msg.role == AgentMessageRole::User {
                user_turns += 1;
            }
        }

        Self {
            estimated_tokens,
            context_window: 0,
            user_turns,
            message_count: messages.len() as u32,
            last_message_is_user,
        }
    }

    pub fn with_context_window(mut self, window: u64) -> Self {
        self.context_window = window;
        self
    }
}

pub fn should_compact(context: &CompactContext, config: &CompactionConfig) -> bool {
    config
        .token_threshold
        .map(|t| context.estimated_tokens >= t as u64)
        .unwrap_or(false)
        || config
            .token_threshold_percentage
            .map(|p| {
                let window = if context.context_window > 0 {
                    context.context_window
                } else {
                    DEFAULT_CONTEXT_WINDOW
                };
                context.estimated_tokens as f64 >= window as f64 * p
            })
            .unwrap_or(false)
        || config
            .turn_threshold
            .map(|t| context.user_turns >= t)
            .unwrap_or(false)
        || config
            .message_threshold
            .map(|t| context.message_count >= t)
            .unwrap_or(false)
        || config.on_turn_end.unwrap_or(false) && context.last_message_is_user
}

pub fn effective_token_threshold(config: &CompactionConfig) -> u64 {
    let from_absolute = config.token_threshold.map(|t| t as u64);
    let from_percentage = config
        .token_threshold_percentage
        .map(|p| (DEFAULT_CONTEXT_WINDOW as f64 * p) as u64);
    from_absolute
        .or(from_percentage)
        .unwrap_or(DEFAULT_TOKEN_THRESHOLD)
}

pub fn compact(messages: &mut Vec<AgentMessage>, config: &CompactionConfig) -> u64 {
    let total_before = estimate_tokens(messages);
    let retention = config.overlap_event_count.max(1) as usize;

    let Some(range) = find_eviction_range(messages, retention) else {
        return 0;
    };

    let summary = build_structured_summary(&messages[range.clone()]);
    messages.splice(range, std::iter::once(AgentMessage::user(summary)));

    let total_after = estimate_tokens(messages);
    total_before.saturating_sub(total_after)
}

fn find_eviction_range(
    messages: &[AgentMessage],
    retention: usize,
) -> Option<std::ops::Range<usize>> {
    let len = messages.len();
    if len <= 2 {
        return None;
    }

    let start = messages.iter().position(|msg| {
        msg.role == AgentMessageRole::Assistant || msg.role == AgentMessageRole::User
    })?;

    let end = len.saturating_sub(retention).saturating_sub(1);
    if start >= end || end == 0 {
        return None;
    }

    // If the eviction boundary falls inside a tool run (the retained side would
    // begin with `Tool` results), walk the boundary back to the assistant
    // message that issued the matching `tool_calls`. Evicting that assistant
    // message while keeping its tool results — or vice versa — produces history
    // the provider rejects with 400 (orphaned tool results / dangling
    // tool_calls). Pulling the boundary back to the start of the tool run keeps
    // the assistant + its tool results together on the retained side.
    if messages[end].role == AgentMessageRole::Tool {
        let mut tool_start = end;
        while tool_start > start && messages[tool_start].role == AgentMessageRole::Tool {
            tool_start -= 1;
        }
        // `tool_start` now points at the assistant message carrying tool_calls
        // (or the first message of the run if it has no preceding assistant).
        // Evict up to but excluding it so the whole tool run is retained.
        if tool_start > start {
            return Some(start..tool_start);
        }
        return None;
    }

    Some(start..end)
}

fn build_structured_summary(messages: &[AgentMessage]) -> String {
    let entries: Vec<SummaryEntry> = messages.iter().filter_map(build_entry).collect();
    let deduped = deduplicate_entries(entries);
    render_summary_template(&deduped)
}

#[derive(Debug, Clone)]
enum SummaryEntry {
    UserText {
        text: String,
    },
    AssistantText {
        text: String,
    },
    ToolCall {
        name: String,
        args: String,
        success: Option<bool>,
    },
    SystemMsg {
        text: String,
    },
}

fn build_entry(msg: &AgentMessage) -> Option<SummaryEntry> {
    match msg.role {
        AgentMessageRole::User => {
            let text = msg
                .parts
                .iter()
                .map(|p| match p {
                    MessagePart::Text { text } => text.as_str(),
                })
                .collect::<Vec<_>>()
                .join("\n");
            if text.trim().is_empty() {
                None
            } else {
                Some(SummaryEntry::UserText {
                    text: text.trim().to_string(),
                })
            }
        }
        AgentMessageRole::Assistant if !msg.tool_calls.is_empty() => {
            let call = &msg.tool_calls[0];
            let args = serde_json::to_string(&call.arguments).unwrap_or_else(|_| "{}".to_string());
            let args_short = truncate_on_char_boundary(&args, 200);
            Some(SummaryEntry::ToolCall {
                name: call.name.clone(),
                args: args_short,
                success: None,
            })
        }
        AgentMessageRole::Assistant => {
            let text = msg
                .parts
                .iter()
                .map(|p| match p {
                    MessagePart::Text { text } => text.as_str(),
                })
                .collect::<Vec<_>>()
                .join("\n");
            if text.trim().is_empty() {
                None
            } else {
                Some(SummaryEntry::AssistantText {
                    text: text.trim().to_string(),
                })
            }
        }
        AgentMessageRole::Tool => {
            let text = msg
                .parts
                .iter()
                .map(|p| match p {
                    MessagePart::Text { text } => text.as_str(),
                })
                .collect::<Vec<_>>()
                .join("\n");
            let is_success = !text.contains("error") && !text.contains("Error");
            let text = format_tool_result(&text);
            Some(SummaryEntry::ToolCall {
                name: "tool_result".to_string(),
                args: text,
                success: Some(is_success),
            })
        }
        AgentMessageRole::System => {
            let text = msg
                .parts
                .iter()
                .map(|p| match p {
                    MessagePart::Text { text } => text.as_str(),
                })
                .collect::<Vec<_>>()
                .join("\n");
            let trimmed = text.trim().to_string();
            if trimmed.is_empty() || is_prompt_segment(&trimmed) {
                None
            } else {
                Some(SummaryEntry::SystemMsg { text: trimmed })
            }
        }
    }
}

fn is_prompt_segment(text: &str) -> bool {
    text.contains("## ") && text.len() > 500
}

fn format_tool_result(text: &str) -> String {
    truncate_on_char_boundary(text, 300)
}

fn deduplicate_entries(entries: Vec<SummaryEntry>) -> Vec<SummaryEntry> {
    let mut seen_paths: HashSet<String> = HashSet::new();
    let mut result = Vec::new();

    for entry in entries {
        let should_keep = match &entry {
            SummaryEntry::ToolCall { name, args, .. }
                if name == "read_file" || name == "write_file" || name == "edit_file" =>
            {
                if let Some(path) = extract_file_path(args) {
                    let key = format!("{name}:{path}");
                    !seen_paths.contains(&key)
                } else {
                    true
                }
            }
            _ => true,
        };

        if should_keep {
            if let SummaryEntry::ToolCall { name, args, .. } = &entry {
                if name == "read_file" || name == "write_file" || name == "edit_file" {
                    if let Some(path) = extract_file_path(args) {
                        seen_paths.insert(format!("{name}:{path}"));
                    }
                }
            }
            result.push(entry);
        }
    }

    result
}

fn extract_file_path(args: &str) -> Option<String> {
    let needle = "\"path\":";
    if let Some(pos) = args.find(needle) {
        let rest = &args[pos + needle.len()..].trim();
        let inner = rest.strip_prefix('"')?;
        let end = inner.find('"')?;
        Some(inner[..end].to_string())
    } else {
        None
    }
}

fn render_summary_template(entries: &[SummaryEntry]) -> String {
    let mut lines = vec![
        "Conversation summary (compacted)".to_string(),
        String::new(),
    ];

    for (idx, entry) in entries.iter().enumerate() {
        let num = idx + 1;
        match entry {
            SummaryEntry::UserText { text } => {
                let short = truncate_on_char_boundary(text, 200);
                lines.push(format!("{num}. [User] {short}"));
            }
            SummaryEntry::AssistantText { text } => {
                let short = truncate_on_char_boundary(text, 200);
                lines.push(format!("{num}. [Assistant] {short}"));
            }
            SummaryEntry::ToolCall {
                name,
                args,
                success,
            } => {
                let status = match success {
                    Some(true) => " ok",
                    Some(false) => " failed",
                    None => "",
                };
                lines.push(format!("{num}. [{name}{status}] {args}"));
            }
            SummaryEntry::SystemMsg { text } => {
                let short = truncate_on_char_boundary(text, 200);
                lines.push(format!("{num}. [System] {short}"));
            }
        }
    }

    lines.push(String::new());
    lines.push("Continue based on this context.".to_string());
    lines.join("\n")
}

fn estimate_tokens(messages: &[AgentMessage]) -> u64 {
    estimate_tokens_static(messages)
}

pub fn estimate_tokens_static(messages: &[AgentMessage]) -> u64 {
    let chars: usize = messages
        .iter()
        .flat_map(|msg| msg.parts.iter())
        .map(|part| match part {
            MessagePart::Text { text } => text.len(),
        })
        .sum();
    (chars as u64 / 4).max(1)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::primitives::AgentMessage;

    fn make_msg(role: AgentMessageRole, text: &str) -> AgentMessage {
        match role {
            AgentMessageRole::System => AgentMessage::system(text),
            AgentMessageRole::User => AgentMessage::user(text),
            AgentMessageRole::Assistant => AgentMessage::assistant(text),
            AgentMessageRole::Tool => AgentMessage::tool_result("t1", text),
        }
    }

    #[test]
    fn empty_messages_no_compaction() {
        let messages: Vec<AgentMessage> = vec![];
        let ctx = CompactContext::from_messages(&messages);
        let config = CompactionConfig {
            enabled: true,
            token_threshold: Some(10),
            token_threshold_percentage: None,
            turn_threshold: None,
            message_threshold: None,
            eviction_window: 0.2,
            retention_window: 0,
            overlap_event_count: 2,
            chars_per_token: 4,
            on_turn_end: None,
        };
        assert!(!should_compact(&ctx, &config));
    }

    #[test]
    fn token_threshold_triggers() {
        let mut messages: Vec<AgentMessage> = Vec::new();
        for _ in 0..50 {
            messages.push(make_msg(AgentMessageRole::User, "a".repeat(100).as_str()));
        }
        let ctx = CompactContext::from_messages(&messages);
        let config = CompactionConfig {
            enabled: true,
            token_threshold: Some(100),
            token_threshold_percentage: None,
            turn_threshold: None,
            message_threshold: None,
            eviction_window: 0.2,
            retention_window: 0,
            overlap_event_count: 2,
            chars_per_token: 4,
            on_turn_end: None,
        };
        assert!(should_compact(&ctx, &config));
    }

    #[test]
    fn message_threshold_triggers() {
        let messages: Vec<AgentMessage> = vec![
            make_msg(AgentMessageRole::System, "sys"),
            make_msg(AgentMessageRole::User, "hi"),
            make_msg(AgentMessageRole::Assistant, "hello"),
            make_msg(AgentMessageRole::User, "more"),
        ];
        let ctx = CompactContext::from_messages(&messages);
        let config = CompactionConfig {
            enabled: true,
            token_threshold: None,
            token_threshold_percentage: None,
            turn_threshold: None,
            message_threshold: Some(3),
            eviction_window: 0.2,
            retention_window: 0,
            overlap_event_count: 2,
            chars_per_token: 4,
            on_turn_end: None,
        };
        assert!(should_compact(&ctx, &config));
    }

    #[test]
    fn compaction_reduces_message_count() {
        let mut messages: Vec<AgentMessage> = Vec::new();
        messages.push(make_msg(AgentMessageRole::System, "sys prompt"));
        messages.push(make_msg(AgentMessageRole::User, "build a habit tracker"));
        messages.push(make_msg(AgentMessageRole::Assistant, ""));
        messages[2].tool_calls = vec![crate::primitives::ToolCall {
            id: "c1".into(),
            name: "bash".into(),
            arguments: serde_json::json!({"command": "ls"}),
        }];
        messages.push(make_msg(AgentMessageRole::Tool, r#"{"status":"ok"}"#));
        messages.push(make_msg(AgentMessageRole::Assistant, "I'll build it"));
        messages.push(make_msg(AgentMessageRole::User, "go ahead"));

        let before = messages.len();
        let config = CompactionConfig {
            enabled: true,
            token_threshold: Some(10),
            token_threshold_percentage: None,
            turn_threshold: None,
            message_threshold: None,
            eviction_window: 0.2,
            retention_window: 0,
            overlap_event_count: 1,
            chars_per_token: 4,
            on_turn_end: None,
        };
        compact(&mut messages, &config);
        assert!(messages.len() < before);
    }

    fn assistant_with_tool_call(id: &str) -> AgentMessage {
        let mut msg = make_msg(AgentMessageRole::Assistant, "");
        msg.tool_calls = vec![crate::primitives::ToolCall {
            id: id.into(),
            name: "bash".into(),
            arguments: serde_json::json!({"command": "ls"}),
        }];
        msg
    }

    #[test]
    fn truncate_on_char_boundary_handles_multibyte_at_cut_point() {
        // A run of 4-byte emoji: slicing at any non-multiple-of-4 byte offset
        // would land mid-character and panic with naive `&s[..n]`.
        let text = "😀".repeat(100);
        let truncated = truncate_on_char_boundary(&text, 201);
        assert!(truncated.ends_with("..."));
        // 201 bytes -> 200 is the nearest lower char boundary (50 emoji).
        assert_eq!(truncated, format!("{}...", "😀".repeat(50)));
    }

    #[test]
    fn truncate_on_char_boundary_returns_short_text_unchanged() {
        assert_eq!(truncate_on_char_boundary("hello", 200), "hello");
    }

    #[test]
    fn compaction_does_not_panic_on_multibyte_content() {
        // Long multi-byte user/assistant/tool content that crosses the 200/300
        // byte truncation boundaries inside a multi-byte character.
        let emoji_blob = "🚀".repeat(400);
        let mut messages: Vec<AgentMessage> = vec![
            make_msg(AgentMessageRole::System, "sys prompt"),
            make_msg(AgentMessageRole::User, &emoji_blob),
            make_msg(AgentMessageRole::Assistant, &emoji_blob),
            make_msg(AgentMessageRole::User, "follow up"),
            make_msg(AgentMessageRole::Assistant, &emoji_blob),
            make_msg(AgentMessageRole::User, "go ahead"),
        ];
        let before = messages.len();
        let config = CompactionConfig {
            enabled: true,
            token_threshold: Some(10),
            token_threshold_percentage: None,
            turn_threshold: None,
            message_threshold: None,
            eviction_window: 0.2,
            retention_window: 0,
            overlap_event_count: 1,
            chars_per_token: 4,
            on_turn_end: None,
        };
        // Must not panic on byte-offset slicing of multi-byte content.
        compact(&mut messages, &config);
        assert!(messages.len() < before);
    }

    #[test]
    fn eviction_keeps_assistant_tool_calls_with_tool_results() {
        // History: system, user, assistant(tool_call), tool_result, assistant, user
        // The eviction boundary must not split the assistant(tool_call) from its
        // tool_result. With retention=2 the boundary lands on the tool_result, so
        // the range must stop before the assistant(tool_call) message.
        let messages: Vec<AgentMessage> = vec![
            make_msg(AgentMessageRole::System, "sys"),
            make_msg(AgentMessageRole::User, "do a thing"),
            assistant_with_tool_call("c1"),
            make_msg(AgentMessageRole::Tool, r#"{"status":"ok"}"#),
            make_msg(AgentMessageRole::Assistant, "done"),
            make_msg(AgentMessageRole::User, "thanks"),
        ];

        // end = 6 - 2 - 1 = 3 -> the tool_result message.
        let range = find_eviction_range(&messages, 2).expect("eviction range should be produced");

        // The retained tail (range.end onward) must not start with a Tool message
        // that has no preceding assistant tool_calls message.
        let retained_head = &messages[range.end];
        assert_ne!(
            retained_head.role,
            AgentMessageRole::Tool,
            "retained history must not begin with an orphaned tool result"
        );
        // The assistant message carrying tool_calls and its tool result must both
        // be on the same side of the boundary (both retained here).
        let assistant_idx = 2;
        let tool_idx = 3;
        let assistant_evicted = range.contains(&assistant_idx);
        let tool_evicted = range.contains(&tool_idx);
        assert_eq!(
            assistant_evicted, tool_evicted,
            "assistant tool_calls and its tool result must stay together"
        );
    }

    #[test]
    fn eviction_keeps_multi_call_tool_run_together() {
        // Multiple consecutive tool results following one assistant tool_calls
        // message must all stay grouped with that assistant message.
        let messages: Vec<AgentMessage> = vec![
            make_msg(AgentMessageRole::System, "sys"),
            make_msg(AgentMessageRole::User, "u1"),
            make_msg(AgentMessageRole::Assistant, "a1"),
            make_msg(AgentMessageRole::User, "u2"),
            assistant_with_tool_call("c1"),
            make_msg(AgentMessageRole::Tool, r#"{"r":1}"#),
            make_msg(AgentMessageRole::Tool, r#"{"r":2}"#),
            make_msg(AgentMessageRole::Assistant, "final"),
            make_msg(AgentMessageRole::User, "u3"),
        ];

        // retention=3 -> end = 9-3-1 = 5, which is a Tool message mid-run.
        let range = find_eviction_range(&messages, 3).expect("eviction range should be produced");

        assert_ne!(
            messages[range.end].role,
            AgentMessageRole::Tool,
            "retained history must not begin with an orphaned tool result"
        );
        // The assistant(tool_call) at index 4 and both tool results (5, 6) must
        // all be retained (not evicted), since the boundary fell inside the run.
        assert!(!range.contains(&4));
        assert!(!range.contains(&5));
        assert!(!range.contains(&6));
    }
}
