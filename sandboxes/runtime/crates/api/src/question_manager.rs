use std::collections::HashMap;
use std::sync::Arc;

use agent::QuestionRequester;
use async_trait::async_trait;
use chrono::Utc;
use domain::{
    event_types, validate_question_answer, validate_request_user_input_payload, OutboundEvent,
    QuestionAnswerPayload, QuestionRequest, QuestionRequestState, RequestUserInputPayload,
    SessionId,
};
use nanoid::nanoid;
use outbound::OutboundEmitter;
use serde::{Deserialize, Serialize};
use storage::QuestionRequestRepo;
use tokio::sync::{oneshot, Mutex};

use crate::session_stream::SessionStreamBroker;

#[derive(Debug, thiserror::Error)]
pub enum QuestionAnswerError {
    #[error("question request not found")]
    NotFound,
    #[error("question request has already been answered")]
    AlreadyAnswered,
    #[error("question request cannot be resumed by this runtime")]
    NotWaiting,
    #[error("invalid question answer: {0}")]
    InvalidAnswer(String),
    #[error(transparent)]
    Storage(#[from] storage::StorageError),
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct QuestionAnswerResponse {
    pub session_id: String,
    pub question_request_id: String,
    pub state: String,
}

type PendingSender = oneshot::Sender<QuestionAnswerPayload>;

pub struct QuestionManager {
    repo: Arc<dyn QuestionRequestRepo>,
    broker: Arc<SessionStreamBroker>,
    outbound: Option<Arc<OutboundEmitter>>,
    pending: Mutex<HashMap<String, PendingSender>>,
}

impl QuestionManager {
    pub fn new(repo: Arc<dyn QuestionRequestRepo>, broker: Arc<SessionStreamBroker>) -> Self {
        Self {
            repo,
            broker,
            outbound: None,
            pending: Mutex::new(HashMap::new()),
        }
    }

    pub fn with_outbound_emitter(mut self, outbound: Arc<OutboundEmitter>) -> Self {
        self.outbound = Some(outbound);
        self
    }

    pub async fn answer(
        &self,
        session_id: &SessionId,
        question_request_id: &str,
        answer: QuestionAnswerPayload,
    ) -> Result<QuestionAnswerResponse, QuestionAnswerError> {
        let stored = self
            .repo
            .get(question_request_id)
            .await?
            .filter(|request| request.session_id == *session_id)
            .ok_or(QuestionAnswerError::NotFound)?;
        if matches!(stored.state, QuestionRequestState::Answered) {
            return Err(QuestionAnswerError::AlreadyAnswered);
        }
        validate_question_answer(&stored.request, &answer)
            .map_err(QuestionAnswerError::InvalidAnswer)?;
        let sender = {
            let mut pending = self.pending.lock().await;
            pending
                .remove(question_request_id)
                .ok_or(QuestionAnswerError::NotWaiting)?
        };
        let answered = self
            .repo
            .answer(question_request_id, &answer, Utc::now())
            .await?;
        if !answered {
            return Err(QuestionAnswerError::AlreadyAnswered);
        }
        self.publish_answered(session_id, question_request_id, &answer)
            .await;
        let _ = sender.send(answer);
        Ok(QuestionAnswerResponse {
            session_id: session_id.as_str().to_string(),
            question_request_id: question_request_id.to_string(),
            state: "answered".to_string(),
        })
    }

    async fn publish_requested(
        &self,
        session_id: &SessionId,
        question_request_id: &str,
        request: &RequestUserInputPayload,
    ) {
        if let Some(stream_id) = self.broker.stream_id_for_session(session_id.as_str()).await {
            let payload = serde_json::json!({
                "session_id": session_id.as_str(),
                "stream_id": &stream_id,
                "question_request_id": question_request_id,
                "questions": &request.questions,
            });
            self.broker
                .publish(&stream_id, "question_requested", payload.clone())
                .await;
            self.emit_outbound(event_types::QUESTION_REQUESTED, payload)
                .await;
        }
    }

    async fn publish_answered(
        &self,
        session_id: &SessionId,
        question_request_id: &str,
        answer: &QuestionAnswerPayload,
    ) {
        if let Some(stream_id) = self.broker.stream_id_for_session(session_id.as_str()).await {
            let payload = serde_json::json!({
                "session_id": session_id.as_str(),
                "stream_id": &stream_id,
                "question_request_id": question_request_id,
                "answers": &answer.answers,
                "user": &answer.user,
                "user_display_name": &answer.user_display_name,
            });
            self.broker
                .publish(&stream_id, "question_answered", payload.clone())
                .await;
            self.emit_outbound(event_types::QUESTION_ANSWERED, payload)
                .await;
        }
    }

    async fn emit_outbound(&self, event_type: &str, mut payload: serde_json::Value) {
        let Some(outbound) = self.outbound.as_ref() else {
            return;
        };
        if let Some(map) = payload.as_object_mut() {
            map.insert("event_name".to_string(), serde_json::json!(event_type));
            map.entry("event_id".to_string())
                .or_insert_with(|| serde_json::json!(format!("evt_rt_{}", nanoid!(16))));
            map.entry("sequence".to_string())
                .or_insert_with(|| serde_json::json!(0));
            map.entry("occurred_at".to_string())
                .or_insert_with(|| serde_json::json!(Utc::now()));
            map.entry("scope".to_string())
                .or_insert_with(|| serde_json::json!("main"));
        }
        outbound.emit(OutboundEvent::new(event_type, payload)).await;
    }
}

#[async_trait]
impl QuestionRequester for QuestionManager {
    async fn request_user_input(
        &self,
        session_id: &SessionId,
        request: RequestUserInputPayload,
    ) -> anyhow::Result<(String, QuestionAnswerPayload)> {
        validate_request_user_input_payload(&request)
            .map_err(|error| anyhow::anyhow!("invalid request_user_input arguments: {error}"))?;
        let now = Utc::now();
        let id = format!("question-request-{}", nanoid!(12));
        self.repo
            .create(&QuestionRequest {
                id: id.clone(),
                session_id: session_id.clone(),
                request: request.clone(),
                answer: None,
                state: QuestionRequestState::Pending,
                created_at: now,
                answered_at: None,
                updated_at: now,
            })
            .await?;
        let (tx, rx) = oneshot::channel();
        {
            let mut pending = self.pending.lock().await;
            pending.insert(id.clone(), tx);
        }
        self.publish_requested(session_id, &id, &request).await;
        let answer = rx.await?;
        Ok((id, answer))
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::collections::{BTreeMap, HashMap};

    use async_trait::async_trait;
    use domain::{QuestionAnswerValue, QuestionOption, RequestUserInputQuestion};
    use storage::Result as StorageResult;

    #[derive(Default)]
    struct MemoryQuestionRepo {
        rows: Mutex<HashMap<String, QuestionRequest>>,
    }

    #[async_trait]
    impl QuestionRequestRepo for MemoryQuestionRepo {
        async fn create(&self, request: &QuestionRequest) -> StorageResult<()> {
            self.rows
                .lock()
                .await
                .insert(request.id.clone(), request.clone());
            Ok(())
        }

        async fn get(&self, id: &str) -> StorageResult<Option<QuestionRequest>> {
            Ok(self.rows.lock().await.get(id).cloned())
        }

        async fn answer(
            &self,
            id: &str,
            answer: &QuestionAnswerPayload,
            answered_at: chrono::DateTime<Utc>,
        ) -> StorageResult<bool> {
            let mut rows = self.rows.lock().await;
            let Some(row) = rows.get_mut(id) else {
                return Ok(false);
            };
            if row.state != QuestionRequestState::Pending {
                return Ok(false);
            }
            row.state = QuestionRequestState::Answered;
            row.answer = Some(answer.clone());
            row.answered_at = Some(answered_at);
            row.updated_at = answered_at;
            Ok(true)
        }
    }

    fn question_payload() -> RequestUserInputPayload {
        RequestUserInputPayload {
            questions: vec![RequestUserInputQuestion {
                id: "choice".to_string(),
                header: "Choice".to_string(),
                question: "Pick one.".to_string(),
                options: vec![
                    QuestionOption {
                        label: "Yes".to_string(),
                        description: "Proceed.".to_string(),
                    },
                    QuestionOption {
                        label: "No".to_string(),
                        description: "Stop.".to_string(),
                    },
                ],
            }],
        }
    }

    fn answer_payload() -> QuestionAnswerPayload {
        QuestionAnswerPayload {
            answers: BTreeMap::from([(
                "choice".to_string(),
                QuestionAnswerValue {
                    answers: vec!["Yes".to_string()],
                    other: None,
                },
            )]),
            user: Some("user-1".to_string()),
            user_display_name: Some("Ada".to_string()),
        }
    }

    #[tokio::test]
    async fn question_request_publishes_and_resolves_with_answer() {
        let repo = Arc::new(MemoryQuestionRepo::default());
        let broker = Arc::new(SessionStreamBroker::new());
        let manager = Arc::new(QuestionManager::new(repo.clone(), broker.clone()));
        let session_id = SessionId::from("question-session");
        let stream_id = broker.create_stream().await;
        broker
            .register_session(session_id.as_str(), &stream_id)
            .await;
        let (_history, mut stream) = broker.subscribe(&stream_id).await.expect("stream");

        let manager_for_task = manager.clone();
        let session_for_task = session_id.clone();
        let request_task = tokio::spawn(async move {
            manager_for_task
                .request_user_input(&session_for_task, question_payload())
                .await
                .expect("request user input")
        });

        let requested = tokio::time::timeout(std::time::Duration::from_secs(1), stream.recv())
            .await
            .expect("question event")
            .expect("stream event");
        assert_eq!(requested.event, "question_requested");
        let question_id = requested
            .payload
            .get("question_request_id")
            .and_then(serde_json::Value::as_str)
            .expect("question id")
            .to_string();
        assert!(repo.get(&question_id).await.expect("stored").is_some());

        let response = manager
            .answer(&session_id, &question_id, answer_payload())
            .await
            .expect("answer");
        assert_eq!(response.state, "answered");

        let answered_event = tokio::time::timeout(std::time::Duration::from_secs(1), stream.recv())
            .await
            .expect("answered event")
            .expect("stream event");
        assert_eq!(answered_event.event, "question_answered");

        let (returned_id, answer) = request_task.await.expect("join");
        assert_eq!(returned_id, question_id);
        assert_eq!(answer.answers["choice"].answers[0], "Yes");
    }

    #[tokio::test]
    async fn answering_without_live_waiter_is_conflict() {
        let repo = Arc::new(MemoryQuestionRepo::default());
        let broker = Arc::new(SessionStreamBroker::new());
        let manager = QuestionManager::new(repo.clone(), broker);
        let session_id = SessionId::from("question-session");
        let now = Utc::now();
        repo.create(&QuestionRequest {
            id: "question-request-orphaned".to_string(),
            session_id: session_id.clone(),
            request: question_payload(),
            answer: None,
            state: QuestionRequestState::Pending,
            created_at: now,
            answered_at: None,
            updated_at: now,
        })
        .await
        .expect("create question");

        let error = manager
            .answer(&session_id, "question-request-orphaned", answer_payload())
            .await
            .expect_err("orphaned question must fail");
        assert!(matches!(error, QuestionAnswerError::NotWaiting));
    }
}
