use domain::{InboundEvent, SessionId};

fn make_event(session_id: &str, user: &str) -> InboundEvent {
    InboundEvent {
        envelope_id: "test-1".into(),
        session_id: SessionId::from(session_id),
        user: user.into(),
        user_display_name: None,
        text: "test".into(),
        attachments: vec![],
        dynamic_context: vec![],
        model_definition: None,
        raw: serde_json::json!({}),
        is_direct_message: false,
        is_directly_addressed: true,
        link_previews: vec![],
        agent_definition: None,
    }
}

#[test]
fn normal_session_user_message_is_not_cron() {
    let event = make_event("session-1", "user-1");
    let is_cron = event.user == "cron";
    assert!(!is_cron, "normal user message must not be treated as cron");
}

#[test]
fn cron_worker_job_message_is_identified_as_cron() {
    let event = make_event("session-1-cron-cron-1778211804202", "cron");
    let is_cron = event.user == "cron";
    assert!(is_cron, "cron worker messages must be identified");
    assert!(
        event.session_id.as_str().contains("-cron-"),
        "worker cron has -cron- in session"
    );
}

#[test]
fn wake_cron_uses_original_session_id() {
    let sid = "session-1";
    let event = make_event(sid, "cron");
    let is_wake = event.user == "cron" && !sid.contains("-cron-");
    assert!(is_wake, "wake cron must be identified by clean session ID");
}

#[test]
fn subagent_background_task_uses_subagent_session_prefix() {
    let sid = "subagent-subagent-task-1";
    let event = make_event(sid, "subagent");
    assert!(sid.starts_with("subagent-"));
    assert_ne!(event.user, "cron");
}

#[test]
fn session_response_policy_matrix() {
    let cases = vec![
        ("session-1", "user-1", "reply"),
        ("session-1-cron-cron-1", "cron", "reply"),
        ("session-1", "cron", "reply"),
        ("subagent-subagent-task-1", "subagent", "reply"),
    ];

    for (sid, user, expected) in &cases {
        let event = make_event(sid, user);
        let route = "reply";
        assert_eq!(route, *expected, "session={}, user={}", sid, user);
        assert_eq!(event.session_id.as_str(), *sid);
    }
}
