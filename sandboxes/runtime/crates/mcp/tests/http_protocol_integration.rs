use std::collections::HashMap;
use std::path::PathBuf;
use std::process::Stdio;

use domain::{McpSpec, ToolFilter, ToolInputBinding};
use mcp::McpRegistry;
use serde_json::{json, Value};
use tokio::io::{AsyncBufReadExt, BufReader};
use tokio::process::{Child, Command};

struct FixtureServer {
    child: Child,
    base_url: String,
}

impl FixtureServer {
    async fn start() -> Self {
        let fixture =
            PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("tests/fixtures/http_mcp_server.py");
        let mut child = Command::new("python3")
            .arg(fixture)
            .stdout(Stdio::piped())
            .stderr(Stdio::inherit())
            .kill_on_drop(true)
            .spawn()
            .expect("start local MCP fixture");
        let stdout = child.stdout.take().expect("fixture stdout");
        let mut lines = BufReader::new(stdout).lines();
        let line = tokio::time::timeout(std::time::Duration::from_secs(5), lines.next_line())
            .await
            .expect("fixture startup timeout")
            .expect("read fixture port")
            .expect("fixture exited before reporting port");
        let port = line
            .strip_prefix("PORT=")
            .expect("fixture port prefix")
            .parse::<u16>()
            .expect("fixture port number");
        Self {
            child,
            base_url: format!("http://127.0.0.1:{port}"),
        }
    }

    async fn stop(mut self) {
        let _ = self.child.kill().await;
        let _ = self.child.wait().await;
    }
}

#[tokio::test]
async fn streamable_http_auth_catalog_activation_and_calls_work_end_to_end() {
    let fixture = FixtureServer::start().await;
    let specs = vec![
        streamable_spec(
            "noauth",
            format!("{}/noauth", fixture.base_url),
            HashMap::new(),
        ),
        streamable_spec(
            "static",
            format!("{}/static", fixture.base_url),
            HashMap::from([("X-API-Key".to_string(), "${STATIC_API_KEY}".to_string())]),
        ),
        streamable_spec(
            "bearer",
            format!("{}/static-bearer", fixture.base_url),
            HashMap::from([(
                "Authorization".to_string(),
                "Bearer ${STATIC_BEARER_TOKEN}".to_string(),
            )]),
        ),
        streamable_spec(
            "oauth",
            format!("{}/oauth", fixture.base_url),
            HashMap::from([(
                "Authorization".to_string(),
                "Bearer ${USER_OAUTH_TOKEN}".to_string(),
            )]),
        ),
        streamable_spec(
            "machine",
            format!("{}/client-credentials", fixture.base_url),
            HashMap::from([(
                "Authorization".to_string(),
                "Bearer ${CLIENT_CREDENTIALS_ACCESS_TOKEN}".to_string(),
            )]),
        ),
    ];
    let runtime_env = HashMap::from([
        ("STATIC_API_KEY".to_string(), "static-test-key".to_string()),
        (
            "STATIC_BEARER_TOKEN".to_string(),
            "static-bearer-token".to_string(),
        ),
        (
            "USER_OAUTH_TOKEN".to_string(),
            "oauth-user-token".to_string(),
        ),
        (
            "CLIENT_CREDENTIALS_ACCESS_TOKEN".to_string(),
            "machine-access-token".to_string(),
        ),
    ]);
    let registry = McpRegistry::from_specs_allowing_loopback_for_tests(
        &specs,
        &runtime_env,
        std::env::temp_dir(),
    )
    .await;

    let statuses = registry.connection_statuses();
    assert_eq!(statuses.len(), 5);
    assert!(statuses.iter().all(|status| status.connected));
    assert!(statuses.iter().all(|status| status.tool_count == 3));
    assert!(
        registry.live_connection_names().is_empty(),
        "discovery must close every transport"
    );

    let names = registry.available_tool_names();
    assert_eq!(names.len(), 15);
    assert!(names.contains(&"oauth_echo".to_string()));
    assert!(names.contains(&"machine_lookup_customer".to_string()));

    assert!(registry
        .activated_tools_filtered("session-a", "turn-a", None)
        .is_empty());
    assert!(registry
        .activate_tools_filtered("session-a", "turn-a", &[], None)
        .await
        .expect_err("empty batch must fail")
        .to_string()
        .contains("at least one"));
    let restricted = ToolFilter {
        allow: Some(vec!["oauth_echo".to_string()]),
        deny: None,
    };
    let invalid_batch = registry
        .activate_tools_filtered(
            "session-a",
            "turn-a",
            &["oauth_echo".to_string(), "static_echo".to_string()],
            Some(&restricted),
        )
        .await
        .expect_err("invalid batch must fail");
    assert!(invalid_batch.to_string().contains("static_echo"));
    assert!(
        registry
            .activated_tools_filtered("session-a", "turn-a", None)
            .is_empty(),
        "the complete batch must validate before any activation"
    );

    let activation = registry
        .activate_tools_filtered(
            "session-a",
            "turn-a",
            &["oauth_echo".to_string(), "static_echo".to_string()],
            None,
        )
        .await
        .expect("batch activate tools");
    assert_eq!(activation["loaded"], json!(["oauth_echo", "static_echo"]));
    assert_eq!(registry.live_connection_names(), vec!["oauth", "static"]);
    assert_eq!(
        registry.activated_tools_filtered("session-a", "turn-a", None)[0].prefixed_name,
        "oauth_echo"
    );
    assert!(registry
        .activated_tools_filtered("session-b", "turn-a", None)
        .is_empty());
    assert!(registry
        .activated_tools_filtered("session-a", "turn-b", None)
        .is_empty());
    let activated = registry.activated_tools_filtered("session-a", "turn-a", None);
    assert_eq!(activated[0].prefixed_name, "oauth_echo");
    assert_eq!(activated[1].prefixed_name, "static_echo");

    let repeat = registry
        .activate_tools_filtered(
            "session-a",
            "turn-a",
            &["static_echo".to_string(), "oauth_echo".to_string()],
            None,
        )
        .await
        .expect("repeat batch");
    assert_eq!(
        repeat["already_loaded"],
        json!(["static_echo", "oauth_echo"])
    );

    let oauth = registry
        .call_tool_for_session(
            "session-a",
            Some("user-123"),
            "oauth_echo",
            json!({"message": "hello OAuth"}),
        )
        .await
        .expect("call OAuth MCP tool");
    assert_eq!(oauth["structuredContent"]["message"], "hello OAuth");
    assert_eq!(
        oauth["structuredContent"]["authorization"],
        "Bearer oauth-user-token"
    );

    let static_result = registry
        .call_tool("static_echo", json!({"message": "hello static"}))
        .await
        .expect("call static MCP tool");
    assert_eq!(
        static_result["structuredContent"]["api_key"],
        "static-test-key"
    );

    let bearer = registry
        .call_tool("bearer_echo", json!({"message": "hello bearer"}))
        .await
        .expect("call static bearer MCP tool");
    assert_eq!(
        bearer["structuredContent"]["authorization"],
        "Bearer static-bearer-token"
    );

    let machine = registry
        .call_tool("machine_echo", json!({"message": "hello machine"}))
        .await
        .expect("call client-credentials MCP tool");
    assert_eq!(
        machine["structuredContent"]["authorization"],
        "Bearer machine-access-token"
    );

    let noauth = registry
        .call_tool("noauth_echo", json!({"message": "hello public"}))
        .await
        .expect("call no-auth MCP tool");
    assert_eq!(noauth["structuredContent"]["authorization"], Value::Null);
    assert_eq!(noauth["structuredContent"]["api_key"], Value::Null);

    fixture.stop().await;
}

#[tokio::test]
async fn workspace_text_file_binding_projects_schema_and_injects_file_contents() {
    let fixture = FixtureServer::start().await;
    let workspace = std::env::temp_dir().join(format!(
        "hivy-mcp-binding-{}-{}",
        std::process::id(),
        std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .expect("system time")
            .as_nanos()
    ));
    tokio::fs::create_dir_all(workspace.join("investigations"))
        .await
        .expect("create workspace");
    tokio::fs::write(
        workspace.join("investigations/report.md"),
        "# Cluster healthy\n",
    )
    .await
    .expect("write report");
    let specs = vec![McpSpec::StreamableHttp {
        name: "hivy".to_string(),
        url: format!("{}/noauth", fixture.base_url),
        headers: HashMap::new(),
        tool_filter: None,
        tool_name_prefix: None,
        tool_input_bindings: vec![ToolInputBinding::WorkspaceTextFile {
            tool: "echo".to_string(),
            path_argument: "message_file_path".to_string(),
            content_argument: "message".to_string(),
            allowed_extensions: vec![".md".to_string()],
            max_bytes: 1_048_576,
            encoding: "utf-8".to_string(),
        }],
    }];
    let registry = McpRegistry::from_specs_allowing_loopback_for_tests(
        &specs,
        &HashMap::new(),
        workspace.clone(),
    )
    .await;

    registry
        .activate_tools_filtered(
            "binding-session",
            "binding-turn",
            &["hivy_echo".to_string()],
            None,
        )
        .await
        .expect("activate bound tool");
    let activated = registry.activated_tools_filtered("binding-session", "binding-turn", None);
    let details = &activated[0].parameters;
    assert_eq!(details["required"], json!(["message_file_path"]));
    assert!(details["properties"].get("message").is_none());
    assert_eq!(details["properties"]["message_file_path"]["type"], "string");

    let result = registry
        .call_tool(
            "hivy_echo",
            json!({"message_file_path": "investigations/report.md"}),
        )
        .await
        .expect("call tool with workspace file");
    assert_eq!(
        result["structuredContent"]["message"],
        "# Cluster healthy\n"
    );

    drop(registry);
    tokio::fs::remove_dir_all(workspace)
        .await
        .expect("remove workspace");
    fixture.stop().await;
}

#[tokio::test]
async fn workspace_bundle_binding_projects_paths_and_injects_complete_skill_bundle() {
    let fixture = FixtureServer::start().await;
    let workspace = std::env::temp_dir().join(format!(
        "hivy-mcp-skill-bundle-{}-{}",
        std::process::id(),
        std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .expect("system time")
            .as_nanos()
    ));
    tokio::fs::create_dir_all(workspace.join("skills/status/references"))
        .await
        .expect("create references directory");
    tokio::fs::create_dir_all(workspace.join("skills/status/scripts"))
        .await
        .expect("create scripts directory");
    let skill_md = "---\nname: status-check\ndescription: Use when checking status.\n---\n\n# Status\nRun the script.\n";
    tokio::fs::write(workspace.join("skills/status/SKILL.md"), skill_md)
        .await
        .expect("write skill entrypoint");
    tokio::fs::write(
        workspace.join("skills/status/references/api.md"),
        "# API\nUse the health endpoint.\n",
    )
    .await
    .expect("write reference");
    tokio::fs::write(
        workspace.join("skills/status/scripts/check.sh"),
        "#!/bin/sh\ncurl -fsS \"$STATUS_URL\"\n",
    )
    .await
    .expect("write script");
    tokio::fs::create_dir_all(workspace.join("other"))
        .await
        .expect("create unrelated directory");
    tokio::fs::write(workspace.join("other/secret.txt"), "not part of the skill")
        .await
        .expect("write unrelated file");

    let specs = vec![McpSpec::StreamableHttp {
        name: "hivy".to_string(),
        url: format!("{}/noauth", fixture.base_url),
        headers: HashMap::new(),
        tool_filter: None,
        tool_name_prefix: None,
        tool_input_bindings: vec![ToolInputBinding::WorkspaceBundle {
            tool: "create_skill".to_string(),
            entrypoint_path_argument: "entrypoint_file_path".to_string(),
            supporting_file_paths_argument: "supporting_file_paths".to_string(),
            entrypoint_content_argument: "entrypoint_content".to_string(),
            files_argument: "files".to_string(),
            entrypoint_filename: "SKILL.md".to_string(),
            allowed_directories: vec![
                "references".to_string(),
                "templates".to_string(),
                "scripts".to_string(),
                "assets".to_string(),
            ],
            max_files: 256,
            max_file_bytes: 4 * 1024 * 1024,
            max_total_bytes: 16 * 1024 * 1024,
            encoding: "utf-8".to_string(),
        }],
    }];
    let registry = McpRegistry::from_specs_allowing_loopback_for_tests(
        &specs,
        &HashMap::new(),
        workspace.clone(),
    )
    .await;

    registry
        .activate_tools_filtered(
            "skill-session",
            "skill-turn",
            &["hivy_create_skill".to_string()],
            None,
        )
        .await
        .expect("activate bound create_skill");
    let activated = registry.activated_tools_filtered("skill-session", "skill-turn", None);
    let parameters = &activated[0].parameters;
    assert_eq!(parameters["required"], json!(["entrypoint_file_path"]));
    assert!(parameters["properties"].get("entrypoint_content").is_none());
    assert!(parameters["properties"].get("files").is_none());
    assert_eq!(
        parameters["properties"]["entrypoint_file_path"]["type"],
        "string"
    );
    assert_eq!(
        parameters["properties"]["supporting_file_paths"]["items"]["type"],
        "string"
    );

    let result = registry
        .call_tool(
            "hivy_create_skill",
            json!({
                "entrypoint_file_path": "skills/status/SKILL.md",
                "supporting_file_paths": [
                    "skills/status/references/api.md",
                    "skills/status/scripts/check.sh"
                ]
            }),
        )
        .await
        .expect("create skill from workspace bundle");
    assert_eq!(result["structuredContent"]["entrypoint_content"], skill_md);
    assert_eq!(
        result["structuredContent"]["files"]["references/api.md"],
        "# API\nUse the health endpoint.\n"
    );
    assert_eq!(
        result["structuredContent"]["files"]["scripts/check.sh"],
        "#!/bin/sh\ncurl -fsS \"$STATUS_URL\"\n"
    );

    let escaped_bundle = registry
        .call_tool(
            "hivy_create_skill",
            json!({
                "entrypoint_file_path": "skills/status/SKILL.md",
                "supporting_file_paths": ["other/secret.txt"]
            }),
        )
        .await
        .expect_err("supporting file outside the entrypoint directory must fail");
    assert!(escaped_bundle
        .to_string()
        .contains("beneath the skill entrypoint directory"));

    drop(registry);
    tokio::fs::remove_dir_all(workspace)
        .await
        .expect("remove workspace");
    fixture.stop().await;
}

#[tokio::test]
async fn config_reload_discovers_in_background_then_leaves_servers_dormant_until_activation() {
    let fixture = FixtureServer::start().await;
    let registry = std::sync::Arc::new(
        McpRegistry::from_specs_allowing_loopback_for_tests(
            &[],
            &HashMap::new(),
            std::env::temp_dir(),
        )
        .await,
    );
    let specs = vec![
        streamable_spec(
            "slow-a",
            format!("{}/slow", fixture.base_url),
            HashMap::new(),
        ),
        streamable_spec(
            "slow-b",
            format!("{}/slow", fixture.base_url),
            HashMap::new(),
        ),
    ];

    let started = std::time::Instant::now();
    let discovery = registry.reload_from_specs_in_background(&specs, &HashMap::new());
    assert!(
        started.elapsed() < std::time::Duration::from_millis(100),
        "config reload must not wait for MCP discovery"
    );
    assert!(registry.available_tool_names().is_empty());
    assert!(registry.live_connection_names().is_empty());
    assert!(
        tokio::time::timeout(
            std::time::Duration::from_millis(100),
            registry.wait_until_ready()
        )
        .await
        .is_err(),
        "a turn must wait instead of snapshotting the temporarily empty catalog"
    );

    discovery.await.expect("background MCP discovery task");
    tokio::time::timeout(
        std::time::Duration::from_millis(100),
        registry.wait_until_ready(),
    )
    .await
    .expect("ready catalog should unblock turns");
    assert!(
        started.elapsed() < std::time::Duration::from_millis(1800),
        "two one-second discoveries should run in parallel"
    );
    assert_eq!(registry.connection_statuses().len(), 2);
    assert!(registry
        .connection_statuses()
        .iter()
        .all(|status| status.connected));
    assert_eq!(registry.available_tool_names().len(), 6);
    assert!(
        registry.live_connection_names().is_empty(),
        "discovery transports must be shut down"
    );

    registry
        .activate_tools_filtered("session-a", "turn-a", &["slow-a_echo".to_string()], None)
        .await
        .expect("activate tool from first server");
    assert_eq!(registry.live_connection_names(), vec!["slow-a"]);
    assert!(registry
        .activated_tools_filtered("session-a", "turn-a", None)
        .iter()
        .any(|tool| tool.prefixed_name == "slow-a_echo"));
    assert!(registry
        .activated_tools_filtered("session-a", "turn-a", None)
        .iter()
        .all(|tool| tool.server_name != "slow-b"));

    fixture.stop().await;
}

#[tokio::test]
async fn explicit_tool_prefix_sets_the_model_facing_connection_name() {
    let fixture = FixtureServer::start().await;
    let specs = vec![McpSpec::StreamableHttp {
        name: "database-postgres".to_string(),
        url: format!("{}/noauth", fixture.base_url),
        headers: HashMap::new(),
        tool_filter: None,
        tool_name_prefix: Some("postgres_primary".to_string()),
        tool_input_bindings: Vec::new(),
    }];
    let registry = McpRegistry::from_specs_allowing_loopback_for_tests(
        &specs,
        &HashMap::new(),
        std::env::temp_dir(),
    )
    .await;

    let names = registry.available_tool_names();
    assert!(names.contains(&"postgres_primary_echo".to_string()));
    assert!(names.contains(&"postgres_primary_lookup_customer".to_string()));

    registry
        .activate_tools_filtered(
            "database-session",
            "database-turn",
            &["postgres_primary_echo".to_string()],
            None,
        )
        .await
        .expect("activate prefixed database tool");
    let activated = registry.activated_tools_filtered("database-session", "database-turn", None);
    assert_eq!(activated[0].raw_name, "echo");
    assert_eq!(activated[0].server_name, "database-postgres");

    fixture.stop().await;
}

#[tokio::test]
async fn legacy_http_sse_connects_discovers_activates_and_calls_with_oauth_header() {
    let fixture = FixtureServer::start().await;
    let specs = vec![McpSpec::Sse {
        name: "legacy".to_string(),
        url: format!("{}/legacy-sse", fixture.base_url),
        headers: HashMap::from([(
            "Authorization".to_string(),
            "Bearer ${LEGACY_OAUTH_TOKEN}".to_string(),
        )]),
        tool_filter: None,
        tool_name_prefix: None,
        tool_input_bindings: Vec::new(),
    }];
    let runtime_env = HashMap::from([(
        "LEGACY_OAUTH_TOKEN".to_string(),
        "legacy-oauth-token".to_string(),
    )]);
    let registry = McpRegistry::from_specs_allowing_loopback_for_tests(
        &specs,
        &runtime_env,
        std::env::temp_dir(),
    )
    .await;

    let statuses = registry.connection_statuses();
    assert_eq!(statuses.len(), 1);
    assert!(statuses[0].connected, "legacy SSE status: {statuses:?}");
    assert_eq!(statuses[0].tool_count, 3);
    assert_eq!(
        registry.available_tool_names(),
        vec![
            "legacy_create_skill",
            "legacy_echo",
            "legacy_lookup_customer"
        ]
    );

    let activation = registry
        .activate_tools_filtered(
            "legacy-session",
            "legacy-turn",
            &["legacy_echo".to_string()],
            None,
        )
        .await
        .expect("activate legacy SSE tool");
    assert_eq!(activation["loaded"], json!(["legacy_echo"]));
    assert_eq!(
        registry.activated_tools_filtered("legacy-session", "legacy-turn", None)[0].prefixed_name,
        "legacy_echo"
    );

    let result = registry
        .call_tool_for_session(
            "legacy-session",
            Some("user-legacy"),
            "legacy_echo",
            json!({"message": "legacy transport works"}),
        )
        .await
        .expect("call legacy SSE MCP tool");
    assert_eq!(
        result["structuredContent"]["message"],
        "legacy transport works"
    );
    assert_eq!(result["structuredContent"]["endpoint"], "/legacy-sse");
    assert_eq!(
        result["structuredContent"]["authorization"],
        "Bearer legacy-oauth-token"
    );

    drop(registry);
    fixture.stop().await;
}

#[tokio::test]
async fn unsafe_and_long_mcp_names_get_exact_collision_safe_callable_names() {
    let fixture = FixtureServer::start().await;
    let specs = vec![streamable_spec(
        "interop",
        format!("{}/names", fixture.base_url),
        HashMap::new(),
    )];
    let registry = McpRegistry::from_specs_allowing_loopback_for_tests(
        &specs,
        &HashMap::new(),
        std::env::temp_dir(),
    )
    .await;
    let statuses = registry.connection_statuses();
    assert!(statuses[0].connected, "interop status: {statuses:?}");
    assert_eq!(statuses[0].tool_count, 4);

    let catalog = registry.available_tool_names();
    assert_eq!(catalog.len(), 4, "sanitized names must not collide");
    assert!(catalog.iter().all(|name| {
        name.len() <= 64
            && name
                .bytes()
                .all(|byte| byte.is_ascii_alphanumeric() || matches!(byte, b'_' | b'-'))
    }));

    registry
        .activate_tools_filtered("interop-session", "interop-turn", &catalog, None)
        .await
        .expect("load every sanitized callable name");
    let activated = registry.activated_tools_filtered("interop-session", "interop-turn", None);
    let raw_names = [
        "records.list".to_string(),
        "records/list".to_string(),
        "records_list".to_string(),
        format!("read_{}", "x".repeat(100)),
    ];
    for raw_name in raw_names {
        let definition = activated
            .iter()
            .find(|tool| tool.raw_name == raw_name)
            .expect("raw MCP name remains mapped to an activated callable");
        let callable = definition.prefixed_name.clone();
        assert!(catalog.contains(&callable));

        let called = registry
            .call_tool(&callable, json!({}))
            .await
            .expect("call sanitized MCP tool");
        assert_eq!(
            called["structuredContent"]["raw_name"], raw_name,
            "runtime must send the untouched raw name to the MCP server"
        );
    }

    fixture.stop().await;
}

#[tokio::test]
async fn unresolved_auth_placeholder_is_reported_as_connection_status() {
    let specs = vec![streamable_spec(
        "missing-auth",
        "http://127.0.0.1:9/mcp".to_string(),
        HashMap::from([(
            "Authorization".to_string(),
            "Bearer ${MISSING_TOKEN}".to_string(),
        )]),
    )];
    let registry = McpRegistry::from_specs(&specs, &HashMap::new(), std::env::temp_dir()).await;
    let statuses = registry.connection_statuses();
    assert_eq!(statuses.len(), 1);
    assert!(!statuses[0].connected);
    assert!(statuses[0]
        .error
        .as_deref()
        .expect("connection error")
        .contains("missing runtime environment value"));
}

#[tokio::test]
async fn production_policy_rejects_loopback_and_cloud_metadata_before_dial() {
    let specs = vec![
        streamable_spec(
            "loopback",
            "https://127.0.0.1:443/mcp".to_string(),
            HashMap::new(),
        ),
        McpSpec::Sse {
            name: "metadata".to_string(),
            url: "https://169.254.169.254/latest/meta-data/".to_string(),
            headers: HashMap::new(),
            tool_filter: None,
            tool_name_prefix: None,
            tool_input_bindings: Vec::new(),
        },
    ];
    let registry = McpRegistry::from_specs(&specs, &HashMap::new(), std::env::temp_dir()).await;
    let statuses = registry.connection_statuses();
    assert_eq!(statuses.len(), 2);
    assert!(statuses.iter().all(|status| !status.connected));
    assert!(statuses[0]
        .error
        .as_deref()
        .expect("loopback policy error")
        .contains("127.0.0.1"));
    assert!(statuses[1]
        .error
        .as_deref()
        .expect("metadata policy error")
        .contains("169.254.169.254"));
}

#[tokio::test]
async fn streamable_and_legacy_transports_refuse_redirects() {
    let fixture = FixtureServer::start().await;
    let specs = vec![
        streamable_spec(
            "streamable-redirect",
            format!("{}/streamable-redirect", fixture.base_url),
            HashMap::new(),
        ),
        McpSpec::Sse {
            name: "legacy-redirect".to_string(),
            url: format!("{}/legacy-redirect", fixture.base_url),
            headers: HashMap::new(),
            tool_filter: None,
            tool_name_prefix: None,
            tool_input_bindings: Vec::new(),
        },
    ];
    let registry = McpRegistry::from_specs_allowing_loopback_for_tests(
        &specs,
        &HashMap::new(),
        std::env::temp_dir(),
    )
    .await;
    let statuses = registry.connection_statuses();
    assert_eq!(statuses.len(), 2);
    assert!(statuses.iter().all(|status| !status.connected));
    assert!(statuses.iter().all(|status| status
        .error
        .as_deref()
        .is_some_and(|error| error.contains("307"))));

    fixture.stop().await;
}

#[tokio::test]
async fn stdio_startup_timeout_is_enforced_and_reported() {
    let specs = vec![McpSpec::Stdio {
        name: "sleeping".to_string(),
        command: "python3".to_string(),
        args: vec!["-c".to_string(), "import time; time.sleep(60)".to_string()],
        env: HashMap::new(),
        tool_filter: None,
        tool_name_prefix: None,
        tool_input_bindings: Vec::new(),
        startup_timeout_seconds: Some(1),
    }];
    let started = std::time::Instant::now();
    let registry = McpRegistry::from_specs(&specs, &HashMap::new(), std::env::temp_dir()).await;
    assert!(
        started.elapsed() < std::time::Duration::from_secs(5),
        "configured startup timeout was not enforced"
    );
    let statuses = registry.connection_statuses();
    assert_eq!(statuses.len(), 1);
    assert!(!statuses[0].connected);
    assert!(statuses[0]
        .error
        .as_deref()
        .expect("timeout error")
        .contains("timed out after 1 seconds"));
}

fn streamable_spec(name: &str, url: String, headers: HashMap<String, String>) -> McpSpec {
    McpSpec::StreamableHttp {
        name: name.to_string(),
        url,
        headers,
        tool_filter: None,
        tool_name_prefix: None,
        tool_input_bindings: Vec::new(),
    }
}
