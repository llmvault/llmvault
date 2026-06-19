use std::collections::hash_map::DefaultHasher;
use std::hash::{Hash, Hasher};
use std::path::{Path, PathBuf};
use std::sync::{Arc, OnceLock};
use std::time::Duration;

use anyhow::{anyhow, Result};
use async_trait::async_trait;
use domain::SearchConfig;
use fff_search::{
    AiGrepConfig, Constraint, FFFMode, FFFQuery, FilePicker, FilePickerOptions, FileSearchConfig,
    FrecencyTracker, FuzzySearchOptions, GrepMode, GrepSearchOptions, PaginationArgs, QueryTracker,
    SharedFilePicker, SharedFrecency, SharedQueryTracker,
};
use globset::GlobSet;
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};
use serde_json::{json, Value};

use crate::path::{
    build_glob_set, canonicalize_best_effort, enforce_deny_globs, resolve_within_workspace,
    PathPolicyError,
};
use crate::{schema_for, JsonTool, ToolDefinition};

const FILE_SEARCH_NAME: &str = "file_search";
const GLOB_NAME: &str = "glob";
const GREP_NAME: &str = "grep";
const MULTI_GREP_NAME: &str = "multi_grep";

const FILE_SEARCH_DESCRIPTION: &str =
    "Fuzzy search for file paths in the workspace using FFF. Use this when you \
     know part of a filename or path but not the exact location. It matches \
     whole repo-relative paths with typo tolerance and frecency ranking. Use \
     grep or multi_grep for file contents.";

const GLOB_DESCRIPTION: &str =
    "Find files by an exact glob pattern using FFF's indexed file set. Use this \
     for patterns like **/*.rs or packages/*/Cargo.toml. Use file_search when \
     the path is uncertain and grep/multi_grep for contents.";

const GREP_DESCRIPTION: &str =
    "Search file contents with FFF. Use grep for one pattern. It supports plain, \
     regex, and fuzzy modes, optional path/include/exclude filters, context \
     lines, and cursor pagination. Use multi_grep when searching for several \
     symbols or strings at once. Do not use bash for source search.";

const MULTI_GREP_DESCRIPTION: &str =
    "Search file contents for any of several patterns in one FFF pass. Use this \
     for migrations, multiple symbols, TODO/error variants, or cross-language \
     searches. Results include the matching pattern. Do not pass wildcard-only \
     patterns.";

#[derive(Clone)]
pub struct SearchService {
    workspace_root: PathBuf,
    state: Arc<OnceLock<std::result::Result<Arc<SearchState>, String>>>,
}

struct SearchState {
    workspace_root: PathBuf,
    shared_picker: SharedFilePicker,
    shared_frecency: SharedFrecency,
    shared_query_tracker: SharedQueryTracker,
    init_warning: Option<String>,
}

#[derive(Debug, Deserialize, Serialize, JsonSchema)]
pub struct FileSearchArgs {
    /// Fuzzy filename or path query.
    pub query: String,
    /// Optional workspace-relative directory/file scope.
    #[serde(default)]
    pub path: Option<String>,
    /// Maximum number of results to return.
    #[serde(default)]
    pub limit: Option<usize>,
    /// Cursor returned by a previous file_search response.
    #[serde(default)]
    pub cursor: Option<String>,
}

#[derive(Debug, Deserialize, Serialize, JsonSchema)]
pub struct GlobArgs {
    /// Glob pattern, for example **/*.rs or packages/*/Cargo.toml.
    pub pattern: String,
    /// Optional workspace-relative directory scope.
    #[serde(default)]
    pub path: Option<String>,
    /// Maximum number of results to return.
    #[serde(default)]
    pub limit: Option<usize>,
    /// Cursor returned by a previous glob response.
    #[serde(default)]
    pub cursor: Option<String>,
}

#[derive(Debug, Deserialize, Serialize, JsonSchema)]
#[serde(rename_all = "snake_case")]
pub enum GrepSearchMode {
    Plain,
    Regex,
    Fuzzy,
    Auto,
}

#[derive(Debug, Deserialize, Serialize, JsonSchema)]
pub struct GrepArgs {
    /// Pattern to search for in file contents.
    pub pattern: String,
    /// Optional workspace-relative file or directory scope.
    #[serde(default)]
    pub path: Option<String>,
    /// Optional glob include filter, for example **/*.rs.
    #[serde(default)]
    pub include: Option<String>,
    /// Optional glob exclude filter, for example **/target/**.
    #[serde(default)]
    pub exclude: Option<String>,
    /// Search interpretation. auto defaults to regex when the pattern has regex metacharacters.
    #[serde(default)]
    pub mode: Option<GrepSearchMode>,
    /// Force case-sensitive search. When omitted, FFF smart-case is used.
    #[serde(default)]
    pub case_sensitive: Option<bool>,
    /// Number of context lines before and after each match.
    #[serde(default)]
    pub context: Option<usize>,
    /// Maximum number of matches to return.
    #[serde(default)]
    pub limit: Option<usize>,
    /// Cursor returned by a previous grep response.
    #[serde(default)]
    pub cursor: Option<String>,
}

#[derive(Debug, Deserialize, Serialize, JsonSchema)]
pub struct MultiGrepArgs {
    /// Patterns to search for. Matches use OR semantics.
    pub patterns: Vec<String>,
    /// Optional workspace-relative file or directory scope.
    #[serde(default)]
    pub path: Option<String>,
    /// Optional glob include filter, for example **/*.{go,rs}.
    #[serde(default)]
    pub include: Option<String>,
    /// Optional glob exclude filter, for example **/node_modules/**.
    #[serde(default)]
    pub exclude: Option<String>,
    /// Search interpretation for all patterns.
    #[serde(default)]
    pub mode: Option<GrepSearchMode>,
    /// Force case-sensitive search. When omitted, FFF smart-case is used.
    #[serde(default)]
    pub case_sensitive: Option<bool>,
    /// Number of context lines before and after each match.
    #[serde(default)]
    pub context: Option<usize>,
    /// Maximum number of matches to return.
    #[serde(default)]
    pub limit: Option<usize>,
    /// Cursor returned by a previous multi_grep response.
    #[serde(default)]
    pub cursor: Option<String>,
}

pub struct FileSearchTool {
    config: SearchConfig,
    service: SearchService,
}

pub struct GlobTool {
    config: SearchConfig,
    service: SearchService,
}

pub struct GrepTool {
    config: SearchConfig,
    service: SearchService,
}

pub struct MultiGrepTool {
    config: SearchConfig,
    service: SearchService,
}

impl SearchService {
    pub fn new(workspace_root: PathBuf) -> Self {
        Self {
            workspace_root,
            state: Arc::new(OnceLock::new()),
        }
    }

    fn state(&self, config: &SearchConfig) -> Result<Arc<SearchState>> {
        let state = self.state.get_or_init(|| {
            SearchState::new(self.workspace_root.clone(), config)
                .map(Arc::new)
                .map_err(|error| error.to_string())
        });
        match state {
            Ok(state) => Ok(state.clone()),
            Err(error) => Err(anyhow!("fff search initialization failed: {error}")),
        }
    }

    pub fn track_access(&self, path: &Path) {
        let Some(Ok(state)) = self.state.get() else {
            return;
        };
        state.track_access(path);
    }

    pub fn notify_files_changed(&self) {
        let Some(Ok(state)) = self.state.get() else {
            return;
        };
        state.notify_files_changed();
    }
}

impl SearchState {
    fn new(workspace_root: PathBuf, config: &SearchConfig) -> Result<Self> {
        let shared_picker = SharedFilePicker::default();
        let shared_frecency = SharedFrecency::default();
        let shared_query_tracker = SharedQueryTracker::default();
        let db_root = fff_db_root(&workspace_root);
        let mut warnings = Vec::new();

        if let Err(error) = std::fs::create_dir_all(&db_root) {
            warnings.push(format!(
                "could not create fff db dir {}: {error}",
                db_root.display()
            ));
        } else {
            if let Err(error) = FrecencyTracker::open(db_root.join("frecency"))
                .and_then(|tracker| shared_frecency.init(tracker))
            {
                warnings.push(format!("frecency disabled: {error}"));
            }
            if let Err(error) = QueryTracker::open(db_root.join("queries"))
                .and_then(|tracker| shared_query_tracker.init(tracker))
            {
                warnings.push(format!("query history disabled: {error}"));
            }
        }

        FilePicker::new_with_shared_state(
            shared_picker.clone(),
            shared_frecency.clone(),
            FilePickerOptions {
                base_path: workspace_root.display().to_string(),
                enable_mmap_cache: false,
                enable_content_indexing: config.enable_content_indexing,
                mode: FFFMode::Ai,
                cache_budget: None,
                watch: true,
                follow_symlinks: config.follow_symlinks,
                enable_fs_root_scanning: false,
                enable_home_dir_scanning: workspace_is_home(&workspace_root),
            },
        )?;

        let scan_complete = shared_picker
            .wait_for_indexing_complete(Duration::from_secs(config.timeout_seconds.max(1) as u64));
        if !scan_complete {
            warnings.push("initial fff scan is still in progress".to_string());
        }

        Ok(Self {
            workspace_root,
            shared_picker,
            shared_frecency,
            shared_query_tracker,
            init_warning: if warnings.is_empty() {
                None
            } else {
                Some(warnings.join("; "))
            },
        })
    }

    fn track_access(&self, path: &Path) {
        if let Ok(guard) = self.shared_frecency.read() {
            if let Some(tracker) = guard.as_ref() {
                let _ = tracker.track_access(path);
            }
        }
    }

    fn track_grep_query(&self, query: &str) {
        if let Ok(mut guard) = self.shared_query_tracker.write() {
            if let Some(tracker) = guard.as_mut() {
                let _ = tracker.track_grep_query(query, &self.workspace_root);
            }
        }
    }

    fn notify_files_changed(&self) {
        let _ = self
            .shared_picker
            .trigger_full_rescan_async(&self.shared_frecency);
    }
}

impl FileSearchTool {
    pub fn new(config: SearchConfig, service: SearchService) -> Self {
        Self { config, service }
    }

    pub fn into_tool(self) -> Arc<dyn JsonTool> {
        Arc::new(self)
    }

    async fn execute(&self, args: Value) -> Result<Value> {
        let parsed: FileSearchArgs =
            serde_json::from_value(args).map_err(|error| anyhow!("invalid arguments: {error}"))?;
        let query = require_non_empty(&parsed.query, "query")?;
        let state = self.service.state(&self.config)?;
        let offset = parse_cursor(parsed.cursor.as_deref())?;
        let limit = normalize_limit(parsed.limit, self.config.max_results);
        let scoped_prefix =
            resolve_optional_scope(&state.workspace_root, parsed.path.as_deref(), &self.config)?;
        let deny_globs = build_glob_set(&self.config.deny_globs);

        let query = FFFQuery::parse(query, FileSearchConfig);
        let guard = state.shared_picker.read()?;
        let picker = guard
            .as_ref()
            .ok_or_else(|| anyhow!("fff picker is not initialized"))?;
        let query_guard = state.shared_query_tracker.read()?;
        let result = picker.fuzzy_search(
            &query,
            query_guard.as_ref(),
            FuzzySearchOptions {
                max_threads: 0,
                current_file: None,
                project_path: Some(&state.workspace_root),
                combo_boost_score_multiplier: 100,
                min_combo_count: 2,
                pagination: PaginationArgs {
                    offset,
                    limit: limit.saturating_mul(2).max(limit),
                },
            },
        );
        let returned = result.items.len();
        let total_matched = result.total_matched;
        let mut matches = Vec::new();
        for item in result.items {
            let relative_path = item.relative_path(picker);
            if !path_allowed(
                &state.workspace_root,
                &relative_path,
                scoped_prefix.as_deref(),
                &deny_globs,
            ) {
                continue;
            }
            matches.push(json!({
                "path": state.workspace_root.join(&relative_path).display().to_string(),
                "relative_path": relative_path,
                "kind": "file",
            }));
            if matches.len() >= limit {
                break;
            }
        }
        let count = matches.len();

        Ok(json!({
            "query": parsed.query,
            "matches": matches,
            "count": count,
            "total_matched": total_matched,
            "next_cursor": next_offset_cursor(offset, returned, total_matched),
            "scan": scan_status(picker),
            "warning": state.init_warning,
        }))
    }
}

#[async_trait]
impl JsonTool for FileSearchTool {
    fn definition(&self) -> ToolDefinition {
        ToolDefinition {
            name: FILE_SEARCH_NAME.to_string(),
            description: FILE_SEARCH_DESCRIPTION.to_string(),
            parameters: schema_for::<FileSearchArgs>(),
        }
    }

    async fn call(&self, args: Value) -> Result<Value> {
        self.execute(args).await
    }
}

impl GlobTool {
    pub fn new(config: SearchConfig, service: SearchService) -> Self {
        Self { config, service }
    }

    pub fn into_tool(self) -> Arc<dyn JsonTool> {
        Arc::new(self)
    }

    async fn execute(&self, args: Value) -> Result<Value> {
        let parsed: GlobArgs =
            serde_json::from_value(args).map_err(|error| anyhow!("invalid arguments: {error}"))?;
        let pattern = require_non_empty(&parsed.pattern, "pattern")?;
        reject_wildcard_only(pattern)?;
        let state = self.service.state(&self.config)?;
        let offset = parse_cursor(parsed.cursor.as_deref())?;
        let limit = normalize_limit(parsed.limit, self.config.max_results);
        let scoped_prefix =
            resolve_optional_scope(&state.workspace_root, parsed.path.as_deref(), &self.config)?;
        let deny_globs = build_glob_set(&self.config.deny_globs);

        let guard = state.shared_picker.read()?;
        let picker = guard
            .as_ref()
            .ok_or_else(|| anyhow!("fff picker is not initialized"))?;
        let result = picker.glob(
            pattern,
            FuzzySearchOptions {
                max_threads: 0,
                current_file: None,
                project_path: Some(&state.workspace_root),
                combo_boost_score_multiplier: 0,
                min_combo_count: 0,
                pagination: PaginationArgs {
                    offset,
                    limit: limit.saturating_mul(2).max(limit),
                },
            },
        );
        let returned = result.items.len();
        let total_matched = result.total_matched;
        let mut matches = Vec::new();
        for item in result.items {
            let relative_path = item.relative_path(picker);
            if !path_allowed(
                &state.workspace_root,
                &relative_path,
                scoped_prefix.as_deref(),
                &deny_globs,
            ) {
                continue;
            }
            matches.push(json!({
                "path": state.workspace_root.join(&relative_path).display().to_string(),
                "relative_path": relative_path,
            }));
            if matches.len() >= limit {
                break;
            }
        }
        let count = matches.len();

        Ok(json!({
            "pattern": parsed.pattern,
            "matches": matches,
            "count": count,
            "total_matched": total_matched,
            "next_cursor": next_offset_cursor(offset, returned, total_matched),
            "scan": scan_status(picker),
            "warning": state.init_warning,
        }))
    }
}

#[async_trait]
impl JsonTool for GlobTool {
    fn definition(&self) -> ToolDefinition {
        ToolDefinition {
            name: GLOB_NAME.to_string(),
            description: GLOB_DESCRIPTION.to_string(),
            parameters: schema_for::<GlobArgs>(),
        }
    }

    async fn call(&self, args: Value) -> Result<Value> {
        self.execute(args).await
    }
}

impl GrepTool {
    pub fn new(config: SearchConfig, service: SearchService) -> Self {
        Self { config, service }
    }

    pub fn into_tool(self) -> Arc<dyn JsonTool> {
        Arc::new(self)
    }

    async fn execute(&self, args: Value) -> Result<Value> {
        let parsed: GrepArgs =
            serde_json::from_value(args).map_err(|error| anyhow!("invalid arguments: {error}"))?;
        let pattern = require_non_empty(&parsed.pattern, "pattern")?;
        reject_wildcard_only(pattern)?;
        let state = self.service.state(&self.config)?;
        let file_offset = parse_cursor(parsed.cursor.as_deref())?;
        let limit = normalize_limit(parsed.limit, self.config.max_results);
        let constraints = OwnedGrepConstraints::new(
            &state.workspace_root,
            &self.config,
            &parsed.path,
            &parsed.include,
            &parsed.exclude,
        )?;
        let constraint_values = constraints.as_constraints();
        let mut query = FFFQuery::parse(pattern, AiGrepConfig);
        query.constraints.extend(constraint_values.iter().cloned());
        let options = grep_options(
            &self.config,
            &parsed.mode,
            parsed.case_sensitive,
            parsed.context,
            limit,
            file_offset,
            &[pattern],
        );

        let guard = state.shared_picker.read()?;
        let picker = guard
            .as_ref()
            .ok_or_else(|| anyhow!("fff picker is not initialized"))?;
        let result = picker.grep(&query, &options);
        state.track_grep_query(pattern);
        Ok(format_grep_result(
            &state,
            picker,
            pattern,
            None,
            result,
            state.init_warning.clone(),
        ))
    }
}

#[async_trait]
impl JsonTool for GrepTool {
    fn definition(&self) -> ToolDefinition {
        ToolDefinition {
            name: GREP_NAME.to_string(),
            description: GREP_DESCRIPTION.to_string(),
            parameters: schema_for::<GrepArgs>(),
        }
    }

    async fn call(&self, args: Value) -> Result<Value> {
        self.execute(args).await
    }
}

impl MultiGrepTool {
    pub fn new(config: SearchConfig, service: SearchService) -> Self {
        Self { config, service }
    }

    pub fn into_tool(self) -> Arc<dyn JsonTool> {
        Arc::new(self)
    }

    async fn execute(&self, args: Value) -> Result<Value> {
        let parsed: MultiGrepArgs =
            serde_json::from_value(args).map_err(|error| anyhow!("invalid arguments: {error}"))?;
        if parsed.patterns.is_empty() {
            return Err(anyhow!("patterns must contain at least one pattern"));
        }
        if parsed.patterns.len() > 32 {
            return Err(anyhow!("patterns must contain at most 32 patterns"));
        }
        let mut patterns = Vec::with_capacity(parsed.patterns.len());
        for pattern in &parsed.patterns {
            let pattern = require_non_empty(pattern, "patterns[]")?;
            reject_wildcard_only(pattern)?;
            if !patterns.contains(&pattern) {
                patterns.push(pattern);
            }
        }
        let state = self.service.state(&self.config)?;
        let file_offset = parse_cursor(parsed.cursor.as_deref())?;
        let limit = normalize_limit(parsed.limit, self.config.max_results);
        let constraints = OwnedGrepConstraints::new(
            &state.workspace_root,
            &self.config,
            &parsed.path,
            &parsed.include,
            &parsed.exclude,
        )?;
        let constraint_values = constraints.as_constraints();
        let options = grep_options(
            &self.config,
            &parsed.mode,
            parsed.case_sensitive,
            parsed.context,
            limit,
            file_offset,
            &patterns,
        );

        let guard = state.shared_picker.read()?;
        let picker = guard
            .as_ref()
            .ok_or_else(|| anyhow!("fff picker is not initialized"))?;
        let result = picker.multi_grep(&patterns, &constraint_values, &options);
        state.track_grep_query(&patterns.join(" OR "));
        Ok(format_grep_result(
            &state,
            picker,
            &patterns.join(" OR "),
            Some(&patterns),
            result,
            state.init_warning.clone(),
        ))
    }
}

#[async_trait]
impl JsonTool for MultiGrepTool {
    fn definition(&self) -> ToolDefinition {
        ToolDefinition {
            name: MULTI_GREP_NAME.to_string(),
            description: MULTI_GREP_DESCRIPTION.to_string(),
            parameters: schema_for::<MultiGrepArgs>(),
        }
    }

    async fn call(&self, args: Value) -> Result<Value> {
        self.execute(args).await
    }
}

fn grep_options(
    config: &SearchConfig,
    mode: &Option<GrepSearchMode>,
    case_sensitive: Option<bool>,
    context: Option<usize>,
    limit: usize,
    file_offset: usize,
    patterns: &[&str],
) -> GrepSearchOptions {
    let context = context.unwrap_or(0).min(5);
    GrepSearchOptions {
        max_file_size: config.max_output_bytes.max(64 * 1024),
        max_matches_per_file: 200,
        smart_case: case_sensitive.map(|value| !value).unwrap_or(true),
        file_offset,
        page_limit: limit,
        mode: match mode {
            Some(GrepSearchMode::Regex) => GrepMode::Regex,
            Some(GrepSearchMode::Fuzzy) => GrepMode::Fuzzy,
            Some(GrepSearchMode::Plain) => GrepMode::PlainText,
            Some(GrepSearchMode::Auto) | None => {
                if patterns.iter().any(|pattern| looks_like_regex(pattern)) {
                    GrepMode::Regex
                } else {
                    GrepMode::PlainText
                }
            }
        },
        time_budget_ms: config.timeout_seconds.max(1) as u64 * 1000,
        before_context: context,
        after_context: context,
        classify_definitions: true,
        trim_whitespace: false,
        abort_signal: None,
    }
}

fn format_grep_result(
    state: &SearchState,
    picker: &FilePicker,
    query: &str,
    patterns: Option<&[&str]>,
    result: fff_search::GrepResult<'_>,
    warning: Option<String>,
) -> Value {
    let mut matches = Vec::with_capacity(result.matches.len());
    for item in &result.matches {
        let Some(file) = result.files.get(item.file_index) else {
            continue;
        };
        let relative_path = file.relative_path(picker);
        let mut value = json!({
            "path": state.workspace_root.join(&relative_path).display().to_string(),
            "relative_path": relative_path,
            "line_number": item.line_number,
            "column": item.col,
            "line": item.line_content,
            "is_definition": item.is_definition,
            "context_before": item.context_before,
            "context_after": item.context_after,
        });
        if let Some(score) = item.fuzzy_score {
            value["fuzzy_score"] = json!(score);
        }
        if let Some(patterns) = patterns {
            value["matched_pattern"] = json!(best_matching_pattern(&item.line_content, patterns));
        }
        matches.push(value);
    }

    let count = matches.len();
    json!({
        "query": query,
        "matches": matches,
        "count": count,
        "files_with_matches": result.files_with_matches,
        "total_files": result.total_files,
        "filtered_file_count": result.filtered_file_count,
        "total_files_searched": result.total_files_searched,
        "next_cursor": if result.next_file_offset > 0 { Some(result.next_file_offset.to_string()) } else { None },
        "regex_fallback_error": result.regex_fallback_error,
        "warning": warning,
        "scan": scan_status(picker),
    })
}

fn best_matching_pattern<'a>(line: &str, patterns: &'a [&'a str]) -> Option<&'a str> {
    let lower = line.to_lowercase();
    patterns
        .iter()
        .copied()
        .find(|pattern| line.contains(*pattern) || lower.contains(&pattern.to_lowercase()))
        .or_else(|| patterns.first().copied())
}

struct OwnedGrepConstraints {
    path_file: Option<String>,
    path_glob: Option<String>,
    include: Option<String>,
    exclude: Option<String>,
}

impl OwnedGrepConstraints {
    fn new(
        workspace_root: &Path,
        config: &SearchConfig,
        path: &Option<String>,
        include: &Option<String>,
        exclude: &Option<String>,
    ) -> Result<Self> {
        let path_constraint = if let Some(path) = path
            .as_deref()
            .map(str::trim)
            .filter(|value| !value.is_empty())
        {
            let resolved = resolve_within_workspace(workspace_root, path, &config.allowed_roots)
                .map_err(map_path_error)?;
            let relative = relative_path_string(workspace_root, &resolved);
            if resolved.is_dir() {
                (None, Some(format!("{}/**", relative.trim_end_matches('/'))))
            } else {
                (Some(relative), None)
            }
        } else {
            (None, None)
        };
        Ok(Self {
            path_file: path_constraint.0,
            path_glob: path_constraint.1,
            include: include
                .as_deref()
                .map(str::trim)
                .filter(|value| !value.is_empty())
                .map(str::to_string),
            exclude: exclude
                .as_deref()
                .map(str::trim)
                .filter(|value| !value.is_empty())
                .map(str::to_string),
        })
    }

    fn as_constraints(&self) -> Vec<Constraint<'_>> {
        let mut constraints = Vec::new();
        if let Some(path_file) = self.path_file.as_deref() {
            constraints.push(Constraint::FilePath(path_file));
        }
        if let Some(path_glob) = self.path_glob.as_deref() {
            constraints.push(Constraint::Glob(path_glob));
        }
        if let Some(include) = self.include.as_deref() {
            constraints.push(Constraint::Glob(include));
        }
        if let Some(exclude) = self.exclude.as_deref() {
            constraints.push(Constraint::Not(Box::new(Constraint::Glob(exclude))));
        }
        constraints
    }
}

fn looks_like_regex(pattern: &str) -> bool {
    pattern.contains(".*")
        || pattern.contains(".+")
        || pattern.contains("\\")
        || pattern.contains("[")
        || pattern.contains("]")
        || pattern.contains("|")
        || pattern.contains("^")
        || pattern.contains("$")
        || pattern.contains("(?")
        || pattern.contains("{")
        || pattern.contains("}")
}

fn resolve_optional_scope(
    workspace_root: &Path,
    raw: Option<&str>,
    config: &SearchConfig,
) -> Result<Option<String>> {
    let Some(raw) = raw.map(str::trim).filter(|value| !value.is_empty()) else {
        return Ok(None);
    };
    let resolved = resolve_within_workspace(workspace_root, raw, &config.allowed_roots)
        .map_err(map_path_error)?;
    Ok(Some(relative_path_string(workspace_root, &resolved)))
}

fn path_allowed(
    workspace_root: &Path,
    relative_path: &str,
    scoped_prefix: Option<&str>,
    deny_globs: &GlobSet,
) -> bool {
    if let Some(prefix) = scoped_prefix {
        let prefix = prefix.trim_end_matches('/');
        if !prefix.is_empty()
            && relative_path != prefix
            && !relative_path.starts_with(&format!("{prefix}/"))
        {
            return false;
        }
    }
    let absolute = workspace_root.join(relative_path);
    enforce_deny_globs(&absolute, deny_globs).is_ok()
        && enforce_deny_globs(Path::new(relative_path), deny_globs).is_ok()
}

fn relative_path_string(workspace_root: &Path, path: &Path) -> String {
    path.strip_prefix(workspace_root)
        .unwrap_or(path)
        .to_string_lossy()
        .replace('\\', "/")
}

fn scan_status(picker: &FilePicker) -> Value {
    let progress = picker.get_scan_progress();
    json!({
        "scanned_files_count": progress.scanned_files_count,
        "is_scanning": progress.is_scanning,
        "is_watcher_ready": progress.is_watcher_ready,
        "is_warmup_complete": progress.is_warmup_complete,
    })
}

fn parse_cursor(cursor: Option<&str>) -> Result<usize> {
    let Some(cursor) = cursor.map(str::trim).filter(|value| !value.is_empty()) else {
        return Ok(0);
    };
    cursor
        .parse::<usize>()
        .map_err(|_| anyhow!("cursor must be a cursor returned by this tool"))
}

fn next_offset_cursor(offset: usize, returned: usize, total: usize) -> Option<String> {
    let next = offset.saturating_add(returned);
    if next < total {
        Some(next.to_string())
    } else {
        None
    }
}

fn normalize_limit(limit: Option<usize>, default_max: u32) -> usize {
    limit
        .unwrap_or(default_max as usize)
        .clamp(1, default_max.max(1) as usize)
}

fn require_non_empty<'a>(value: &'a str, field: &str) -> Result<&'a str> {
    let trimmed = value.trim();
    if trimmed.is_empty() {
        return Err(anyhow!("{field} must not be empty"));
    }
    Ok(trimmed)
}

fn reject_wildcard_only(pattern: &str) -> Result<()> {
    let compact: String = pattern.chars().filter(|ch| !ch.is_whitespace()).collect();
    if matches!(compact.as_str(), "*" | "**" | ".*" | ".+" | ".*?" | "^.*$") {
        return Err(anyhow!(
            "pattern is too broad; provide a concrete search term"
        ));
    }
    Ok(())
}

fn fff_db_root(workspace_root: &Path) -> PathBuf {
    let mut hasher = DefaultHasher::new();
    canonicalize_best_effort(workspace_root).hash(&mut hasher);
    std::env::temp_dir()
        .join("hivy-fff")
        .join(format!("{:016x}", hasher.finish()))
}

fn workspace_is_home(workspace_root: &Path) -> bool {
    let Some(home) = std::env::var_os("HOME") else {
        return false;
    };
    canonicalize_best_effort(workspace_root) == canonicalize_best_effort(Path::new(&home))
}

fn map_path_error(error: PathPolicyError) -> anyhow::Error {
    anyhow!(error.to_string())
}
