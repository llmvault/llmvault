use std::sync::Arc;

use domain::{
    ApplyPatchConfig, LspConfig, LspServerConfig, ReadFileConfig, SearchConfig, WriteFileConfig,
};
use serde_json::json;
use tools::{
    ApplyPatchTool, EditTool, FileSearchTool, GlobTool, GrepTool, JsonTool, LocalFsOperations,
    LspService, LspTool, MultiGrepTool, ReadTool, SearchService,
};

#[tokio::test]
async fn fff_search_tools_cover_paths_content_and_multi_pattern_search() {
    let dir = temp_dir("hivy-fff-tools");
    std::fs::create_dir_all(dir.join("src")).unwrap();
    std::fs::create_dir_all(dir.join("cmd")).unwrap();
    std::fs::write(
        dir.join("src/lib.rs"),
        "pub struct RuntimeToken;\npub fn compute_value() -> i32 { 42 }\n",
    )
    .unwrap();
    std::fs::write(dir.join("src/util.rs"), "pub fn helper() {}\n").unwrap();
    std::fs::write(
        dir.join("cmd/main.go"),
        "package main\nfunc main() { println(\"RuntimeToken\") }\n",
    )
    .unwrap();
    std::fs::write(dir.join("README.md"), "runtime fixture\n").unwrap();

    let service = SearchService::new(dir.clone());
    let config = SearchConfig {
        max_results: 20,
        timeout_seconds: 5,
        ..Default::default()
    };

    let file_search = FileSearchTool::new(config.clone(), service.clone());
    let found = file_search
        .call(json!({"query": "lib.rs"}))
        .await
        .expect("file_search should succeed");
    assert!(contains_relative_path(&found, "src/lib.rs"), "{found}");

    let glob = GlobTool::new(config.clone(), service.clone());
    let globbed = glob
        .call(json!({"pattern": "**/*.rs", "limit": 1}))
        .await
        .expect("glob should succeed");
    assert!(contains_relative_path(&globbed, "src/lib.rs"), "{globbed}");
    assert!(
        globbed.get("next_cursor").is_some(),
        "limited glob should expose cursor metadata: {globbed}"
    );

    let grep = GrepTool::new(config.clone(), service.clone());
    let matches = grep
        .call(json!({"pattern": "RuntimeToken", "include": "**/*.rs"}))
        .await
        .expect("grep should succeed");
    assert!(contains_relative_path(&matches, "src/lib.rs"), "{matches}");
    let regex_matches = grep
        .call(json!({"pattern": "Runtime.*Token", "include": "**/*.rs"}))
        .await
        .expect("grep auto regex should succeed");
    assert!(
        contains_relative_path(&regex_matches, "src/lib.rs"),
        "{regex_matches}"
    );

    let multi = MultiGrepTool::new(config, service);
    let multi_matches = multi
        .call(json!({
            "patterns": ["RuntimeToken", "compute_value"],
            "include": "**/*.{rs,go}",
            "limit": 10
        }))
        .await
        .expect("multi_grep should succeed");
    assert!(
        contains_relative_path(&multi_matches, "src/lib.rs"),
        "{multi_matches}"
    );
    assert!(
        multi_matches.to_string().contains("matched_pattern"),
        "{multi_matches}"
    );

    let _ = std::fs::remove_dir_all(&dir);
}

#[tokio::test]
async fn apply_patch_updates_adds_and_deletes_files() {
    let dir = temp_dir("hivy-apply-patch");
    std::fs::create_dir_all(&dir).unwrap();
    std::fs::write(dir.join("app.txt"), "alpha\nbeta\n").unwrap();
    std::fs::write(dir.join("remove.txt"), "delete me\n").unwrap();

    let tool = ApplyPatchTool::new(
        ApplyPatchConfig {
            allowed_roots: vec![],
            max_file_size_bytes: 1024 * 1024,
            deny_globs: vec![],
            atomic: true,
        },
        dir.clone(),
    );
    let result = tool
        .call(json!({
            "patch": "*** Begin Patch\n*** Update File: app.txt\n@@\n alpha\n-beta\n+gamma\n*** Add File: notes.txt\n+created\n*** Delete File: remove.txt\n*** End Patch"
        }))
        .await
        .expect("patch should apply");

    assert_eq!(
        std::fs::read_to_string(dir.join("app.txt")).unwrap(),
        "alpha\ngamma\n"
    );
    assert_eq!(
        std::fs::read_to_string(dir.join("notes.txt")).unwrap(),
        "created\n"
    );
    assert!(!dir.join("remove.txt").exists());
    assert_eq!(result["operation_count"], 3);

    let _ = std::fs::remove_dir_all(&dir);
}

#[tokio::test]
async fn apply_patch_accepts_wrapped_add_file_patches_from_models() {
    let dir = temp_dir("hivy-apply-patch-wrapped");
    std::fs::create_dir_all(&dir).unwrap();

    let tool = ApplyPatchTool::new(
        ApplyPatchConfig {
            allowed_roots: vec![],
            max_file_size_bytes: 1024 * 1024,
            deny_globs: vec![],
            atomic: true,
        },
        dir.clone(),
    );
    let result = tool
        .call(json!({
            "patch": "Here is the patch:\n```patch\n*** Begin Patch\n*** Add File: TOOLING_E2E.md\ntoken: test-token\nREAL_REPOS_CONFIRMED\n+FFF_TOOLS_CONFIRMED\nAPPLY_PATCH_CONFIRMED\nLSP_CONFIRMED\n*** End Patch\n```"
        }))
        .await
        .expect("wrapped model patch should apply");

    assert_eq!(
        std::fs::read_to_string(dir.join("TOOLING_E2E.md")).unwrap(),
        "token: test-token\nREAL_REPOS_CONFIRMED\nFFF_TOOLS_CONFIRMED\nAPPLY_PATCH_CONFIRMED\nLSP_CONFIRMED\n"
    );
    assert_eq!(result["operation_count"], 1);

    let _ = std::fs::remove_dir_all(&dir);
}

#[tokio::test]
async fn edit_file_rejects_stale_content_after_read() {
    let dir = temp_dir("hivy-stale-edit");
    std::fs::create_dir_all(&dir).unwrap();
    std::fs::write(dir.join("config.txt"), "value=one\n").unwrap();

    let config = WriteFileConfig {
        allowed_roots: vec![],
        max_file_size_bytes: 1024 * 1024,
        deny_globs: vec![],
        atomic: true,
    };
    let read_config = ReadFileConfig {
        allowed_roots: vec![],
        max_file_size_bytes: 1024 * 1024,
        deny_globs: vec![],
    };
    let files_read = Arc::new(std::sync::Mutex::new(std::collections::HashMap::new()));
    let read = ReadTool::new(read_config, dir.clone(), Arc::new(LocalFsOperations))
        .with_files_read(files_read.clone());
    read.call(json!({"path": "config.txt"}))
        .await
        .expect("read should succeed");
    std::fs::write(dir.join("config.txt"), "value=two\n").unwrap();

    let edit =
        EditTool::new(config, dir.clone(), Arc::new(LocalFsOperations)).with_files_read(files_read);
    let error = edit
        .call(json!({
            "path": "config.txt",
            "edits": [{"old_text": "value=two\n", "new_text": "value=three\n"}]
        }))
        .await
        .expect_err("stale edit should fail");
    assert!(
        error.to_string().contains("changed after it was read"),
        "{error}"
    );

    let _ = std::fs::remove_dir_all(&dir);
}

#[tokio::test]
async fn lsp_document_symbols_use_static_fallback_without_server_install() {
    let dir = temp_dir("hivy-lsp-tool");
    std::fs::create_dir_all(&dir).unwrap();
    std::fs::write(
        dir.join("service.toy"),
        "class Service:\n    def handle(self):\n        return 1\n",
    )
    .unwrap();

    let tool = LspTool::new(
        LspConfig::default(),
        dir.clone(),
        LspService::new(dir.clone()),
    );
    let result = tool
        .call(json!({"operation": "documentSymbol", "filePath": "service.toy"}))
        .await
        .expect("lsp document symbols should succeed");
    let rendered = result.to_string();
    assert!(rendered.contains("static_fallback"), "{rendered}");
    assert!(rendered.contains("Service"), "{rendered}");
    assert!(rendered.contains("handle"), "{rendered}");

    let _ = std::fs::remove_dir_all(&dir);
}

#[tokio::test]
async fn lsp_uses_real_json_rpc_server_when_configured() {
    if std::process::Command::new("python3")
        .arg("--version")
        .output()
        .is_err()
    {
        return;
    }

    let dir = temp_dir("hivy-real-lsp-tool");
    std::fs::create_dir_all(&dir).unwrap();
    std::fs::write(dir.join("service.fake"), "symbol RealSymbol\n").unwrap();
    std::fs::write(
        dir.join("fake_lsp.py"),
        r#"import json
import sys

def read_message():
    headers = {}
    while True:
        line = sys.stdin.buffer.readline()
        if not line:
            return None
        line = line.decode("ascii").strip()
        if not line:
            break
        key, value = line.split(":", 1)
        headers[key.lower()] = value.strip()
    body = sys.stdin.buffer.read(int(headers["content-length"]))
    return json.loads(body)

def send(message):
    body = json.dumps(message).encode("utf-8")
    sys.stdout.buffer.write(b"Content-Length: " + str(len(body)).encode("ascii") + b"\r\n\r\n" + body)
    sys.stdout.buffer.flush()

while True:
    message = read_message()
    if message is None:
        break
    method = message.get("method")
    if "id" in message:
        if method == "initialize":
            result = {"capabilities": {"textDocumentSync": 1, "documentSymbolProvider": True, "hoverProvider": True}}
        elif method == "textDocument/documentSymbol":
            result = [{
                "name": "RealSymbol",
                "kind": 12,
                "range": {"start": {"line": 0, "character": 0}, "end": {"line": 0, "character": 17}},
                "selectionRange": {"start": {"line": 0, "character": 7}, "end": {"line": 0, "character": 17}}
            }]
        elif method == "textDocument/hover":
            result = {"contents": {"kind": "plaintext", "value": "RealHover"}}
        else:
            result = None
        send({"jsonrpc": "2.0", "id": message["id"], "result": result})
    elif method == "textDocument/didOpen":
        uri = message["params"]["textDocument"]["uri"]
        send({"jsonrpc": "2.0", "method": "textDocument/publishDiagnostics", "params": {"uri": uri, "diagnostics": []}})
"#,
    )
    .unwrap();

    let config = LspConfig {
        timeout_seconds: 2,
        fallback_enabled: false,
        servers: vec![LspServerConfig {
            id: "fake".to_string(),
            command: vec![
                "python3".to_string(),
                dir.join("fake_lsp.py").display().to_string(),
            ],
            extensions: vec![".fake".to_string()],
            root_markers: vec![],
            strict_root: false,
            disabled: false,
            initialization_options: None,
        }],
        ..Default::default()
    };
    let tool = LspTool::new(config, dir.clone(), LspService::new(dir.clone()));
    let symbols = tool
        .call(json!({"operation": "documentSymbol", "filePath": "service.fake"}))
        .await
        .expect("documentSymbol should use fake lsp");
    let rendered = symbols.to_string();
    assert!(rendered.contains("\"backend\":\"lsp\""), "{rendered}");
    assert!(rendered.contains("RealSymbol"), "{rendered}");

    let hover = tool
        .call(json!({"operation": "hover", "filePath": "service.fake", "line": 1, "character": 8}))
        .await
        .expect("hover should use fake lsp");
    assert!(hover.to_string().contains("RealHover"), "{hover}");

    let _ = std::fs::remove_dir_all(&dir);
}

#[tokio::test]
async fn lsp_skips_servers_without_requested_capability() {
    if std::process::Command::new("python3")
        .arg("--version")
        .output()
        .is_err()
    {
        return;
    }

    let dir = temp_dir("hivy-lsp-capability-filter");
    std::fs::create_dir_all(&dir).unwrap();
    std::fs::write(dir.join("component.fake"), "symbol RealSymbol\n").unwrap();
    std::fs::write(
        dir.join("capability_lsp.py"),
        r#"import json
import sys

mode = sys.argv[1]

def read_message():
    headers = {}
    while True:
        line = sys.stdin.buffer.readline()
        if not line:
            return None
        line = line.decode("ascii").strip()
        if not line:
            break
        key, value = line.split(":", 1)
        headers[key.lower()] = value.strip()
    body = sys.stdin.buffer.read(int(headers["content-length"]))
    return json.loads(body)

def send(message):
    body = json.dumps(message).encode("utf-8")
    sys.stdout.buffer.write(b"Content-Length: " + str(len(body)).encode("ascii") + b"\r\n\r\n" + body)
    sys.stdout.buffer.flush()

while True:
    message = read_message()
    if message is None:
        break
    method = message.get("method")
    if "id" in message:
        if method == "initialize":
            capabilities = {"textDocumentSync": 1, "hoverProvider": True}
            if mode == "symbols":
                capabilities["documentSymbolProvider"] = True
            send({"jsonrpc": "2.0", "id": message["id"], "result": {"capabilities": capabilities}})
        elif method == "textDocument/documentSymbol" and mode == "symbols":
            send({"jsonrpc": "2.0", "id": message["id"], "result": [{
                "name": "RealSymbol",
                "kind": 12,
                "range": {"start": {"line": 0, "character": 0}, "end": {"line": 0, "character": 17}},
                "selectionRange": {"start": {"line": 0, "character": 7}, "end": {"line": 0, "character": 17}}
            }]})
        elif method == "textDocument/documentSymbol":
            send({"jsonrpc": "2.0", "id": message["id"], "error": {"code": -32601, "message": "documentSymbol should have been filtered"}})
        elif method == "textDocument/hover":
            send({"jsonrpc": "2.0", "id": message["id"], "result": {"contents": "hover"}})
        else:
            send({"jsonrpc": "2.0", "id": message["id"], "result": None})
"#,
    )
    .unwrap();

    let config = LspConfig {
        timeout_seconds: 2,
        fallback_enabled: false,
        servers: vec![
            LspServerConfig {
                id: "symbols".to_string(),
                command: vec![
                    "python3".to_string(),
                    dir.join("capability_lsp.py").display().to_string(),
                    "symbols".to_string(),
                ],
                extensions: vec![".fake".to_string()],
                root_markers: vec![],
                strict_root: false,
                disabled: false,
                initialization_options: None,
            },
            LspServerConfig {
                id: "hover-only".to_string(),
                command: vec![
                    "python3".to_string(),
                    dir.join("capability_lsp.py").display().to_string(),
                    "hover-only".to_string(),
                ],
                extensions: vec![".fake".to_string()],
                root_markers: vec![],
                strict_root: false,
                disabled: false,
                initialization_options: None,
            },
        ],
        ..Default::default()
    };
    let tool = LspTool::new(config, dir.clone(), LspService::new(dir.clone()));
    let symbols = tool
        .call(json!({"operation": "documentSymbol", "filePath": "component.fake"}))
        .await
        .expect("documentSymbol should skip hover-only lsp");
    let rendered = symbols.to_string();
    assert!(rendered.contains("RealSymbol"), "{rendered}");
    assert!(!rendered.contains("hover-only"), "{rendered}");

    let _ = std::fs::remove_dir_all(&dir);
}

fn contains_relative_path(value: &serde_json::Value, path: &str) -> bool {
    value.to_string().contains(path)
}

fn temp_dir(prefix: &str) -> std::path::PathBuf {
    std::env::temp_dir().join(format!(
        "{prefix}-{}-{}",
        std::process::id(),
        std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .unwrap()
            .as_nanos()
    ))
}
