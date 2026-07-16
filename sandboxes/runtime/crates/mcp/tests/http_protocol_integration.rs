use std::collections::HashMap;
use std::path::PathBuf;
use std::process::Stdio;

use domain::McpSpec;
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
    assert!(statuses.iter().all(|status| status.tool_count == 2));
    assert!(
        registry.live_connection_names().is_empty(),
        "discovery must close every transport"
    );

    let names = registry.available_tool_names();
    assert_eq!(names.len(), 10);
    assert!(names.contains(&"oauth_echo".to_string()));
    assert!(names.contains(&"machine_lookup_customer".to_string()));

    let search = registry.search_tools_filtered("customer email", "summary", None, None);
    assert!(search["total"].as_u64().expect("search total") >= 4);
    assert!(search["servers"].as_array().expect("grouped servers").len() >= 5);
    assert!(search.to_string().contains("lookup_customer"));

    assert!(registry
        .activated_tools_filtered("session-a", None)
        .is_empty());
    let details = registry
        .activate_tool_filtered("session-a", "oauth_echo", None)
        .await
        .expect("activate OAuth tool");
    assert_eq!(registry.live_connection_names(), vec!["oauth"]);
    assert_eq!(details["activated"], true);
    assert_eq!(details["input_schema"]["required"][0], "message");
    assert_eq!(
        registry.activated_tools_filtered("session-a", None)[0].prefixed_name,
        "oauth_echo"
    );
    assert!(registry
        .activated_tools_filtered("session-b", None)
        .is_empty());

    registry
        .activate_tool_filtered("session-a", "static_echo", None)
        .await
        .expect("activate static tool");
    let activated = registry.activated_tools_filtered("session-a", None);
    assert_eq!(activated[0].prefixed_name, "oauth_echo");
    assert_eq!(activated[1].prefixed_name, "static_echo");

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

    discovery.await.expect("background MCP discovery task");
    assert!(
        started.elapsed() < std::time::Duration::from_millis(1800),
        "two one-second discoveries should run in parallel"
    );
    assert_eq!(registry.connection_statuses().len(), 2);
    assert!(registry
        .connection_statuses()
        .iter()
        .all(|status| status.connected));
    assert_eq!(registry.available_tool_names().len(), 4);
    assert!(
        registry.live_connection_names().is_empty(),
        "discovery transports must be shut down"
    );

    registry
        .activate_tool_filtered("session-a", "slow-a_echo", None)
        .await
        .expect("activate tool from first server");
    assert_eq!(registry.live_connection_names(), vec!["slow-a"]);
    assert!(registry
        .activated_tools_filtered("session-a", None)
        .iter()
        .any(|tool| tool.prefixed_name == "slow-a_echo"));
    assert!(registry
        .activated_tools_filtered("session-a", None)
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

    let details = registry
        .activate_tool_filtered("database-session", "postgres_primary_echo", None)
        .await
        .expect("activate prefixed database tool");
    assert_eq!(details["raw_name"], "echo");
    assert_eq!(details["server"], "database-postgres");

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
    assert_eq!(statuses[0].tool_count, 2);
    assert_eq!(
        registry.available_tool_names(),
        vec!["legacy_echo", "legacy_lookup_customer"]
    );

    let search = registry.search_tools_filtered("echo payload", "summary", None, None);
    assert_eq!(search["servers"][0]["server"], "legacy");
    assert_eq!(search["servers"][0]["tools"][0]["name"], "legacy_echo");
    let details = registry
        .activate_tool_filtered("legacy-session", "legacy_echo", None)
        .await
        .expect("activate legacy SSE tool");
    assert_eq!(details["activated"], true);
    assert_eq!(
        registry.activated_tools_filtered("legacy-session", None)[0].prefixed_name,
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

    let raw_names = [
        "records.list".to_string(),
        "records/list".to_string(),
        "records_list".to_string(),
        format!("read_{}", "x".repeat(100)),
    ];
    for raw_name in raw_names {
        let search = registry.search_tools_filtered(&raw_name, "full", Some(10), None);
        let result = search["servers"]
            .as_array()
            .expect("search servers")
            .iter()
            .flat_map(|server| server["tools"].as_array().expect("server tools"))
            .find(|tool| tool["raw_name"] == raw_name)
            .expect("raw MCP name remains searchable");
        let callable = result["name"]
            .as_str()
            .expect("model-safe callable name")
            .to_string();
        assert!(catalog.contains(&callable));

        let details = registry
            .activate_tool_filtered("interop-session", &callable, None)
            .await
            .expect("activate exact callable name");
        assert_eq!(details["name"], callable);
        assert_eq!(details["raw_name"], raw_name);

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
    }
}
