use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, Default, PartialEq, Eq, Serialize, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct WorkspaceConfig {
    #[serde(default)]
    pub repos: Vec<WorkspaceRepoConfig>,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct WorkspaceRepoConfig {
    pub id: String,
    pub name: String,
    pub full_name: String,
    pub clone_url: String,
    #[serde(default)]
    pub depth: Option<u32>,
}
