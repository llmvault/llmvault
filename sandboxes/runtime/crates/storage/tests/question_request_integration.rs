use std::collections::BTreeMap;
use std::sync::Arc;

use chrono::Utc;
use domain::{
    QuestionAnswerPayload, QuestionAnswerValue, QuestionOption, QuestionRequest,
    QuestionRequestState, RequestUserInputPayload, RequestUserInputQuestion, Session, SessionId,
    SessionStatus,
};
use storage::{init_sqlite_store, QuestionRequestRepo, SessionRepo};

async fn repos() -> (
    tempfile::TempDir,
    Arc<dyn SessionRepo>,
    Arc<dyn QuestionRequestRepo>,
) {
    let dir = tempfile::tempdir().expect("tempdir");
    let db = dir.path().join("runtime.db");
    let store = init_sqlite_store(db).await.expect("init sqlite");
    let sessions: Arc<dyn SessionRepo> = Arc::new(storage::SqliteSessionRepo::new(&store));
    let questions: Arc<dyn QuestionRequestRepo> =
        Arc::new(storage::SqliteQuestionRequestRepo::new(&store));
    (dir, sessions, questions)
}

fn request_payload() -> RequestUserInputPayload {
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
async fn question_request_lifecycle_is_first_class_storage() {
    let (_dir, sessions, questions) = repos().await;
    let now = Utc::now();
    let session_id = SessionId::from("question-session");
    sessions
        .create(&Session {
            id: session_id.clone(),
            status: SessionStatus::Active,
            created_at: now,
            last_activity_at: now,
        })
        .await
        .expect("create session");

    questions
        .create(&QuestionRequest {
            id: "question-request-1".to_string(),
            session_id: session_id.clone(),
            request: request_payload(),
            answer: None,
            state: QuestionRequestState::Pending,
            created_at: now,
            answered_at: None,
            updated_at: now,
        })
        .await
        .expect("create question");

    let fetched = questions
        .get("question-request-1")
        .await
        .expect("get question")
        .expect("question exists");
    assert_eq!(fetched.session_id, session_id);
    assert_eq!(fetched.state, QuestionRequestState::Pending);
    assert_eq!(fetched.request.questions[0].id, "choice");
    assert!(fetched.answer.is_none());

    let answered = questions
        .answer("question-request-1", &answer_payload(), Utc::now())
        .await
        .expect("answer question");
    assert!(answered);
    let duplicate = questions
        .answer("question-request-1", &answer_payload(), Utc::now())
        .await
        .expect("duplicate answer");
    assert!(!duplicate);

    let fetched = questions
        .get("question-request-1")
        .await
        .expect("get answered question")
        .expect("question exists");
    assert_eq!(fetched.state, QuestionRequestState::Answered);
    assert_eq!(
        fetched.answer.expect("answer").answers["choice"].answers[0],
        "Yes"
    );
    assert!(fetched.answered_at.is_some());
}
