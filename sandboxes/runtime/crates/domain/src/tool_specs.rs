use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, Serialize, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
#[serde(tag = "type", content = "config")]
pub enum ToolSpec {
    #[serde(rename = "builtin.bash")]
    Bash(BashConfig),
    #[serde(rename = "builtin.read_file")]
    ReadFile(ReadFileConfig),
    #[serde(rename = "builtin.write_file")]
    WriteFile(WriteFileConfig),
    #[serde(rename = "builtin.file_search")]
    FileSearch(SearchConfig),
    #[serde(rename = "builtin.glob")]
    Glob(SearchConfig),
    #[serde(rename = "builtin.grep")]
    Grep(SearchConfig),
    #[serde(rename = "builtin.multi_grep")]
    MultiGrep(SearchConfig),
    #[serde(rename = "builtin.apply_patch")]
    ApplyPatch(ApplyPatchConfig),
    #[serde(rename = "builtin.lsp")]
    Lsp(LspConfig),
    #[serde(rename = "builtin.subagent_task")]
    SubagentTask(#[serde(default)] SubagentTaskConfig),
    #[serde(rename = "builtin.check_bash_status")]
    CheckBashStatus,
    #[serde(rename = "builtin.skills_list")]
    SkillsList,
    #[serde(rename = "builtin.skill_view")]
    SkillView,
    #[serde(rename = "builtin.skill_manage")]
    SkillManage,
    #[serde(rename = "builtin.search_sessions")]
    SearchSessions,
    #[serde(rename = "builtin.request_user_input")]
    RequestUserInput,
    #[serde(rename = "builtin.update_plan")]
    UpdatePlan,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct BashConfig {
    pub workdir: String,
    pub timeout_seconds: u32,
    pub max_output_bytes: u64,
    #[serde(default)]
    pub deny_patterns: Vec<String>,
    #[serde(default)]
    pub env_passthrough: Vec<String>,
    #[serde(default = "default_sandbox")]
    pub sandbox: String,
}

fn default_sandbox() -> String {
    "process_isolated".into()
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct ReadFileConfig {
    pub allowed_roots: Vec<String>,
    pub max_file_size_bytes: u64,
    #[serde(default)]
    pub deny_globs: Vec<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct WriteFileConfig {
    pub allowed_roots: Vec<String>,
    pub max_file_size_bytes: u64,
    #[serde(default)]
    pub deny_globs: Vec<String>,
    #[serde(default = "default_atomic")]
    pub atomic: bool,
}

fn default_atomic() -> bool {
    true
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct SearchConfig {
    #[serde(default)]
    pub allowed_roots: Vec<String>,
    #[serde(default)]
    pub deny_globs: Vec<String>,
    #[serde(default = "default_search_max_results")]
    pub max_results: u32,
    #[serde(default = "default_search_max_output_bytes")]
    pub max_output_bytes: u64,
    #[serde(default = "default_search_timeout_seconds")]
    pub timeout_seconds: u32,
    #[serde(default)]
    pub include_hidden: bool,
    #[serde(default = "default_respect_gitignore")]
    pub respect_gitignore: bool,
    #[serde(default)]
    pub follow_symlinks: bool,
    #[serde(default = "default_content_indexing")]
    pub enable_content_indexing: bool,
}

fn default_search_max_results() -> u32 {
    100
}

fn default_search_max_output_bytes() -> u64 {
    256 * 1024
}

fn default_search_timeout_seconds() -> u32 {
    10
}

fn default_respect_gitignore() -> bool {
    true
}

fn default_content_indexing() -> bool {
    true
}

impl Default for SearchConfig {
    fn default() -> Self {
        Self {
            allowed_roots: Vec::new(),
            deny_globs: Vec::new(),
            max_results: default_search_max_results(),
            max_output_bytes: default_search_max_output_bytes(),
            timeout_seconds: default_search_timeout_seconds(),
            include_hidden: false,
            respect_gitignore: true,
            follow_symlinks: false,
            enable_content_indexing: true,
        }
    }
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct ApplyPatchConfig {
    pub allowed_roots: Vec<String>,
    pub max_file_size_bytes: u64,
    #[serde(default)]
    pub deny_globs: Vec<String>,
    #[serde(default = "default_atomic")]
    pub atomic: bool,
}

impl Default for ApplyPatchConfig {
    fn default() -> Self {
        Self {
            allowed_roots: Vec::new(),
            max_file_size_bytes: 5 * 1024 * 1024,
            deny_globs: Vec::new(),
            atomic: true,
        }
    }
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct LspConfig {
    #[serde(default)]
    pub enabled: bool,
    #[serde(default)]
    pub allowed_roots: Vec<String>,
    #[serde(default = "default_lsp_timeout_seconds")]
    pub timeout_seconds: u32,
    #[serde(default = "default_lsp_fallback_enabled")]
    pub fallback_enabled: bool,
    #[serde(default)]
    pub servers: Vec<LspServerConfig>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct LspServerConfig {
    pub id: String,
    pub command: Vec<String>,
    #[serde(default)]
    pub extensions: Vec<String>,
    #[serde(default)]
    pub root_markers: Vec<String>,
    #[serde(default)]
    pub strict_root: bool,
    #[serde(default)]
    pub disabled: bool,
    #[serde(default)]
    pub initialization_options: Option<serde_json::Value>,
}

fn default_lsp_timeout_seconds() -> u32 {
    15
}

fn default_lsp_fallback_enabled() -> bool {
    true
}

impl Default for LspConfig {
    fn default() -> Self {
        Self {
            enabled: true,
            allowed_roots: Vec::new(),
            timeout_seconds: default_lsp_timeout_seconds(),
            fallback_enabled: default_lsp_fallback_enabled(),
            servers: Vec::new(),
        }
    }
}

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct SubagentTaskConfig {
    /// Allowlist of sub-agent names. Empty = all sub-agents available.
    #[serde(default)]
    pub agents: Vec<String>,
}

pub fn default_parent_builtin_tool_specs() -> Vec<ToolSpec> {
    let mut specs = default_subagent_builtin_tool_specs();
    specs.extend([
        ToolSpec::SubagentTask(Default::default()),
        ToolSpec::RequestUserInput,
    ]);
    specs
}

pub fn default_subagent_builtin_tool_specs() -> Vec<ToolSpec> {
    vec![
        ToolSpec::Bash(BashConfig {
            workdir: ".".into(),
            timeout_seconds: 60,
            max_output_bytes: 5 * 1024 * 1024,
            deny_patterns: vec![
                "rm -rf /".into(),
                "rm -rf ~".into(),
                "mkfs".into(),
                "dd if=".into(),
                ":(){:|:&};:".into(),
                "shutdown".into(),
                "reboot".into(),
            ],
            env_passthrough: vec![
                "HOME".into(),
                "PATH".into(),
                "LANG".into(),
                "LC_ALL".into(),
                "HIVY_SANDBOX_DATA_ROOT".into(),
                "HIVY_DOCKER_DATA_ROOT".into(),
                "TMPDIR".into(),
                "TEMP".into(),
                "TMP".into(),
                "DOCKER_TMPDIR".into(),
                "HIVY_CONTROL_PLANE_URL".into(),
                "HIVY_AGENT_ID".into(),
                "HIVY_RUNTIME_SECRET".into(),
            ],
            sandbox: "process_isolated".into(),
        }),
        ToolSpec::ReadFile(ReadFileConfig {
            allowed_roots: vec![],
            max_file_size_bytes: 5 * 1024 * 1024,
            deny_globs: vec![],
        }),
        ToolSpec::WriteFile(WriteFileConfig {
            allowed_roots: vec![],
            max_file_size_bytes: 5 * 1024 * 1024,
            deny_globs: vec![],
            atomic: true,
        }),
        ToolSpec::FileSearch(SearchConfig::default()),
        ToolSpec::Glob(SearchConfig::default()),
        ToolSpec::Grep(SearchConfig::default()),
        ToolSpec::MultiGrep(SearchConfig::default()),
        ToolSpec::ApplyPatch(ApplyPatchConfig::default()),
        ToolSpec::Lsp(LspConfig::default()),
        ToolSpec::CheckBashStatus,
        ToolSpec::SearchSessions,
        ToolSpec::UpdatePlan,
        ToolSpec::SkillsList,
        ToolSpec::SkillView,
        ToolSpec::SkillManage,
    ]
}

#[cfg(test)]
mod tests {
    use super::{default_subagent_builtin_tool_specs, ToolSpec};

    #[test]
    fn default_bash_env_passthrough_keeps_storage_defaults() {
        let specs = default_subagent_builtin_tool_specs();
        let bash = specs
            .iter()
            .find_map(|spec| match spec {
                ToolSpec::Bash(config) => Some(config),
                _ => None,
            })
            .expect("default bash tool");

        for key in [
            "HIVY_SANDBOX_DATA_ROOT",
            "HIVY_DOCKER_DATA_ROOT",
            "TMPDIR",
            "TEMP",
            "TMP",
            "DOCKER_TMPDIR",
            "HIVY_CONTROL_PLANE_URL",
            "HIVY_AGENT_ID",
            "HIVY_RUNTIME_SECRET",
        ] {
            assert!(
                bash.env_passthrough.iter().any(|value| value == key),
                "missing {key} from bash env passthrough"
            );
        }
    }
}
