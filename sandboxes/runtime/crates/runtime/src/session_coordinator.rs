use dashmap::DashMap;
use domain::{InboundEvent, SessionId};
use std::sync::Mutex;

pub struct SessionCoordinator {
    sessions: DashMap<SessionId, Mutex<Vec<InboundEvent>>>,
}

pub enum Submission {
    RunNow,
    Queued,
}

impl SessionCoordinator {
    pub fn new() -> Self {
        Self {
            sessions: DashMap::new(),
        }
    }

    pub fn submit_or_queue(&self, inbound: InboundEvent) -> Submission {
        let session_id = inbound.session_id.clone();
        match self.sessions.get(&session_id) {
            Some(queue) => {
                queue.lock().unwrap().push(inbound);
                Submission::Queued
            }
            None => {
                self.sessions.insert(session_id, Mutex::new(Vec::new()));
                Submission::RunNow
            }
        }
    }

    pub fn reserve(&self, session_id: &SessionId) {
        self.sessions
            .insert(session_id.clone(), Mutex::new(Vec::new()));
    }

    pub fn finish_turn(&self, session_id: &SessionId) -> Vec<InboundEvent> {
        self.sessions
            .remove(session_id)
            .map(|(_, queue)| queue.into_inner().unwrap())
            .unwrap_or_default()
    }

    pub fn drain_queued(&self, session_id: &SessionId) -> Vec<InboundEvent> {
        self.sessions
            .get(session_id)
            .map(|queue| {
                let mut queue = queue.lock().unwrap();
                std::mem::take(&mut *queue)
            })
            .unwrap_or_default()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use domain::{reply::MessageHandle, Attachment};

    fn inbound(session: &str, envelope: &str, text: &str) -> InboundEvent {
        InboundEvent {
            envelope_id: envelope.to_string(),
            session_id: SessionId::from(session.to_string()),
            user: "U123".to_string(),
            user_display_name: Some("Ada".to_string()),
            text: text.to_string(),
            attachments: vec![Attachment {
                url: "https://example.test/log.txt".to_string(),
                mime_type: "text/plain".to_string(),
                name: "log.txt".to_string(),
                size_bytes: Some(42),
            }],
            dynamic_context: Vec::new(),
            raw: serde_json::json!({"source": "test"}),
            inbound_handle: MessageHandle {
                channel: "C123".to_string(),
                ts: envelope.to_string(),
            },
            is_direct_message: false,
            is_directly_addressed: true,
            link_previews: Vec::new(),
            agent_definition: None,
        }
    }

    #[test]
    fn queues_same_session_while_turn_is_active() {
        let coordinator = SessionCoordinator::new();
        let first = inbound("C123-T1", "E1", "first");
        let second = inbound("C123-T1", "E2", "second");

        assert!(matches!(
            coordinator.submit_or_queue(first),
            Submission::RunNow
        ));
        assert!(matches!(
            coordinator.submit_or_queue(second),
            Submission::Queued
        ));

        let queued = coordinator.finish_turn(&SessionId::from("C123-T1".to_string()));
        assert_eq!(queued.len(), 1);
        assert_eq!(queued[0].text, "second");
        assert_eq!(queued[0].attachments.len(), 1);
    }

    #[test]
    fn submit_after_turn_finished_returns_run_now_and_leaves_reservation() {
        // Models the delegate-completion path: the parent turn has already
        // finished (no coordinator entry), so submit_or_queue takes a fresh
        // reservation and returns RunNow. The handler MUST react to RunNow by
        // re-dispatching the inbound (P0-27); otherwise the entry it just
        // inserted is owned by no task and the session is reserved forever.
        let coordinator = SessionCoordinator::new();
        let session = SessionId::from("parent-session".to_string());

        // First turn runs and finishes.
        assert!(matches!(
            coordinator.submit_or_queue(inbound("parent-session", "E1", "first")),
            Submission::RunNow
        ));
        assert!(coordinator.finish_turn(&session).is_empty());

        // Delegate result arrives after the parent turn already finished.
        let decision =
            coordinator.submit_or_queue(inbound("parent-session", "delegate-result", "result"));
        assert!(
            matches!(decision, Submission::RunNow),
            "a delegate result for an idle session must signal RunNow so the handler re-dispatches it"
        );

        // The fresh reservation now exists but holds no queued payload (the
        // payload was returned to the caller via RunNow). If the handler does
        // not release it, it leaks forever.
        assert!(
            coordinator.drain_queued(&session).is_empty(),
            "RunNow returns the payload to the caller; the reservation queue is empty"
        );

        // The handler releases the reservation before re-dispatching.
        let _ = coordinator.finish_turn(&session);
        assert!(matches!(
            coordinator.submit_or_queue(inbound("parent-session", "redispatch", "result")),
            Submission::RunNow
        ));
    }

    #[tokio::test]
    async fn panicking_turn_task_releases_coordinator_entry() {
        // Models the spawned turn task in main.rs: handle_inbound reserves the
        // session via submit_or_queue, then the turn body panics (P0-26: a
        // multi-byte slice panic or any other remaining panic source). The panic
        // guard around the spawned task MUST call finish_turn so future inbound
        // messages for the session are not queued forever behind a turn that will
        // never finish.
        use futures::FutureExt;
        use std::sync::Arc;

        let coordinator = Arc::new(SessionCoordinator::new());
        let session = SessionId::from("panic-session".to_string());

        let task_coordinator = coordinator.clone();
        let task_session = session.clone();
        let panic_guard_coordinator = coordinator.clone();
        let panic_guard_session = session.clone();

        let join = tokio::spawn(async move {
            let result = std::panic::AssertUnwindSafe(async move {
                // Reserve the session entry, exactly like submit_or_queue's RunNow
                // path before the turn runs.
                assert!(matches!(
                    task_coordinator.submit_or_queue(inbound(task_session.as_str(), "E1", "boom")),
                    Submission::RunNow
                ));
                // The turn body panics before reaching finish_turn.
                panic!("simulated turn panic");
            })
            .catch_unwind()
            .await;

            if result.is_err() {
                // Panic guard: release the reservation so the session is not
                // permanently blocked.
                panic_guard_coordinator.finish_turn(&panic_guard_session);
            }
        });

        // The task panicked internally but the guard caught it, so the join
        // succeeds.
        join.await.expect("panic guard must keep the task from aborting the join");

        // Without the panic guard's finish_turn, the reservation would still be
        // present and this submission would be Queued (leaked forever). With the
        // guard it is released, so a new message runs normally.
        assert!(
            matches!(
                coordinator.submit_or_queue(inbound(session.as_str(), "E2", "next")),
                Submission::RunNow
            ),
            "a panicking turn must release the coordinator entry so the next message runs"
        );
    }

    #[test]
    fn different_sessions_run_independently() {
        let coordinator = SessionCoordinator::new();

        assert!(matches!(
            coordinator.submit_or_queue(inbound("C123-T1", "E1", "first")),
            Submission::RunNow
        ));
        assert!(matches!(
            coordinator.submit_or_queue(inbound("C123-T2", "E2", "second")),
            Submission::RunNow
        ));

        assert!(coordinator
            .finish_turn(&SessionId::from("C123-T1".to_string()))
            .is_empty());
        assert!(coordinator
            .finish_turn(&SessionId::from("C123-T2".to_string()))
            .is_empty());
    }
}
