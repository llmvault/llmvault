mod auth;
mod browser_auth;
mod canvas_preview;
mod handlers;
mod observability_handlers;
mod plan_manager;
mod question_manager;
mod repos;
mod session_stream;
mod state;

use std::{env, net::SocketAddr};

use axum::{
    http::{
        header::{HeaderName, AUTHORIZATION, CONTENT_TYPE},
        Method,
    },
    routing::{get, post, put},
    Router,
};
use tokio::sync::oneshot;
use tower_http::cors::{Any, CorsLayer};
use tracing::{info, warn};

const RUNTIME_CORS_MODE_ENV: &str = "HIVY_RUNTIME_CORS_MODE";
const DAYTONA_SKIP_PREVIEW_WARNING_HEADER: &str = "x-daytona-skip-preview-warning";

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
enum RuntimeCorsMode {
    Runtime,
    UpstreamProxy,
}

pub use plan_manager::PlanManager;
pub use question_manager::{QuestionAnswerError, QuestionAnswerResponse, QuestionManager};
pub use repos::RepoService;
pub use session_stream::{
    SessionInterrupter, SessionMessageState, SessionStreamBroker, SessionStreamEvent,
};
pub use state::{ApiState, DrainController, DrainStatusResponse, OutboundConfigReloader};

#[cfg(feature = "openapi")]
mod openapi {
    use utoipa::OpenApi;

    #[derive(OpenApi)]
    #[openapi(
        info(title = "Hivy Sandboxes Runtime API", version = "0.0.1"),
        paths(
            crate::handlers::put_config,
            crate::handlers::post_control_commands,
            crate::handlers::post_control_drain,
            crate::handlers::get_control_drain,
            crate::handlers::post_control_drain_cancel,
            crate::handlers::get_config,
            crate::handlers::list_sessions,
            crate::handlers::get_session_detail,
            crate::handlers::healthz,
            crate::handlers::readyz,
            crate::handlers::post_session_message,
            crate::handlers::post_session_interrupt,
            crate::handlers::post_question_answer,
            crate::canvas_preview::preview_canvas_file,
            crate::handlers::get_session_live_stream,
            crate::repos::list_repos,
            crate::repos::get_repo_tree,
            crate::repos::get_repo_content,
            crate::repos::get_repo_diff,
            crate::observability_handlers::get_trace_events,
            crate::observability_handlers::get_trace_summary,
        ),
        components(schemas(
            domain::AgentDefinition,
            domain::AgentMeta,
            domain::SystemPromptConfig,
            domain::SystemPromptSegment,
            domain::StaticPromptSegment,
            domain::DynamicContextPromptSegment,
            domain::ListPromptSegment,
            domain::Limits,
            domain::ContextConfig,
            domain::CompactionConfig,
            domain::ModelConfig,
            domain::ReasoningEffort,
            domain::ToolSpec,
            domain::BashConfig,
            domain::ReadFileConfig,
            domain::WriteFileConfig,
            domain::DriveUploadConfig,
            domain::DriveDownloadConfig,
            domain::ApplyPatchConfig,
            domain::LspConfig,
            domain::LspServerConfig,
            domain::McpSpec,
            domain::ToolFilter,
            domain::OutboundEvent,
            domain::OutboundChannelSpec,
            domain::OutboundChannelKind,
            domain::WorkspaceConfig,
            domain::WorkspaceRepoConfig,
            domain::Attachment,
            domain::LinkPreview,
            domain::HistoryMessage,
            domain::SessionId,
            domain::SessionStatus,
            domain::Session,
            domain::EventKind,
            domain::SessionEvent,
            domain::SubagentTaskState,
            domain::SubagentTask,
            domain::PlanItemStatus,
            domain::PlanItem,
            domain::UpdatePlanPayload,
            domain::UpdatePlanResult,
            domain::QuestionRequestState,
            domain::QuestionOption,
            domain::RequestUserInputQuestion,
            domain::RequestUserInputPayload,
            domain::QuestionAnswerValue,
            domain::QuestionAnswerPayload,
            domain::QuestionRequest,
            domain::RequestUserInputResult,
            observability::ObservabilityEventType,
            observability::EventTimings,
            observability::ModelUsage,
            observability::ToolUsage,
            observability::ObservabilityEvent,
            observability::TraceSummary,
            crate::handlers::ConfigUpdateRequest,
            crate::handlers::ConfigResponse,
            crate::handlers::HealthResponse,
            crate::handlers::ControlCommandsRequest,
            crate::handlers::ControlCommandsResponse,
            crate::handlers::ControlCommandResult,
            crate::state::DrainStatusResponse,
            crate::handlers::ListSessionsParams,
            crate::handlers::ListSessionsResponse,
            crate::handlers::SessionDetailResponse,
            crate::handlers::SessionInterruptResponse,
            crate::session_stream::SessionStreamEvent,
            crate::session_stream::SessionMessageRequest,
            crate::session_stream::SessionMessageResponse,
            crate::question_manager::QuestionAnswerResponse,
            crate::repos::RepoInfo,
            crate::repos::RepoListResponse,
            crate::repos::TreeResponse,
            crate::repos::TreeEntry,
            crate::repos::ContentResponse,
            crate::repos::DiffResponse,
        )),
        modifiers(&SecurityAddon),
        security(("bearer" = []))
    )]
    pub struct ApiDoc;

    struct SecurityAddon;

    impl utoipa::Modify for SecurityAddon {
        fn modify(&self, openapi: &mut utoipa::openapi::OpenApi) {
            use utoipa::openapi::security::{HttpAuthScheme, HttpBuilder, SecurityScheme};

            if let Some(components) = openapi.components.as_mut() {
                components.add_security_scheme(
                    "bearer",
                    SecurityScheme::Http(
                        HttpBuilder::new()
                            .scheme(HttpAuthScheme::Bearer)
                            .bearer_format("runtime-secret")
                            .build(),
                    ),
                );
            }
        }
    }
}

#[cfg(feature = "openapi")]
pub use openapi::ApiDoc;

pub fn build_router(state: ApiState) -> Router {
    let protected = Router::new()
        .route(
            "/config",
            put(handlers::put_config).get(handlers::get_config),
        )
        .route("/control/commands", post(handlers::post_control_commands))
        .route(
            "/control/drain",
            post(handlers::post_control_drain).get(handlers::get_control_drain),
        )
        .route(
            "/control/drain/cancel",
            post(handlers::post_control_drain_cancel),
        )
        .route("/sessions", get(handlers::list_sessions))
        .route("/sessions/:session_id", get(handlers::get_session_detail))
        .route(
            "/sessions/:session_id/messages",
            post(handlers::post_session_message),
        )
        .route(
            "/sessions/:session_id/interrupt",
            post(handlers::post_session_interrupt),
        )
        .route(
            "/sessions/:session_id/questions/:question_request_id/answer",
            post(handlers::post_question_answer),
        )
        .route(
            "/sessions/:session_id/stream",
            get(handlers::get_session_live_stream),
        )
        .route(
            "/sessions/:session_id/streams/:stream_id",
            get(handlers::get_session_stream),
        )
        .route("/healthz", get(handlers::healthz))
        .route("/readyz", get(handlers::readyz))
        .route(
            "/observability/traces/:trace_id/events",
            get(observability_handlers::get_trace_events),
        )
        .route(
            "/observability/traces/:trace_id/summary",
            get(observability_handlers::get_trace_summary),
        )
        .route("/repos", get(repos::list_repos))
        .route("/repos/:repo_id/tree", get(repos::get_repo_tree))
        .route("/repos/:repo_id/content", get(repos::get_repo_content))
        .route("/repos/:repo_id/diff", get(repos::get_repo_diff))
        .layer(axum::middleware::from_fn_with_state(
            state.clone(),
            auth::bearer_auth,
        ));
    let iframe_preview = Router::new().route(
        "/canvas/preview/*path",
        get(canvas_preview::preview_canvas_file),
    );
    let router = Router::new()
        .merge(iframe_preview)
        .merge(protected)
        .with_state(state);
    apply_runtime_cors(router, runtime_cors_mode_from_env())
}

fn apply_runtime_cors(router: Router, mode: RuntimeCorsMode) -> Router {
    match mode {
        RuntimeCorsMode::Runtime => router.layer(
            CorsLayer::new()
                .allow_origin(Any)
                .allow_methods([Method::GET, Method::POST, Method::PUT, Method::OPTIONS])
                .allow_headers([
                    AUTHORIZATION,
                    CONTENT_TYPE,
                    HeaderName::from_static(DAYTONA_SKIP_PREVIEW_WARNING_HEADER),
                ]),
        ),
        RuntimeCorsMode::UpstreamProxy => router,
    }
}

fn runtime_cors_mode_from_env() -> RuntimeCorsMode {
    runtime_cors_mode(env::var(RUNTIME_CORS_MODE_ENV).ok().as_deref())
}

fn runtime_cors_mode(value: Option<&str>) -> RuntimeCorsMode {
    match value.map(str::trim) {
        Some(value) if value.eq_ignore_ascii_case("upstream_proxy") => {
            RuntimeCorsMode::UpstreamProxy
        }
        _ => RuntimeCorsMode::Runtime,
    }
}

pub async fn serve(
    bind_addr: SocketAddr,
    state: ApiState,
) -> (tokio::task::JoinHandle<()>, oneshot::Sender<()>) {
    let (cancel_signal, cancel_receiver) = oneshot::channel::<()>();
    let router = build_router(state);
    let handle = tokio::spawn(async move {
        match tokio::net::TcpListener::bind(bind_addr).await {
            Ok(listener) => {
                info!(%bind_addr, "control-plane HTTP server listening");
                let result = axum::serve(listener, router)
                    .with_graceful_shutdown(async move {
                        let _ = cancel_receiver.await;
                    })
                    .await;
                if let Err(error) = result {
                    warn!(%error, "control-plane HTTP server exited with error");
                }
            }
            Err(error) => {
                warn!(%bind_addr, %error, "control-plane HTTP bind failed");
            }
        }
    });
    (handle, cancel_signal)
}

#[cfg(test)]
mod cors_tests {
    use super::{apply_runtime_cors, runtime_cors_mode, RuntimeCorsMode};
    use axum::{
        body::Body,
        http::{Request, StatusCode},
        routing::get,
        Router,
    };
    use tower::ServiceExt;

    fn test_router(mode: RuntimeCorsMode) -> Router {
        apply_runtime_cors(
            Router::new().route("/test", get(|| async { StatusCode::OK })),
            mode,
        )
    }

    #[tokio::test]
    async fn runtime_cors_allows_daytona_warning_bypass_header() {
        let response = test_router(RuntimeCorsMode::Runtime)
            .oneshot(
                Request::builder()
                    .method("OPTIONS")
                    .uri("/test")
                    .header("origin", "https://usehivy.test")
                    .header("access-control-request-method", "GET")
                    .header(
                        "access-control-request-headers",
                        "authorization,x-daytona-skip-preview-warning",
                    )
                    .body(Body::empty())
                    .expect("build preflight request"),
            )
            .await
            .expect("serve preflight request");

        assert_eq!(response.status(), StatusCode::OK);
        assert_eq!(
            response
                .headers()
                .get("access-control-allow-origin")
                .and_then(|value| value.to_str().ok()),
            Some("*")
        );
        let allowed_headers = response
            .headers()
            .get("access-control-allow-headers")
            .and_then(|value| value.to_str().ok())
            .unwrap_or_default();
        assert!(allowed_headers.contains("x-daytona-skip-preview-warning"));
    }

    #[tokio::test]
    async fn upstream_proxy_mode_omits_runtime_cors_headers() {
        let response = test_router(RuntimeCorsMode::UpstreamProxy)
            .oneshot(
                Request::builder()
                    .uri("/test")
                    .header("origin", "https://usehivy.test")
                    .body(Body::empty())
                    .expect("build request"),
            )
            .await
            .expect("serve request");

        assert_eq!(response.status(), StatusCode::OK);
        assert!(!response
            .headers()
            .contains_key("access-control-allow-origin"));
    }

    #[test]
    fn upstream_proxy_cors_mode_is_explicit() {
        assert_eq!(
            runtime_cors_mode(Some("upstream_proxy")),
            RuntimeCorsMode::UpstreamProxy
        );
        assert_eq!(runtime_cors_mode(None), RuntimeCorsMode::Runtime);
        assert_eq!(
            runtime_cors_mode(Some("unexpected")),
            RuntimeCorsMode::Runtime
        );
    }
}

#[cfg(all(test, feature = "openapi"))]
mod openapi_tests {
    use super::ApiDoc;
    use std::collections::BTreeSet;
    use utoipa::OpenApi;

    #[test]
    fn openapi_paths_match_router_routes() {
        let spec = ApiDoc::openapi();
        let actual: BTreeSet<String> = spec.paths.paths.keys().cloned().collect();
        let expected = BTreeSet::from([
            "/config".to_string(),
            "/control/commands".to_string(),
            "/control/drain".to_string(),
            "/control/drain/cancel".to_string(),
            "/canvas/preview/{path}".to_string(),
            "/healthz".to_string(),
            "/observability/traces/{trace_id}/events".to_string(),
            "/observability/traces/{trace_id}/summary".to_string(),
            "/readyz".to_string(),
            "/repos".to_string(),
            "/repos/{repo_id}/content".to_string(),
            "/repos/{repo_id}/diff".to_string(),
            "/repos/{repo_id}/tree".to_string(),
            "/sessions".to_string(),
            "/sessions/{session_id}".to_string(),
            "/sessions/{session_id}/interrupt".to_string(),
            "/sessions/{session_id}/messages".to_string(),
            "/sessions/{session_id}/questions/{question_request_id}/answer".to_string(),
            "/sessions/{session_id}/stream".to_string(),
        ]);

        assert_eq!(actual, expected);
    }

    #[test]
    fn openapi_auth_documents_healthz_as_anonymous_only() {
        let spec = serde_json::to_value(ApiDoc::openapi()).expect("serialize OpenAPI spec");

        assert_eq!(spec["security"], serde_json::json!([{ "bearer": [] }]));
        assert_eq!(
            spec["paths"]["/healthz"]["get"]["security"],
            serde_json::json!([{}])
        );
        assert_eq!(
            spec["paths"]["/readyz"]["get"]["security"],
            serde_json::json!([{ "bearer": [] }])
        );
    }
}
