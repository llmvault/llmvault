use std::collections::HashMap;
use std::sync::atomic::{AtomicU64, Ordering};

use domain::{
    AgentDefinition, AgentMeta, ModelConfig, StaticPromptSegment, SystemPromptConfig,
    SystemPromptSegment, WorkspaceConfig, WorkspaceRepoConfig,
};
use storage::{init_sqlite_store, ConfigRepo, ConfigSnapshot, SqliteConfigRepo};

static DB_COUNTER: AtomicU64 = AtomicU64::new(0);

#[tokio::test]
async fn config_snapshot_persists_definition_and_runtime_env() {
    let unique = DB_COUNTER.fetch_add(1, Ordering::Relaxed);
    let db_path = std::env::temp_dir().join(format!(
        "agent-config-snapshot-{}-{unique}.db",
        std::process::id()
    ));
    let store = init_sqlite_store(&db_path).await.expect("init sqlite");
    let repo = SqliteConfigRepo::new(&store);

    repo.upsert(&ConfigSnapshot {
        definition: test_definition("snapshot-agent"),
        runtime_env: HashMap::from([
            ("HIVY_PROXY_API_KEY".to_string(), "ptok_config".to_string()),
            (
                "HIVY_RUNTIME_SECRET".to_string(),
                "runtime-secret".to_string(),
            ),
        ]),
        workspace: WorkspaceConfig {
            repos: vec![WorkspaceRepoConfig {
                id: "usehivy/hivy".to_string(),
                name: "hivy".to_string(),
                full_name: "usehivy/hivy".to_string(),
                clone_url: "https://github.com/usehivy/hivy.git".to_string(),
                depth: Some(1),
            }],
        },
    })
    .await
    .expect("upsert snapshot");

    let loaded = repo
        .load()
        .await
        .expect("load snapshot")
        .expect("snapshot exists");
    assert_eq!(loaded.definition.agent.name, "snapshot-agent");
    assert_eq!(
        loaded.runtime_env.get("HIVY_PROXY_API_KEY"),
        Some(&"ptok_config".to_string())
    );
    assert_eq!(loaded.workspace.repos.len(), 1);
    assert_eq!(loaded.workspace.repos[0].full_name, "usehivy/hivy");

    let _ = std::fs::remove_file(db_path);
}

fn test_definition(name: &str) -> AgentDefinition {
    AgentDefinition {
        agent: AgentMeta {
            name: name.to_string(),
            description: String::new(),
        },
        system_prompt: SystemPromptConfig {
            cacheable_segments: vec![SystemPromptSegment::StaticText(StaticPromptSegment {
                title: String::new(),
                content: "test".to_string(),
            })],
            dynamic_segments: Vec::new(),
        },
        model: ModelConfig::OpenaiCompatible {
            base_url: "http://localhost".to_string(),
            model_id: "test".to_string(),
            canonical_model_id: Some("test".to_string()),
            provider_id: Some("atlascloud".to_string()),
            upstream_model_id: Some("test".to_string()),
            model_profile: None,
            provider_options: HashMap::new(),
            capabilities: None,
            api_key_env: "HIVY_PROXY_API_KEY".to_string(),
            temperature: None,
            max_output_tokens: None,
            reasoning_effort: None,
            extra_headers: HashMap::new(),
            fallback: None,
        },
        limits: Default::default(),
        context: Default::default(),
        tools: Some(Vec::new()),
        mcp_servers: Vec::new(),
        mcp_tool_filter: None,
        outbound_channels: Vec::new(),
        sub_agents: Default::default(),
        safety: Default::default(),
        auto_load_skills: Default::default(),
    }
}
