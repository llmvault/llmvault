use std::collections::VecDeque;

use domain::InboundEvent;
use serde_json::Value;

use super::{inbound_event_source, session_stream_id, trace_id, turn_id, ScheduledRunStatus};

const MAX_REGULAR_BATCHES_BEFORE_SCHEDULED: usize = 1;

#[derive(Default)]
pub(super) struct QueuedInboundBacklog {
    regular: Vec<InboundEvent>,
    scheduled: VecDeque<InboundEvent>,
    regular_batches_since_scheduled: usize,
}

impl QueuedInboundBacklog {
    pub(super) fn is_empty(&self) -> bool {
        self.regular.is_empty() && self.scheduled.is_empty()
    }
}

pub(super) fn next_queued_inbound(
    current: &InboundEvent,
    queued: Vec<InboundEvent>,
    backlog: &mut QueuedInboundBacklog,
) -> Option<InboundEvent> {
    for event in queued {
        if is_scheduled_run_inbound(&event) {
            backlog.scheduled.push_back(event);
        } else {
            backlog.regular.push(event);
        }
    }

    if !backlog.scheduled.is_empty()
        && (backlog.regular.is_empty()
            || backlog.regular_batches_since_scheduled >= MAX_REGULAR_BATCHES_BEFORE_SCHEDULED)
    {
        backlog.regular_batches_since_scheduled = 0;
        return backlog.scheduled.pop_front();
    }

    if !backlog.regular.is_empty() {
        backlog.regular_batches_since_scheduled += 1;
        let regular = std::mem::take(&mut backlog.regular);
        return Some(merge_queued_inbound(current, regular));
    }

    if let Some(scheduled) = backlog.scheduled.pop_front() {
        backlog.regular_batches_since_scheduled = 0;
        return Some(scheduled);
    }
    None
}

fn is_scheduled_run_inbound(inbound: &InboundEvent) -> bool {
    !matches!(
        ScheduledRunStatus::from_inbound(inbound),
        ScheduledRunStatus::None
    )
}

fn merge_queued_inbound(current: &InboundEvent, queued: Vec<InboundEvent>) -> InboundEvent {
    let mut merged = current.clone();
    let mut text =
        String::from("[Additional request(s) received while working on the previous task]\n");
    let mut attachments = Vec::new();
    let mut raw_events = Vec::new();
    let mut model_definition = current.model_definition.clone();
    for (index, event) in queued.into_iter().enumerate() {
        if event.model_definition.is_some() {
            model_definition = event.model_definition.clone();
        }
        let number = index + 1;
        let source = inbound_event_source(&event);
        let display_name = event
            .user_display_name
            .clone()
            .unwrap_or_else(|| event.user.clone());
        text.push_str(&format!(
            "\n{number}. Source: {source}; User: {display_name} ({})\n",
            event.user
        ));
        if !event.text.trim().is_empty() {
            text.push_str("   Text:\n");
            for line in event.text.lines() {
                text.push_str("   ");
                text.push_str(line);
                text.push('\n');
            }
        }
        if !event.attachments.is_empty() {
            text.push_str("   Attachments:\n");
            for attachment in &event.attachments {
                text.push_str(&format!(
                    "   - {} ({}, {} bytes)\n",
                    attachment.name,
                    attachment.mime_type,
                    attachment.size_bytes.unwrap_or_default()
                ));
            }
        }
        attachments.extend(event.attachments.clone());
        raw_events.push(serde_json::json!({
            "envelope_id": event.envelope_id,
            "source": source,
            "user": event.user,
            "user_display_name": event.user_display_name,
            "raw": event.raw,
        }));
    }

    merged.envelope_id = format!("{}-queued", current.envelope_id);
    merged.user = "queued-inbound".to_string();
    merged.user_display_name = Some("Queued inbound messages".to_string());
    merged.text = text;
    merged.attachments = attachments;
    merged.model_definition = model_definition;
    let mut raw = serde_json::json!({
        "source": "queued_batch",
        "events": raw_events,
    });
    // Preserve the running session stream id. Follow-up requests in the same
    // session use this same stable stream, so no terminal bridging is needed.
    if let Some(map) = raw.as_object_mut() {
        if let Some(id) = session_stream_id(current) {
            map.insert("session_stream_id".to_string(), Value::String(id));
        }
        if let Some(trace_id) = trace_id(current) {
            map.insert("trace_id".to_string(), Value::String(trace_id));
        }
        if let Some(turn_id) = turn_id(current) {
            map.insert("turn_id".to_string(), Value::String(turn_id));
        }
    }
    merged.raw = raw;
    merged
}

#[cfg(test)]
mod tests {
    use chrono::Utc;
    use domain::{Attachment, InboundEvent, SessionId};

    use super::*;
    use crate::handler::{copy_inbound_metadata, is_subagent_task_inbound};

    fn inbound(session: &str, envelope: &str, text: &str, raw: serde_json::Value) -> InboundEvent {
        InboundEvent {
            envelope_id: envelope.to_string(),
            session_id: SessionId::from(session.to_string()),
            user: "U123".to_string(),
            user_display_name: Some("Ada".to_string()),
            text: text.to_string(),
            attachments: vec![Attachment {
                url: "https://example.test/evidence.txt".to_string(),
                mime_type: "text/plain".to_string(),
                name: "evidence.txt".to_string(),
                size_bytes: Some(128),
            }],
            session_context: Vec::new(),
            model_definition: None,
            raw,
            is_direct_message: false,
            is_directly_addressed: true,
            link_previews: Vec::new(),
            agent_definition: None,
        }
    }

    #[test]
    fn merged_queued_turn_preserves_event_metadata_and_attachments() {
        let current = inbound(
            "C123-T1",
            "E1",
            "working",
            serde_json::json!({"source": "http"}),
        );
        let queued = vec![
            inbound(
                "C123-T1",
                "E2",
                "first follow-up",
                serde_json::json!({
                    "source": "trigger",
                    "refs": {"issue": "123"},
                    "summary_refs": {"title": "Bug"}
                }),
            ),
            inbound(
                "C123-T1",
                "E3",
                "second follow-up",
                serde_json::json!({"source": "http"}),
            ),
        ];

        let merged = merge_queued_inbound(&current, queued);

        assert_eq!(merged.session_id.as_str(), "C123-T1");
        assert_eq!(merged.user, "queued-inbound");
        assert_eq!(merged.attachments.len(), 2);
        assert!(merged.text.contains("1. Source: trigger; User: Ada (U123)"));
        assert!(merged.text.contains("first follow-up"));
        assert!(merged.text.contains("evidence.txt (text/plain, 128 bytes)"));
        assert_eq!(merged.raw["source"], "queued_batch");
        assert_eq!(merged.raw["events"][0]["raw"]["refs"]["issue"], "123");
        assert_eq!(
            merged.raw["events"][0]["raw"]["summary_refs"]["title"],
            "Bug"
        );
    }

    #[test]
    fn merged_turn_preserves_stable_session_stream_and_turn_context() {
        let current = inbound(
            "C123-T1",
            "E1",
            "working",
            serde_json::json!({
                "source": "session",
                "session_stream_id": "stream-session",
                "trace_id": "trace-1",
                "turn_id": "turn-1"
            }),
        );
        let queued = vec![
            inbound(
                "C123-T1",
                "E2",
                "follow-up",
                serde_json::json!({
                    "source": "session",
                    "session_stream_id": "stream-session"
                }),
            ),
            inbound(
                "C123-T1",
                "E3",
                "another follow-up",
                serde_json::json!({
                    "source": "session",
                    "session_stream_id": "stream-session"
                }),
            ),
        ];

        let merged = merge_queued_inbound(&current, queued);

        assert_eq!(
            session_stream_id(&merged).as_deref(),
            Some("stream-session")
        );
        assert_eq!(trace_id(&merged).as_deref(), Some("trace-1"));
        assert_eq!(turn_id(&merged).as_deref(), Some("turn-1"));
        assert!(merged.raw.get("bridged_stream_ids").is_none());
    }

    #[test]
    fn repeated_merges_keep_the_same_stable_stream() {
        let current = inbound(
            "C123-T1",
            "E1",
            "working",
            serde_json::json!({
                "source": "session",
                "session_stream_id": "stream-session",
                "trace_id": "trace-1",
                "turn_id": "turn-1"
            }),
        );
        let first = merge_queued_inbound(
            &current,
            vec![inbound(
                "C123-T1",
                "E2",
                "follow-up 1",
                serde_json::json!({"source": "session", "session_stream_id": "stream-session"}),
            )],
        );
        let second = merge_queued_inbound(
            &first,
            vec![inbound(
                "C123-T1",
                "E3",
                "follow-up 2",
                serde_json::json!({"source": "session", "session_stream_id": "stream-session"}),
            )],
        );

        assert_eq!(
            session_stream_id(&second).as_deref(),
            Some("stream-session")
        );
        assert_eq!(trace_id(&second).as_deref(), Some("trace-1"));
        assert_eq!(turn_id(&second).as_deref(), Some("turn-1"));
        assert!(second.raw.get("bridged_stream_ids").is_none());
    }

    #[test]
    fn scheduled_cron_queued_during_active_turn_runs_as_standalone_inbound() {
        let current = inbound(
            "session-1",
            "E1",
            "working",
            serde_json::json!({
                "source": "session",
                "session_stream_id": "stream-1",
                "trace_id": "trace-1",
                "turn_id": "turn-1"
            }),
        );
        let now = Utc::now();
        let cron = inbound(
            "session-1",
            "cron-1",
            "Check the background process",
            serde_json::json!({
                "source": "cron",
                "job_kind": "cron",
                "job_id": "cron-1",
                "schedule_run_key": "cron-1:2026-06-15T17:44:35Z",
                "schedule_scheduled_at": now,
                "schedule_started_at": now,
                "schedule_is_one_shot": false
            }),
        );
        let mut backlog = QueuedInboundBacklog::default();

        let next = next_queued_inbound(&current, vec![cron], &mut backlog)
            .expect("scheduled cron should be selected");

        assert_eq!(next.raw["source"], "cron");
        assert_eq!(next.raw["job_id"], "cron-1");
        assert_eq!(next.raw["schedule_run_key"], "cron-1:2026-06-15T17:44:35Z");
        assert!(next.raw.get("events").is_none());
        assert!(turn_id(&next).is_none());
        assert!(trace_id(&next).is_none());
    }

    #[test]
    fn scheduled_backlog_runs_after_one_regular_batch_even_when_more_regular_arrives() {
        let current = inbound(
            "session-1",
            "E1",
            "working",
            serde_json::json!({"source": "session"}),
        );
        let now = Utc::now();
        let cron = inbound(
            "session-1",
            "cron-1",
            "Check status",
            serde_json::json!({
                "source": "cron",
                "job_kind": "cron",
                "job_id": "cron-1",
                "schedule_run_key": "cron-1:2026-06-15T17:44:35Z",
                "schedule_scheduled_at": now,
                "schedule_started_at": now,
                "schedule_is_one_shot": false
            }),
        );
        let regular_1 = inbound(
            "session-1",
            "E2",
            "follow up one",
            serde_json::json!({"source": "session"}),
        );
        let regular_2 = inbound(
            "session-1",
            "E3",
            "follow up two",
            serde_json::json!({"source": "session"}),
        );
        let mut backlog = QueuedInboundBacklog::default();

        let first = next_queued_inbound(&current, vec![cron, regular_1], &mut backlog)
            .expect("regular batch should be selected first");
        assert_eq!(first.raw["source"], "queued_batch");

        let second = next_queued_inbound(&first, vec![regular_2], &mut backlog)
            .expect("scheduled backlog should be selected after one regular batch");
        assert_eq!(second.raw["source"], "cron");
        assert_eq!(second.raw["job_id"], "cron-1");

        let third = next_queued_inbound(&second, Vec::new(), &mut backlog)
            .expect("deferred regular should still be preserved");
        assert_eq!(third.raw["source"], "queued_batch");
        assert!(third.text.contains("follow up two"));
    }

    #[test]
    fn malformed_scheduled_metadata_is_still_classified_as_scheduled() {
        let inbound = inbound(
            "session-1",
            "cron-1",
            "Check status",
            serde_json::json!({
                "source": "cron",
                "job_kind": "cron",
                "job_id": "cron-1",
                "schedule_run_key": "cron-1:bad-timestamp",
                "schedule_scheduled_at": "not-a-timestamp",
                "schedule_started_at": Utc::now()
            }),
        );

        assert!(is_scheduled_run_inbound(&inbound));
        match ScheduledRunStatus::from_inbound(&inbound) {
            ScheduledRunStatus::Malformed(malformed) => {
                assert_eq!(malformed.job_id.as_deref(), Some("cron-1"));
                assert_eq!(malformed.reason, "invalid schedule_scheduled_at");
            }
            ScheduledRunStatus::None | ScheduledRunStatus::Valid(_) => {
                panic!("expected malformed scheduled metadata")
            }
        }
    }

    #[test]
    fn session_inbound_source_and_final_metadata_are_preserved() {
        let inbound = inbound(
            "session-1",
            "E1",
            "hello",
            serde_json::json!({
                "source": "session",
                "session_stream_id": "stream-1",
                "trace_id": "trace-1",
                "turn_id": "turn-1",
                "raw": {
                    "provider": "fake-provider",
                    "route_id": "route-1"
                }
            }),
        );
        let mut payload = serde_json::json!({
            "session_id": inbound.session_id.as_str(),
            "source": inbound_event_source(&inbound),
            "text": "done"
        });

        copy_inbound_metadata(&mut payload, &inbound);

        assert_eq!(payload["source"], "session");
        assert_eq!(payload["trace_id"], "trace-1");
        assert_eq!(payload["turn_id"], "turn-1");
        assert_eq!(payload["provider"], "fake-provider");
        assert_eq!(payload["route_id"], "route-1");
    }

    #[test]
    fn scheduled_inbound_without_subagent_kind_is_not_treated_as_subagent_task_completion() {
        let inbound = inbound(
            "C123-T1",
            "cron-1",
            "Check subagent task status",
            serde_json::json!({
                "source": "cron",
                "job_kind": "cron",
                "job_id": "cron-1",
                "parent_session_id": "C123-T1"
            }),
        );

        assert_eq!(inbound_event_source(&inbound), "cron");
        assert!(!is_subagent_task_inbound(&inbound));
    }

    #[test]
    fn subagent_task_inbound_requires_explicit_subagent_task_kind() {
        let plain_cron = inbound(
            "C123-T1",
            "cron-1",
            "Check subagent task status",
            serde_json::json!({
                "source": "cron",
                "job_id": "cron-1",
                "parent_session_id": "C123-T1"
            }),
        );
        let subagent_task = inbound(
            "subagent-task-session",
            "subagent-task-1",
            "do work",
            serde_json::json!({
                "source": "cron",
                "job_kind": "subagent_task",
                "job_id": "subagent-task-1",
                "parent_session_id": "C123-T1"
            }),
        );

        assert!(!is_subagent_task_inbound(&plain_cron));
        assert!(is_subagent_task_inbound(&subagent_task));
    }
}
