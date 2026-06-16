mod git;
mod service;
mod types;

use axum::{
    extract::{Path, Query, State},
    http::StatusCode,
    Json,
};
use serde::Deserialize;

use crate::state::ApiState;

pub use service::RepoService;
pub use types::{ContentResponse, DiffResponse, RepoListResponse, TreeResponse};
#[cfg(feature = "openapi")]
pub use types::{RepoInfo, TreeEntry};

#[derive(Debug, Default, Deserialize)]
pub struct RepoPathQuery {
    #[serde(default)]
    path: Option<String>,
    #[serde(default)]
    offset: Option<usize>,
    #[serde(default)]
    limit: Option<usize>,
    #[serde(default)]
    context: Option<u32>,
}

#[cfg_attr(feature = "openapi", utoipa::path(
    get,
    path = "/repos",
    responses((status = 200, description = "Tracked repositories", body = RepoListResponse))
))]
pub async fn list_repos(
    State(state): State<ApiState>,
) -> Result<Json<RepoListResponse>, (StatusCode, String)> {
    let Some(repo_service) = state.repo_service.as_ref() else {
        return Err((
            StatusCode::SERVICE_UNAVAILABLE,
            "repo API is not enabled".to_string(),
        ));
    };
    repo_service
        .list_repos()
        .await
        .map(Json)
        .map_err(repo_error_response)
}

#[cfg_attr(feature = "openapi", utoipa::path(
    get,
    path = "/repos/{repo_id}/tree",
    params(("repo_id" = String, Path, description = "Repository ID")),
    responses((status = 200, description = "Repository tree entries", body = TreeResponse))
))]
pub async fn get_repo_tree(
    State(state): State<ApiState>,
    Path(repo_id): Path<String>,
    Query(query): Query<RepoPathQuery>,
) -> Result<Json<TreeResponse>, (StatusCode, String)> {
    let Some(repo_service) = state.repo_service.as_ref() else {
        return Err((
            StatusCode::SERVICE_UNAVAILABLE,
            "repo API is not enabled".to_string(),
        ));
    };
    repo_service
        .tree(&repo_id, query.path.as_deref().unwrap_or(""))
        .await
        .map(Json)
        .map_err(repo_error_response)
}

#[cfg_attr(feature = "openapi", utoipa::path(
    get,
    path = "/repos/{repo_id}/content",
    params(("repo_id" = String, Path, description = "Repository ID")),
    responses((status = 200, description = "Repository file content", body = ContentResponse))
))]
pub async fn get_repo_content(
    State(state): State<ApiState>,
    Path(repo_id): Path<String>,
    Query(query): Query<RepoPathQuery>,
) -> Result<Json<ContentResponse>, (StatusCode, String)> {
    let Some(repo_service) = state.repo_service.as_ref() else {
        return Err((
            StatusCode::SERVICE_UNAVAILABLE,
            "repo API is not enabled".to_string(),
        ));
    };
    repo_service
        .content(
            &repo_id,
            query.path.as_deref().unwrap_or(""),
            query.offset,
            query.limit,
        )
        .await
        .map(Json)
        .map_err(repo_error_response)
}

#[cfg_attr(feature = "openapi", utoipa::path(
    get,
    path = "/repos/{repo_id}/diff",
    params(("repo_id" = String, Path, description = "Repository ID")),
    responses((status = 200, description = "Repository unified diff", body = DiffResponse))
))]
pub async fn get_repo_diff(
    State(state): State<ApiState>,
    Path(repo_id): Path<String>,
    Query(query): Query<RepoPathQuery>,
) -> Result<Json<DiffResponse>, (StatusCode, String)> {
    let Some(repo_service) = state.repo_service.as_ref() else {
        return Err((
            StatusCode::SERVICE_UNAVAILABLE,
            "repo API is not enabled".to_string(),
        ));
    };
    repo_service
        .diff(&repo_id, query.path.as_deref(), query.context)
        .await
        .map(Json)
        .map_err(repo_error_response)
}

fn repo_error_response(error: anyhow::Error) -> (StatusCode, String) {
    let message = error.to_string();
    let status = if message.contains("not found") {
        StatusCode::NOT_FOUND
    } else if message.contains("invalid")
        || message.contains("escapes")
        || message.contains("too large")
        || message.contains("not a")
        || message.contains("outside")
    {
        StatusCode::BAD_REQUEST
    } else {
        StatusCode::INTERNAL_SERVER_ERROR
    };
    (status, message)
}
