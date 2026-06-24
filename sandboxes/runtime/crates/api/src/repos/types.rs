use serde::Serialize;

#[derive(Debug, Clone, Serialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct RepoInfo {
    pub id: String,
    pub name: String,
    pub relative_path: String,
    pub head_sha: String,
    pub base_sha: String,
}

#[derive(Debug, Serialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct RepoListResponse {
    pub repos: Vec<RepoInfo>,
}

#[derive(Debug, Serialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct TreeResponse {
    pub repo_id: String,
    pub path: String,
    pub entries: Vec<TreeEntry>,
}

#[derive(Debug, Serialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct TreeEntry {
    pub name: String,
    pub path: String,
    #[serde(rename = "type")]
    pub kind: String,
    pub size: Option<u64>,
    pub git_status: Option<String>,
}

#[derive(Debug, Serialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct ContentResponse {
    pub repo_id: String,
    pub path: String,
    pub content: String,
    pub encoding: String,
    pub truncated: bool,
    pub total_lines: usize,
    pub total_bytes: usize,
    pub shown_lines: usize,
    pub offset: Option<usize>,
    pub limit: Option<usize>,
}

#[derive(Debug, Serialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct DiffFileSummary {
    pub path: String,
    pub status: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub previous_path: Option<String>,
}

#[derive(Debug, Serialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct DiffResponse {
    pub repo_id: String,
    pub path: Option<String>,
    pub diff: String,
    pub truncated: bool,
    pub total_bytes: usize,
    pub max_bytes: usize,
    pub files: Vec<DiffFileSummary>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub message: Option<String>,
}
