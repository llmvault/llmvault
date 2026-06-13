use std::collections::{BTreeMap, BTreeSet};

use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};

use crate::SessionId;

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
#[serde(rename_all = "lowercase")]
pub enum QuestionRequestState {
    Pending,
    Answered,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct QuestionOption {
    pub label: String,
    pub description: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct RequestUserInputQuestion {
    pub id: String,
    pub header: String,
    pub question: String,
    pub options: Vec<QuestionOption>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct RequestUserInputPayload {
    pub questions: Vec<RequestUserInputQuestion>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct QuestionAnswerValue {
    pub answers: Vec<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub other: Option<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct QuestionAnswerPayload {
    pub answers: BTreeMap<String, QuestionAnswerValue>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub user: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub user_display_name: Option<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct QuestionRequest {
    pub id: String,
    pub session_id: SessionId,
    pub request: RequestUserInputPayload,
    pub answer: Option<QuestionAnswerPayload>,
    pub state: QuestionRequestState,
    pub created_at: DateTime<Utc>,
    pub answered_at: Option<DateTime<Utc>>,
    pub updated_at: DateTime<Utc>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct RequestUserInputResult {
    pub question_request_id: String,
    pub answers: BTreeMap<String, QuestionAnswerValue>,
}

pub fn validate_request_user_input_payload(
    payload: &RequestUserInputPayload,
) -> Result<(), String> {
    if payload.questions.is_empty() || payload.questions.len() > 3 {
        return Err("questions must contain 1 to 3 items".to_string());
    }
    let mut ids = BTreeSet::new();
    for question in &payload.questions {
        let id = question.id.trim();
        if id.is_empty() {
            return Err("question id is required".to_string());
        }
        if !ids.insert(id.to_string()) {
            return Err(format!("duplicate question id: {id}"));
        }
        if question.header.trim().is_empty() {
            return Err(format!("question {id} header is required"));
        }
        if question.header.chars().count() > 12 {
            return Err(format!(
                "question {id} header must be 12 characters or fewer"
            ));
        }
        if question.question.trim().is_empty() {
            return Err(format!("question {id} prompt is required"));
        }
        if question.options.len() < 2 || question.options.len() > 3 {
            return Err(format!("question {id} must have 2 to 3 options"));
        }
        let mut labels = BTreeSet::new();
        for option in &question.options {
            let label = option.label.trim();
            if label.is_empty() {
                return Err(format!("question {id} option label is required"));
            }
            if label == "Other" {
                return Err(format!(
                    "question {id} must not include the automatic Other option"
                ));
            }
            if !labels.insert(label.to_string()) {
                return Err(format!("question {id} has duplicate option label: {label}"));
            }
            if option.description.trim().is_empty() {
                return Err(format!("question {id} option description is required"));
            }
        }
    }
    Ok(())
}

pub fn validate_question_answer(
    request: &RequestUserInputPayload,
    answer: &QuestionAnswerPayload,
) -> Result<(), String> {
    validate_request_user_input_payload(request)?;
    if answer.answers.len() != request.questions.len() {
        return Err("answers must include exactly one entry per question".to_string());
    }
    let expected_ids: BTreeSet<&str> = request.questions.iter().map(|q| q.id.as_str()).collect();
    for key in answer.answers.keys() {
        if !expected_ids.contains(key.as_str()) {
            return Err(format!("answer contains unknown question id: {key}"));
        }
    }
    for question in &request.questions {
        let Some(value) = answer.answers.get(&question.id) else {
            return Err(format!("answer missing question id: {}", question.id));
        };
        if value.answers.len() != 1 {
            return Err(format!(
                "question {} answer must contain one choice",
                question.id
            ));
        }
        let selected = value.answers[0].trim();
        if selected == "Other" {
            if value
                .other
                .as_deref()
                .is_none_or(|other| other.trim().is_empty())
            {
                return Err(format!(
                    "question {} Other answer requires free-form text",
                    question.id
                ));
            }
            continue;
        }
        if value.other.is_some() {
            return Err(format!(
                "question {} non-Other answer must not include other text",
                question.id
            ));
        }
        let valid = question
            .options
            .iter()
            .any(|option| option.label.trim() == selected);
        if !valid {
            return Err(format!(
                "question {} selected unknown option: {selected}",
                question.id
            ));
        }
    }
    Ok(())
}
