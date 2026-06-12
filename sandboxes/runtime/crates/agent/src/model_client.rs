use std::collections::HashMap;
use std::pin::Pin;
use std::time::Duration;

use async_stream::stream;
use domain::{ModelConfig, ReasoningEffort};
use futures::{Stream, StreamExt};
use reqwest::{Client, Response};
use safety::json_repair::JsonRepair;
use serde_json::Value;

use crate::primitives::{
    CacheControlPolicy, FinishReason, ModelRequest, ModelStreamEvent, ProviderUsage, ToolCall,
};
use crate::request_builder::build_openai_compatible_request;
use crate::{AgentError, Result};

pub type ModelEventStream = Pin<Box<dyn Stream<Item = Result<ModelStreamEvent>> + Send>>;

/// Maximum time to establish a TCP/TLS connection to the provider.
const CONNECT_TIMEOUT: Duration = Duration::from_secs(15);
/// Maximum time to receive response headers after the request is sent.
/// Bounds half-open connections that accept the request but never reply.
const HEADER_TIMEOUT: Duration = Duration::from_secs(60);
/// Maximum idle time between streamed body chunks before the connection is
/// considered stalled. A token stream that goes silent this long is treated as
/// a retryable transport failure rather than hanging the turn forever.
const STREAM_IDLE_TIMEOUT: Duration = Duration::from_secs(120);

/// Idle deadlines applied around the header and body phases of a streamed model
/// request. A stalled provider connection (routine during provider incidents)
/// would otherwise hang `send().await` or `bytes.next().await` forever and lock
/// the turn until the process restarts.
#[derive(Debug, Clone, Copy)]
pub struct ModelTimeouts {
    pub header: Duration,
    pub stream_idle: Duration,
}

impl Default for ModelTimeouts {
    fn default() -> Self {
        Self {
            header: HEADER_TIMEOUT,
            stream_idle: STREAM_IDLE_TIMEOUT,
        }
    }
}

/// Build the shared HTTP client with connect timeout configured. There is no
/// overall request timeout because the body is an open-ended SSE stream; idle
/// deadlines are enforced explicitly around the header and body phases instead.
fn build_http_client() -> Client {
    Client::builder()
        .connect_timeout(CONNECT_TIMEOUT)
        .build()
        .unwrap_or_else(|_| Client::new())
}

#[derive(Debug, Clone)]
pub struct ChatModelClient {
    http: Client,
    endpoints: Vec<ModelEndpoint>,
    retry_policy: ModelRetryPolicy,
    timeouts: ModelTimeouts,
    json_repair: JsonRepair,
}

impl ChatModelClient {
    pub fn new(base_url: impl Into<String>, api_key: impl Into<String>) -> Self {
        Self {
            http: build_http_client(),
            endpoints: vec![ModelEndpoint {
                base_url: base_url.into().trim_end_matches('/').to_string(),
                api_key: api_key.into(),
                model_id: None,
                extra_headers: HashMap::new(),
                cache_policy: CacheControlPolicy::Disabled,
            }],
            retry_policy: ModelRetryPolicy::default(),
            timeouts: ModelTimeouts::default(),
            json_repair: JsonRepair::new(),
        }
    }

    pub fn from_model_config(
        model: &ModelConfig,
        runtime_env: &HashMap<String, String>,
    ) -> Result<ModelClientConfig> {
        let mut endpoints = Vec::new();
        collect_model_endpoints(model, &mut endpoints, runtime_env)?;
        let Some(primary) = endpoints.first() else {
            return Err(AgentError::Model(
                "model config has no endpoints".to_string(),
            ));
        };
        let model_id = primary.model_id.clone().unwrap_or_default();
        let cache_policy = primary.cache_policy;
        Ok(ModelClientConfig {
            client: Self {
                http: build_http_client(),
                endpoints,
                retry_policy: ModelRetryPolicy::default(),
                timeouts: ModelTimeouts::default(),
                json_repair: JsonRepair::new(),
            },
            model_id,
            cache_policy,
            reasoning_effort: primary_reasoning_effort(model),
            temperature: primary_temperature(model),
            max_output_tokens: primary_max_output_tokens(model),
        })
    }

    pub fn with_retry_policy(mut self, retry_policy: ModelRetryPolicy) -> Self {
        self.retry_policy = retry_policy.normalized();
        self
    }

    pub fn with_timeouts(mut self, timeouts: ModelTimeouts) -> Self {
        self.timeouts = timeouts;
        self
    }

    pub async fn stream(&self, request: ModelRequest) -> Result<ModelEventStream> {
        let mut last_failure = None;

        for (endpoint_index, endpoint) in self.endpoints.iter().enumerate() {
            for attempt in 1..=self.retry_policy.max_attempts {
                match self.send_once(endpoint, &request).await {
                    Ok(response) => {
                        return Ok(stream_response(
                            response,
                            &self.json_repair,
                            self.timeouts.stream_idle,
                        ))
                    }
                    Err(failure) => {
                        let should_retry = failure.class.is_retryable()
                            && attempt < self.retry_policy.max_attempts;
                        let fallback_available = endpoint_index + 1 < self.endpoints.len();
                        tracing::warn!(
                            error_class = failure.class.as_str(),
                            status = failure.status,
                            attempt,
                            max_attempts = self.retry_policy.max_attempts,
                            retrying = should_retry,
                            fallback_available,
                            endpoint_index,
                            model = endpoint
                                .model_id
                                .as_deref()
                                .unwrap_or(request.model.as_str()),
                            error = %failure.message,
                            "model request failed"
                        );

                        if should_retry {
                            self.retry_policy.sleep_before_retry(attempt).await;
                            continue;
                        }

                        let should_fallback = fallback_available && failure.class.allows_fallback();
                        last_failure = Some(failure);
                        if should_fallback {
                            break;
                        }

                        return Err(last_failure
                            .expect("last failure is set before return")
                            .into_agent_error());
                    }
                }
            }
        }

        Err(last_failure
            .map(ModelRequestFailure::into_agent_error)
            .unwrap_or_else(|| AgentError::Model("model request failed".to_string())))
    }

    async fn send_once(
        &self,
        endpoint: &ModelEndpoint,
        request: &ModelRequest,
    ) -> std::result::Result<Response, ModelRequestFailure> {
        let mut endpoint_request = request.clone();
        if let Some(model_id) = &endpoint.model_id {
            endpoint_request.model = model_id.clone();
        }
        let body = build_openai_compatible_request(&endpoint_request);
        tracing::debug!(
            bytes = body.to_string().len(),
            tools = request.tools.len(),
            messages = request.messages.len(),
            model = endpoint_request.model,
            "sending model request"
        );
        let mut builder = self
            .http
            .post(format!("{}/chat/completions", endpoint.base_url))
            .bearer_auth(&endpoint.api_key)
            .json(&body);
        for (name, value) in &endpoint.extra_headers {
            builder = builder.header(name, value);
        }
        let response = match tokio::time::timeout(self.timeouts.header, builder.send()).await {
            Ok(result) => result.map_err(ModelRequestFailure::from_transport_error)?,
            Err(_) => {
                return Err(ModelRequestFailure::timeout(format!(
                    "model response headers timed out after {}s",
                    self.timeouts.header.as_secs()
                )));
            }
        };

        if !response.status().is_success() {
            let status = response.status();
            let text = response.text().await.unwrap_or_default();
            return Err(ModelRequestFailure::from_http_error(status.as_u16(), text));
        }

        Ok(response)
    }
}

#[derive(Debug, Clone)]
pub struct ModelClientConfig {
    pub client: ChatModelClient,
    pub model_id: String,
    pub cache_policy: CacheControlPolicy,
    pub reasoning_effort: Option<String>,
    pub temperature: Option<f32>,
    pub max_output_tokens: Option<u32>,
}

#[derive(Debug, Clone)]
struct ModelEndpoint {
    base_url: String,
    api_key: String,
    model_id: Option<String>,
    extra_headers: HashMap<String, String>,
    cache_policy: CacheControlPolicy,
}

#[derive(Debug, Clone)]
pub struct ModelRetryPolicy {
    pub max_attempts: usize,
    pub initial_backoff: Duration,
    pub max_backoff: Duration,
}

impl Default for ModelRetryPolicy {
    fn default() -> Self {
        Self {
            max_attempts: 3,
            initial_backoff: Duration::from_millis(200),
            max_backoff: Duration::from_secs(2),
        }
    }
}

impl ModelRetryPolicy {
    pub fn no_delay(max_attempts: usize) -> Self {
        Self {
            max_attempts: max_attempts.max(1),
            initial_backoff: Duration::ZERO,
            max_backoff: Duration::ZERO,
        }
    }

    fn normalized(mut self) -> Self {
        self.max_attempts = self.max_attempts.max(1);
        self
    }

    async fn sleep_before_retry(&self, attempt: usize) {
        let multiplier = 1_u32.checked_shl((attempt - 1).min(31) as u32).unwrap_or(1);
        let delay = self
            .initial_backoff
            .saturating_mul(multiplier)
            .min(self.max_backoff);
        if !delay.is_zero() {
            tokio::time::sleep(delay).await;
        }
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ModelErrorClass {
    Timeout,
    Transport,
    RateLimited,
    Server,
    Overloaded,
    Auth,
    Billing,
    ContextLength,
    InvalidRequest,
    Unknown,
}

impl ModelErrorClass {
    pub fn as_str(self) -> &'static str {
        match self {
            Self::Timeout => "timeout",
            Self::Transport => "transport",
            Self::RateLimited => "rate_limited",
            Self::Server => "server",
            Self::Overloaded => "overloaded",
            Self::Auth => "auth",
            Self::Billing => "billing",
            Self::ContextLength => "context_length",
            Self::InvalidRequest => "invalid_request",
            Self::Unknown => "unknown",
        }
    }

    fn is_retryable(self) -> bool {
        matches!(
            self,
            Self::Timeout | Self::Transport | Self::RateLimited | Self::Server | Self::Overloaded
        )
    }

    fn allows_fallback(self) -> bool {
        !matches!(self, Self::ContextLength | Self::InvalidRequest)
    }
}

#[derive(Debug, Clone)]
struct ModelRequestFailure {
    class: ModelErrorClass,
    status: Option<u16>,
    message: String,
}

impl ModelRequestFailure {
    fn from_transport_error(error: reqwest::Error) -> Self {
        let class = if error.is_timeout() {
            ModelErrorClass::Timeout
        } else {
            ModelErrorClass::Transport
        };
        Self {
            class,
            status: None,
            message: error.to_string(),
        }
    }

    fn timeout(message: String) -> Self {
        Self {
            class: ModelErrorClass::Timeout,
            status: None,
            message,
        }
    }

    fn from_http_error(status: u16, body: String) -> Self {
        let message = extract_model_error_message(&body);
        Self {
            class: classify_http_error(status, &message, &body),
            status: Some(status),
            message,
        }
    }

    fn into_agent_error(self) -> AgentError {
        let status = self
            .status
            .map(|status| format!("HTTP {status}: "))
            .unwrap_or_default();
        AgentError::Model(format!(
            "{}{} ({})",
            status,
            self.message,
            self.class.as_str()
        ))
    }
}

fn stream_response(
    response: Response,
    json_repair: &JsonRepair,
    idle_timeout: Duration,
) -> ModelEventStream {
    let bytes = response.bytes_stream();
    let json_repair = json_repair.clone();
    Box::pin(stream! {
        let mut buffer = String::new();
        // Raw bytes that did not form a complete UTF-8 character on the previous
        // chunk. A multi-byte character (emoji/CJK/accented) can be split across
        // network chunk boundaries, so we must carry the incomplete suffix forward
        // instead of decoding each chunk independently (which would emit U+FFFD and
        // permanently corrupt the persisted history).
        let mut utf8_carry: Vec<u8> = Vec::new();
        let mut tool_accumulator = ToolCallAccumulator::default();
        let mut last_finish_reason: Option<FinishReason> = None;
        let mut saw_event = false;
        futures::pin_mut!(bytes);
        loop {
            let next = match tokio::time::timeout(idle_timeout, bytes.next()).await {
                Ok(next) => next,
                Err(_) => {
                    let failure = ModelRequestFailure::timeout(format!(
                        "model stream stalled: no data for {}s",
                        idle_timeout.as_secs()
                    ));
                    yield Err(failure.into_agent_error());
                    return;
                }
            };
            let Some(chunk) = next else {
                break;
            };
            let chunk = match chunk {
                Ok(chunk) => chunk,
                Err(error) => {
                    let failure = ModelRequestFailure::from_transport_error(error);
                    yield Err(failure.into_agent_error());
                    return;
                }
            };
            buffer.push_str(&decode_utf8_carry(&mut utf8_carry, &chunk));
            while let Some(block) = take_sse_event(&mut buffer) {
                let Some(data) = parse_sse_data(&block) else {
                    // Comment-only event, or an event carrying no `data` field
                    // (e.g. a bare `event:`/`id:` frame). Nothing to dispatch.
                    continue;
                };
                let data = data.as_str();
                saw_event = true;
                if data == "[DONE]" {
                    let calls = tool_accumulator.finish(&json_repair);
                    if !calls.is_empty() {
                        yield Ok(ModelStreamEvent::ToolCalls(calls));
                    }
                    let reason = last_finish_reason.take().unwrap_or(FinishReason::Unknown("no_finish_reason".to_string()));
                    yield Ok(ModelStreamEvent::Done(reason));
                    return;
                }
                let Ok(value) = serde_json::from_str::<Value>(data) else {
                    continue;
                };
                if let Some(usage) = parse_usage(&value) {
                    yield Ok(ModelStreamEvent::Usage(usage));
                }
                if let Some(choices) = value.get("choices").and_then(|v| v.as_array()) {
                    for choice in choices {
                        if let Some(fr) = choice.get("finish_reason").and_then(|v| v.as_str()) {
                            if !fr.is_empty() {
                                last_finish_reason = Some(fr.parse().unwrap_or(FinishReason::Unknown(fr.to_string())));
                            }
                        }
                        let Some(delta) = choice.get("delta") else { continue };
                        if let Some(text) = delta.get("content").and_then(|v| v.as_str()) {
                            if !text.is_empty() {
                                yield Ok(ModelStreamEvent::TextDelta(text.to_string()));
                            }
                        }
                        if let Some(text) = parse_thinking_delta(delta) {
                            if !text.is_empty() {
                                yield Ok(ModelStreamEvent::ThinkingDelta(text));
                            }
                        }
                        if let Some(tool_calls) = delta.get("tool_calls").and_then(|v| v.as_array()) {
                            tool_accumulator.apply_delta(tool_calls);
                        }
                    }
                }
            }
        }
        let reason = if saw_event {
            "model stream ended without [DONE]"
        } else {
            "model stream ended without events"
        };
        yield Err(AgentError::Model(format!("{reason} (transport)")));
    })
}

fn collect_model_endpoints(
    model: &ModelConfig,
    endpoints: &mut Vec<ModelEndpoint>,
    runtime_env: &HashMap<String, String>,
) -> Result<()> {
    match model {
        ModelConfig::OpenaiCompatible {
            base_url,
            model_id,
            api_key_env,
            extra_headers,
            fallback,
            ..
        } => {
            let api_key = runtime_env
                .get(api_key_env)
                .cloned()
                .ok_or_else(|| AgentError::Model(format!("env var `{api_key_env}` not set")))?;
            endpoints.push(ModelEndpoint {
                base_url: base_url.trim_end_matches('/').to_string(),
                api_key,
                model_id: Some(model_id.clone()),
                extra_headers: extra_headers.clone(),
                cache_policy: cache_policy_for_values(base_url, api_key_env),
            });
            if let Some(fallback) = fallback {
                collect_model_endpoints(fallback, endpoints, runtime_env)?;
            }
            Ok(())
        }
    }
}

fn primary_temperature(model: &ModelConfig) -> Option<f32> {
    match model {
        ModelConfig::OpenaiCompatible { temperature, .. } => *temperature,
    }
}

fn primary_max_output_tokens(model: &ModelConfig) -> Option<u32> {
    match model {
        ModelConfig::OpenaiCompatible {
            max_output_tokens, ..
        } => *max_output_tokens,
    }
}

fn primary_reasoning_effort(model: &ModelConfig) -> Option<String> {
    match model {
        ModelConfig::OpenaiCompatible {
            reasoning_effort, ..
        } => reasoning_effort.map(reasoning_effort_to_string),
    }
}

fn reasoning_effort_to_string(effort: ReasoningEffort) -> String {
    match effort {
        ReasoningEffort::Low => "low".to_string(),
        ReasoningEffort::Medium => "medium".to_string(),
        ReasoningEffort::High => "high".to_string(),
    }
}

fn cache_policy_for_values(base_url: &str, _api_key_env: &str) -> CacheControlPolicy {
    if base_url.contains("openrouter") || base_url.contains("127.0.0.1") {
        CacheControlPolicy::OpenRouterGeminiEphemeral
    } else {
        CacheControlPolicy::Disabled
    }
}

fn classify_http_error(status: u16, message: &str, body: &str) -> ModelErrorClass {
    let haystack = format!("{message}\n{body}").to_ascii_lowercase();
    if status == 408 || haystack.contains("timeout") || haystack.contains("timed out") {
        return ModelErrorClass::Timeout;
    }
    if status == 429 || haystack.contains("rate limit") || haystack.contains("too many requests") {
        return ModelErrorClass::RateLimited;
    }
    if status == 529
        || haystack.contains("overloaded")
        || haystack.contains("capacity")
        || haystack.contains("temporarily unavailable")
    {
        return ModelErrorClass::Overloaded;
    }
    if status == 401 || status == 403 || haystack.contains("api key") {
        return ModelErrorClass::Auth;
    }
    if status == 402
        || haystack.contains("billing")
        || haystack.contains("insufficient credit")
        || haystack.contains("insufficient balance")
        || haystack.contains("payment required")
    {
        return ModelErrorClass::Billing;
    }
    if status == 413
        || haystack.contains("context length")
        || haystack.contains("context window")
        || haystack.contains("maximum context")
        || haystack.contains("too many tokens")
        || haystack.contains("token limit")
    {
        return ModelErrorClass::ContextLength;
    }
    if (500..=599).contains(&status) {
        return ModelErrorClass::Server;
    }
    if matches!(status, 400 | 404 | 422) {
        return ModelErrorClass::InvalidRequest;
    }
    ModelErrorClass::Unknown
}

fn extract_model_error_message(body: &str) -> String {
    let Ok(value) = serde_json::from_str::<Value>(body) else {
        return body.to_string();
    };
    value
        .get("error")
        .and_then(|error| {
            error
                .get("message")
                .or_else(|| error.get("code"))
                .and_then(Value::as_str)
        })
        .or_else(|| value.get("message").and_then(Value::as_str))
        .map(ToString::to_string)
        .unwrap_or_else(|| body.to_string())
}

/// Decode a network chunk as UTF-8, carrying any trailing bytes that form an
/// incomplete multi-byte character forward to the next chunk. `carry` holds the
/// leftover bytes from the previous chunk; on return it holds the new leftover.
///
/// This avoids `String::from_utf8_lossy` per chunk, which would replace a
/// character split across a chunk boundary with U+FFFD and corrupt the stream.
fn decode_utf8_carry(carry: &mut Vec<u8>, chunk: &[u8]) -> String {
    let mut bytes = std::mem::take(carry);
    bytes.extend_from_slice(chunk);
    match std::str::from_utf8(&bytes) {
        Ok(text) => text.to_string(),
        Err(error) => {
            let valid_up_to = error.valid_up_to();
            // Bytes after `valid_up_to` are either an incomplete trailing
            // multi-byte sequence (carry them) or genuinely invalid input.
            let remaining = &bytes[valid_up_to..];
            let decoded = if error.error_len().is_none() && remaining.len() < 4 {
                // Incomplete trailing sequence: decode the valid prefix and carry
                // the rest to combine with the next chunk. `valid_up_to` is on a
                // char boundary by definition, so this slice is always valid UTF-8.
                *carry = remaining.to_vec();
                std::str::from_utf8(&bytes[..valid_up_to])
                    .unwrap_or_default()
                    .to_string()
            } else {
                // Genuinely invalid bytes; fall back to lossy for this chunk so we
                // never carry undecodable garbage forward indefinitely.
                String::from_utf8_lossy(&bytes).to_string()
            };
            decoded
        }
    }
}

/// Extract the next complete SSE event block from the buffer. Per the SSE wire
/// format an event is terminated by a blank line, which may use any line ending:
/// `\n\n`, `\r\n\r\n` (CRLF — emitted by some providers and proxies), or `\r\r`.
/// Returns the raw block (without the terminating blank line) and removes it plus
/// the separator from the buffer.
fn take_sse_event(buffer: &mut String) -> Option<String> {
    let (start, len) = find_event_boundary(buffer)?;
    let event = buffer[..start].to_string();
    buffer.replace_range(..start + len, "");
    Some(event)
}

/// Find the earliest blank-line separator in `buffer`. Returns `(start, len)`
/// where `start` is the byte offset of the separator and `len` its byte length.
fn find_event_boundary(buffer: &str) -> Option<(usize, usize)> {
    let candidates = ["\r\n\r\n", "\n\n", "\r\r"];
    candidates
        .iter()
        .filter_map(|sep| buffer.find(sep).map(|idx| (idx, sep.len())))
        .min_by_key(|(idx, _)| *idx)
}

/// Parse an SSE event block per the field grammar and return the concatenated
/// `data` field value (multiple `data:` lines are joined with `\n`). Lines
/// beginning with `:` are comments and ignored; other field names (`event`,
/// `id`, `retry`, …) are ignored because the model protocol only carries JSON in
/// `data`. Lines may be separated by `\n`, `\r\n`, or `\r`. Returns `None` when
/// the block contains no `data` field at all (comment-only / metadata-only
/// frames), so callers skip it rather than treating it as empty data.
fn parse_sse_data(block: &str) -> Option<String> {
    let mut data = String::new();
    let mut saw_data = false;
    for line in block.split(['\n', '\r']) {
        if line.is_empty() || line.starts_with(':') {
            // Empty (an artifact of CR/LF splitting) or a comment line.
            continue;
        }
        let (field, value) = match line.split_once(':') {
            Some((field, value)) => (field, value.strip_prefix(' ').unwrap_or(value)),
            // A line with no colon is a field with an empty value (spec); none of
            // the fields we care about are valueless, so ignore it.
            None => continue,
        };
        if field == "data" {
            if saw_data {
                data.push('\n');
            }
            data.push_str(value);
            saw_data = true;
        }
    }
    saw_data.then_some(data)
}

fn parse_thinking_delta(delta: &Value) -> Option<String> {
    for key in ["reasoning", "reasoning_content", "thinking"] {
        if let Some(text) = delta.get(key).and_then(Value::as_str) {
            return Some(text.to_string());
        }
    }
    delta
        .get("reasoning")
        .or_else(|| delta.get("thinking"))
        .and_then(|value| value.get("content"))
        .and_then(Value::as_str)
        .map(ToString::to_string)
}

fn parse_usage(value: &Value) -> Option<ProviderUsage> {
    let usage = value.get("usage")?;
    let prompt_details = usage.get("prompt_tokens_details").unwrap_or(&Value::Null);
    let completion_details = usage
        .get("completion_tokens_details")
        .unwrap_or(&Value::Null);
    Some(ProviderUsage {
        prompt_tokens: usage
            .get("prompt_tokens")
            .and_then(|v| v.as_i64())
            .unwrap_or_default(),
        completion_tokens: usage
            .get("completion_tokens")
            .and_then(|v| v.as_i64())
            .unwrap_or_default(),
        total_tokens: usage
            .get("total_tokens")
            .and_then(|v| v.as_i64())
            .unwrap_or_default(),
        cached_tokens: prompt_details
            .get("cached_tokens")
            .and_then(|v| v.as_i64())
            .unwrap_or_default(),
        cache_write_tokens: prompt_details
            .get("cache_write_tokens")
            .and_then(|v| v.as_i64())
            .unwrap_or_default(),
        reasoning_tokens: completion_details
            .get("reasoning_tokens")
            .and_then(|v| v.as_i64())
            .unwrap_or_default(),
        cost: usage.get("cost").and_then(|v| v.as_f64()),
        raw: Some(usage.clone()),
    })
}

#[derive(Default)]
struct ToolCallAccumulator {
    calls: Vec<PartialToolCall>,
}

impl ToolCallAccumulator {
    fn apply_delta(&mut self, deltas: &[Value]) {
        for delta in deltas {
            let index = delta.get("index").and_then(|v| v.as_u64()).unwrap_or(0) as usize;
            while self.calls.len() <= index {
                self.calls.push(PartialToolCall::default());
            }
            let call = &mut self.calls[index];
            if let Some(id) = delta.get("id").and_then(|v| v.as_str()) {
                call.id = id.to_string();
            }
            if let Some(function) = delta.get("function") {
                if let Some(name) = function.get("name").and_then(|v| v.as_str()) {
                    call.name.push_str(name);
                }
                if let Some(arguments) = function.get("arguments").and_then(|v| v.as_str()) {
                    call.arguments.push_str(arguments);
                }
            }
        }
    }

    fn finish(self, repair: &JsonRepair) -> Vec<ToolCall> {
        self.calls
            .into_iter()
            .filter(|call| !call.name.is_empty())
            .map(|call| {
                let (repaired_args, was_repaired) = repair.repair(&call.arguments);
                if was_repaired {
                    tracing::debug!(
                        tool = call.name,
                        raw = call.arguments,
                        "repaired malformed JSON tool arguments"
                    );
                }
                ToolCall {
                    id: if call.id.is_empty() {
                        format!("tool_{}", call.name)
                    } else {
                        call.id
                    },
                    name: call.name,
                    arguments: repaired_args,
                }
            })
            .collect()
    }
}

#[derive(Default)]
struct PartialToolCall {
    id: String,
    name: String,
    arguments: String,
}

#[cfg(test)]
mod tests {
    use std::collections::{HashMap, VecDeque};
    use std::net::SocketAddr;
    use std::sync::Arc;
    use std::time::{Duration, UNIX_EPOCH};

    use axum::extract::State;
    use axum::http::{header, StatusCode};
    use axum::response::{IntoResponse, Response};
    use axum::routing::post;
    use axum::{Json, Router};
    use domain::ModelConfig;
    use futures::StreamExt;
    use serde_json::json;
    use tokio::io::AsyncWriteExt;
    use tokio::net::TcpListener;
    use tokio::sync::Mutex;

    use crate::primitives::{AgentMessage, CacheControlPolicy, ModelRequest, ModelStreamEvent};

    use super::{
        classify_http_error, decode_utf8_carry, parse_sse_data, parse_thinking_delta, parse_usage,
        take_sse_event, ChatModelClient, ModelErrorClass, ModelRetryPolicy, ModelTimeouts,
    };

    #[test]
    fn take_sse_event_handles_lf_crlf_and_cr_boundaries() {
        // LF-LF
        let mut buf = "data: a\n\ndata: b\n\n".to_string();
        assert_eq!(take_sse_event(&mut buf).as_deref(), Some("data: a"));
        assert_eq!(take_sse_event(&mut buf).as_deref(), Some("data: b"));
        assert!(take_sse_event(&mut buf).is_none());

        // CRLF-CRLF (providers/proxies that use Windows line endings)
        let mut buf = "data: a\r\n\r\ndata: b\r\n\r\n".to_string();
        assert_eq!(take_sse_event(&mut buf).as_deref(), Some("data: a"));
        assert_eq!(take_sse_event(&mut buf).as_deref(), Some("data: b"));

        // CR-CR (legacy Mac line endings, allowed by the SSE grammar)
        let mut buf = "data: a\r\rdata: b\r\r".to_string();
        assert_eq!(take_sse_event(&mut buf).as_deref(), Some("data: a"));
        assert_eq!(take_sse_event(&mut buf).as_deref(), Some("data: b"));
    }

    #[test]
    fn parse_sse_data_concatenates_multiple_data_fields() {
        // Per spec, multiple data: lines are joined with a newline.
        let block = "data: line one\ndata: line two";
        assert_eq!(parse_sse_data(block).as_deref(), Some("line one\nline two"));
    }

    #[test]
    fn parse_sse_data_ignores_comments_and_other_fields() {
        let block =
            ": this is a keep-alive comment\nevent: message\nid: 42\nretry: 1000\ndata: {\"x\":1}";
        assert_eq!(parse_sse_data(block).as_deref(), Some("{\"x\":1}"));
    }

    #[test]
    fn parse_sse_data_handles_crlf_within_event_and_no_space_after_colon() {
        let block = "event: message\r\ndata:{\"x\":1}";
        assert_eq!(parse_sse_data(block).as_deref(), Some("{\"x\":1}"));
    }

    #[test]
    fn parse_sse_data_returns_none_for_comment_or_metadata_only_blocks() {
        assert_eq!(parse_sse_data(": keep-alive"), None);
        assert_eq!(parse_sse_data("event: ping\nid: 7"), None);
        assert_eq!(parse_sse_data(""), None);
    }

    #[tokio::test]
    async fn stream_parses_crlf_multi_field_sse() {
        // A provider that emits CRLF line endings, a comment frame, multi-field
        // events (event:/id:/data:), and a final [DONE]. The old LF-LF +
        // bare-`data:` parser failed these entirely.
        let body = ": warmup comment\r\n\r\n\
             event: message\r\nid: 1\r\ndata: {\"choices\":[{\"delta\":{\"content\":\"Hello \"}}]}\r\n\r\n\
             event: message\r\ndata: {\"choices\":[{\"delta\":{\"content\":\"world\"},\"finish_reason\":\"stop\"}]}\r\n\r\n\
             data: [DONE]\r\n\r\n";
        let server = FakeModelServer::spawn(vec![FakeResponse::raw(body)]).await;
        let client = ChatModelClient::new(server.base_url(), "test-key");

        let events = collect_events(client.stream(test_request("m")).await.unwrap()).await;
        let text: String = events
            .into_iter()
            .filter_map(|event| match event {
                ModelStreamEvent::TextDelta(text) => Some(text),
                _ => None,
            })
            .collect();
        assert_eq!(text, "Hello world");
    }

    #[test]
    fn decode_utf8_carry_handles_multibyte_split_across_chunks() {
        // "é" is 0xC3 0xA9; "界" is 0xE7 0x95 0x8C; "🚀" is 0xF0 0x9F 0x9A 0x80.
        let full = "café世界🚀";
        let bytes = full.as_bytes();
        let mut carry = Vec::new();
        let mut decoded = String::new();
        // Feed one byte at a time — the worst case for boundary splits.
        for byte in bytes {
            decoded.push_str(&decode_utf8_carry(&mut carry, &[*byte]));
        }
        assert!(carry.is_empty(), "no bytes should be left dangling");
        assert_eq!(decoded, full);
        assert!(
            !decoded.contains('\u{FFFD}'),
            "split multi-byte chars must not be replaced with U+FFFD"
        );
    }

    #[test]
    fn decode_utf8_carry_splits_emoji_exactly_at_boundary() {
        let emoji = "🚀"; // 4 bytes
        let bytes = emoji.as_bytes();
        let mut carry = Vec::new();
        // First chunk: first 2 bytes (incomplete) — must produce nothing yet.
        let first = decode_utf8_carry(&mut carry, &bytes[..2]);
        assert_eq!(first, "");
        assert_eq!(carry.len(), 2);
        // Second chunk: remaining 2 bytes complete the character.
        let second = decode_utf8_carry(&mut carry, &bytes[2..]);
        assert_eq!(second, emoji);
        assert!(carry.is_empty());
    }

    #[test]
    fn decode_utf8_carry_passes_through_clean_ascii() {
        let mut carry = Vec::new();
        assert_eq!(decode_utf8_carry(&mut carry, b"hello"), "hello");
        assert!(carry.is_empty());
    }

    #[test]
    fn decode_utf8_carry_recovers_genuinely_invalid_bytes() {
        let mut carry = Vec::new();
        // 0xFF is never valid UTF-8 and is not an incomplete prefix.
        let decoded = decode_utf8_carry(&mut carry, &[0xFF, b'a']);
        assert!(decoded.contains('\u{FFFD}'));
        assert!(decoded.contains('a'));
        assert!(carry.is_empty());
    }

    #[test]
    fn parses_common_thinking_delta_fields() {
        assert_eq!(
            parse_thinking_delta(&json!({"reasoning": "thinking A"})),
            Some("thinking A".to_string())
        );
        assert_eq!(
            parse_thinking_delta(&json!({"reasoning_content": "thinking B"})),
            Some("thinking B".to_string())
        );
        assert_eq!(
            parse_thinking_delta(&json!({"thinking": "thinking C"})),
            Some("thinking C".to_string())
        );
        assert_eq!(
            parse_thinking_delta(&json!({"reasoning": {"content": "thinking D"}})),
            Some("thinking D".to_string())
        );
    }

    #[test]
    fn ignores_missing_thinking_delta() {
        assert_eq!(parse_thinking_delta(&json!({"content": "visible"})), None);
    }

    #[test]
    fn parses_reasoning_tokens_from_usage_details() {
        let usage = parse_usage(&json!({
            "usage": {
                "prompt_tokens": 10,
                "completion_tokens": 7,
                "total_tokens": 17,
                "prompt_tokens_details": {
                    "cached_tokens": 3,
                    "cache_write_tokens": 2
                },
                "completion_tokens_details": {
                    "reasoning_tokens": 4
                },
                "cost": 0.001
            }
        }))
        .expect("usage");

        assert_eq!(usage.prompt_tokens, 10);
        assert_eq!(usage.completion_tokens, 7);
        assert_eq!(usage.total_tokens, 17);
        assert_eq!(usage.cached_tokens, 3);
        assert_eq!(usage.cache_write_tokens, 2);
        assert_eq!(usage.reasoning_tokens, 4);
        assert_eq!(usage.cost, Some(0.001));
    }

    #[test]
    fn classifies_openrouter_failures() {
        assert_eq!(
            classify_http_error(429, "rate limited", "{}"),
            ModelErrorClass::RateLimited
        );
        assert_eq!(
            classify_http_error(503, "model is overloaded", "{}"),
            ModelErrorClass::Overloaded
        );
        assert_eq!(
            classify_http_error(401, "invalid api key", "{}"),
            ModelErrorClass::Auth
        );
        assert_eq!(
            classify_http_error(402, "insufficient credits", "{}"),
            ModelErrorClass::Billing
        );
        assert_eq!(
            classify_http_error(400, "context length exceeded", "{}"),
            ModelErrorClass::ContextLength
        );
        assert_eq!(
            classify_http_error(400, "invalid request", "{}"),
            ModelErrorClass::InvalidRequest
        );
        assert_eq!(
            classify_http_error(500, "internal error", "{}"),
            ModelErrorClass::Server
        );
    }

    #[tokio::test]
    async fn retries_rate_limited_request_then_streams_success() {
        let server = FakeModelServer::spawn(vec![
            FakeResponse::json_error(StatusCode::TOO_MANY_REQUESTS, "rate limited"),
            FakeResponse::sse("ok"),
        ])
        .await;
        let client = ChatModelClient::new(server.base_url(), "test-key")
            .with_retry_policy(ModelRetryPolicy::no_delay(2));

        let events = collect_events(client.stream(test_request("primary")).await.unwrap()).await;

        assert_eq!(server.request_count().await, 2);
        assert_eq!(
            events
                .into_iter()
                .filter_map(|event| match event {
                    ModelStreamEvent::TextDelta(text) => Some(text),
                    _ => None,
                })
                .collect::<Vec<_>>(),
            vec!["ok".to_string()]
        );
    }

    #[tokio::test]
    async fn does_not_retry_invalid_request() {
        let server = FakeModelServer::spawn(vec![
            FakeResponse::json_error(StatusCode::BAD_REQUEST, "invalid request body"),
            FakeResponse::sse("should not be used"),
        ])
        .await;
        let client = ChatModelClient::new(server.base_url(), "test-key")
            .with_retry_policy(ModelRetryPolicy::no_delay(3));

        let error = match client.stream(test_request("primary")).await {
            Ok(_) => panic!("invalid requests must fail immediately"),
            Err(error) => error,
        };

        assert_eq!(server.request_count().await, 1);
        assert!(error.to_string().contains("invalid_request"));
    }

    #[tokio::test]
    async fn falls_back_to_configured_model_after_transient_failures() {
        let server = FakeModelServer::spawn(vec![
            FakeResponse::json_error(StatusCode::SERVICE_UNAVAILABLE, "provider overloaded"),
            FakeResponse::json_error(StatusCode::SERVICE_UNAVAILABLE, "provider overloaded"),
            FakeResponse::sse("fallback ok"),
        ])
        .await;
        let primary_key = "MODEL_CLIENT_TEST_PRIMARY_KEY";
        let fallback_key = "MODEL_CLIENT_TEST_FALLBACK_KEY";
        let runtime_env = HashMap::from([
            (primary_key.to_string(), "primary-key".to_string()),
            (fallback_key.to_string(), "fallback-key".to_string()),
        ]);

        let model = ModelConfig::OpenaiCompatible {
            base_url: server.base_url(),
            model_id: "primary-model".to_string(),
            api_key_env: primary_key.to_string(),
            temperature: None,
            max_output_tokens: None,
            reasoning_effort: None,
            extra_headers: HashMap::new(),
            fallback: Some(Box::new(ModelConfig::OpenaiCompatible {
                base_url: server.base_url(),
                model_id: "fallback-model".to_string(),
                api_key_env: fallback_key.to_string(),
                temperature: None,
                max_output_tokens: None,
                reasoning_effort: None,
                extra_headers: HashMap::new(),
                fallback: None,
            })),
        };
        let config = ChatModelClient::from_model_config(&model, &runtime_env).unwrap();
        let client = config
            .client
            .with_retry_policy(ModelRetryPolicy::no_delay(2));

        let events =
            collect_events(client.stream(test_request(&config.model_id)).await.unwrap()).await;

        assert_eq!(server.request_count().await, 3);
        assert_eq!(
            server.request_models().await,
            vec![
                "primary-model".to_string(),
                "primary-model".to_string(),
                "fallback-model".to_string(),
            ]
        );
        assert!(matches!(
            config.cache_policy,
            CacheControlPolicy::OpenRouterGeminiEphemeral
        ));
        assert!(events.iter().any(|event| matches!(
            event,
            ModelStreamEvent::TextDelta(text) if text == "fallback ok"
        )));
    }

    #[test]
    fn runtime_env_overlay_wins_for_model_api_key() {
        let key_name = format!(
            "TEST_RUNTIME_MODEL_KEY_{}",
            UNIX_EPOCH.elapsed().expect("system clock").as_nanos()
        );
        std::env::set_var(&key_name, "process-key");
        let model = ModelConfig::OpenaiCompatible {
            base_url: "https://example.com".to_string(),
            model_id: "test-model".to_string(),
            api_key_env: key_name.clone(),
            temperature: None,
            max_output_tokens: None,
            reasoning_effort: None,
            extra_headers: HashMap::new(),
            fallback: None,
        };
        let mut runtime_env = HashMap::new();
        runtime_env.insert(key_name.clone(), "overlay-key".to_string());

        let config =
            ChatModelClient::from_model_config(&model, &runtime_env).expect("runtime env override");
        assert_eq!(config.client.endpoints[0].api_key, "overlay-key");
    }

    #[test]
    fn runtime_env_does_not_fall_back_to_process_for_model_api_key() {
        let key_name = format!(
            "TEST_RUNTIME_MODEL_KEY_FALLBACK_{}",
            UNIX_EPOCH.elapsed().expect("system clock").as_nanos()
        );
        std::env::set_var(&key_name, "process-only-key");
        let model = ModelConfig::OpenaiCompatible {
            base_url: "https://example.com".to_string(),
            model_id: "test-model".to_string(),
            api_key_env: key_name.clone(),
            temperature: None,
            max_output_tokens: None,
            reasoning_effort: None,
            extra_headers: HashMap::new(),
            fallback: None,
        };

        let err = ChatModelClient::from_model_config(&model, &HashMap::new())
            .expect_err("process env fallback must not be used");
        assert!(err.to_string().contains("not set"));
    }

    fn test_request(model: &str) -> ModelRequest {
        ModelRequest {
            model: model.to_string(),
            messages: vec![AgentMessage::user("hello")],
            tools: vec![],
            temperature: None,
            max_output_tokens: None,
            reasoning_effort: None,
            cache_policy: CacheControlPolicy::Disabled,
        }
    }

    async fn collect_events(mut stream: super::ModelEventStream) -> Vec<ModelStreamEvent> {
        let mut events = Vec::new();
        while let Some(event) = stream.next().await {
            events.push(event.unwrap());
        }
        events
    }

    #[tokio::test]
    async fn stream_errors_when_upstream_ends_without_done() {
        let server = FakeModelServer::spawn(vec![FakeResponse::sse_without_done("")]).await;
        let client = ChatModelClient::new(server.base_url(), "test-key");
        let mut stream = client.stream(test_request("test-model")).await.unwrap();

        let err = stream
            .next()
            .await
            .expect("stream should emit an error")
            .expect_err("truncated SSE must not complete cleanly");

        assert!(err.to_string().contains("ended without [DONE]"));
    }

    /// A server that accepts the TCP connection but never sends any response
    /// headers — a half-open connection. `send().await` must not hang forever.
    #[tokio::test]
    async fn header_phase_times_out_on_silent_server() {
        let listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
        let addr = listener.local_addr().unwrap();
        tokio::spawn(async move {
            // Accept the connection and hold it open without ever replying.
            let (_socket, _) = listener.accept().await.unwrap();
            tokio::time::sleep(Duration::from_secs(30)).await;
        });

        let client = ChatModelClient::new(format!("http://{addr}"), "test-key")
            .with_retry_policy(ModelRetryPolicy::no_delay(1))
            .with_timeouts(ModelTimeouts {
                header: Duration::from_millis(150),
                stream_idle: Duration::from_secs(5),
            });

        let result = tokio::time::timeout(
            Duration::from_secs(5),
            client.stream(test_request("test-model")),
        )
        .await
        .expect("client must not hang past the header timeout");

        let err = match result {
            Ok(_) => panic!("silent header phase must surface a failure"),
            Err(err) => err,
        };
        assert!(
            err.to_string().contains("timeout"),
            "expected a retryable timeout failure, got: {err}"
        );
    }

    /// A server that sends response headers and a partial SSE event, then stalls
    /// indefinitely without sending `[DONE]`. `bytes.next().await` must surface a
    /// retryable timeout rather than blocking the turn forever.
    #[tokio::test]
    async fn stream_phase_times_out_on_stalled_body() {
        let listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
        let addr = listener.local_addr().unwrap();
        tokio::spawn(async move {
            let (mut socket, _) = listener.accept().await.unwrap();
            // Consume the request (best-effort) then write headers + one chunk and stall.
            let mut buf = [0u8; 1024];
            let _ = tokio::io::AsyncReadExt::read(&mut socket, &mut buf).await;
            let head = "HTTP/1.1 200 OK\r\n\
                 content-type: text/event-stream\r\n\
                 transfer-encoding: chunked\r\n\r\n";
            socket.write_all(head.as_bytes()).await.unwrap();
            // One chunked body piece: a partial SSE line that never terminates.
            let chunk = "data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n";
            let framed = format!("{:x}\r\n{}\r\n", chunk.len(), chunk);
            socket.write_all(framed.as_bytes()).await.unwrap();
            socket.flush().await.unwrap();
            // Hold the connection open without sending more or closing it.
            tokio::time::sleep(Duration::from_secs(30)).await;
        });

        let client = ChatModelClient::new(format!("http://{addr}"), "test-key")
            .with_retry_policy(ModelRetryPolicy::no_delay(1))
            .with_timeouts(ModelTimeouts {
                header: Duration::from_secs(5),
                stream_idle: Duration::from_millis(150),
            });

        let mut stream = client
            .stream(test_request("test-model"))
            .await
            .expect("stream should start after headers");

        let err = tokio::time::timeout(Duration::from_secs(5), async {
            loop {
                match stream.next().await {
                    Some(Err(err)) => return err,
                    Some(Ok(_)) => continue,
                    None => panic!("stalled stream should error, not end cleanly"),
                }
            }
        })
        .await
        .expect("stalled stream must not hang past the idle timeout");

        assert!(
            err.to_string().contains("stalled") || err.to_string().contains("timeout"),
            "expected an idle-timeout failure, got: {err}"
        );
    }

    struct FakeModelServer {
        addr: SocketAddr,
        state: Arc<FakeState>,
    }

    impl FakeModelServer {
        async fn spawn(responses: Vec<FakeResponse>) -> Self {
            let state = Arc::new(FakeState {
                responses: Mutex::new(VecDeque::from(responses)),
                requests: Mutex::new(Vec::new()),
            });
            let app = Router::new()
                .route("/chat/completions", post(fake_chat_completion))
                .with_state(state.clone());
            let listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
            let addr = listener.local_addr().unwrap();
            tokio::spawn(async move {
                axum::serve(listener, app).await.unwrap();
            });
            Self { addr, state }
        }

        fn base_url(&self) -> String {
            format!("http://{}", self.addr)
        }

        async fn request_count(&self) -> usize {
            self.state.requests.lock().await.len()
        }

        async fn request_models(&self) -> Vec<String> {
            self.state
                .requests
                .lock()
                .await
                .iter()
                .filter_map(|request| request.get("model").and_then(|model| model.as_str()))
                .map(ToString::to_string)
                .collect()
        }
    }

    struct FakeState {
        responses: Mutex<VecDeque<FakeResponse>>,
        requests: Mutex<Vec<serde_json::Value>>,
    }

    #[derive(Clone)]
    struct FakeResponse {
        status: StatusCode,
        body: String,
    }

    impl FakeResponse {
        fn json_error(status: StatusCode, message: &str) -> Self {
            Self {
                status,
                body: json!({"error": {"message": message}}).to_string(),
            }
        }

        fn sse(text: &str) -> Self {
            Self {
                status: StatusCode::OK,
                body: format!(
                    "data: {{\"choices\":[{{\"delta\":{{\"content\":{}}}}}]}}\n\ndata: [DONE]\n\n",
                    serde_json::to_string(text).unwrap()
                ),
            }
        }

        fn raw(body: &str) -> Self {
            Self {
                status: StatusCode::OK,
                body: body.to_string(),
            }
        }

        fn sse_without_done(text: &str) -> Self {
            Self {
                status: StatusCode::OK,
                body: format!(
                    "data: {{\"choices\":[{{\"delta\":{{\"content\":{}}}}}]}}\n\n",
                    serde_json::to_string(text).unwrap()
                ),
            }
        }
    }

    async fn fake_chat_completion(
        State(state): State<Arc<FakeState>>,
        Json(body): Json<serde_json::Value>,
    ) -> Response {
        state.requests.lock().await.push(body);
        let response = state.responses.lock().await.pop_front().unwrap_or_else(|| {
            FakeResponse::json_error(StatusCode::INTERNAL_SERVER_ERROR, "empty")
        });
        (
            response.status,
            [(header::CONTENT_TYPE, "text/event-stream")],
            response.body,
        )
            .into_response()
    }
}
