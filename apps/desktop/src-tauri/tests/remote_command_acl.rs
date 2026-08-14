use std::{collections::BTreeSet, fs, path::Path};

use serde_json::Value;

const BRIDGE_PERMISSIONS: [&str; 4] = [
    "allow-desktop-cloud-url",
    "allow-desktop-info",
    "allow-runtime-request",
    "allow-runtime-session-stream",
];

#[test]
fn remote_cloud_capability_grants_every_desktop_bridge_command() {
    let manifest_dir = Path::new(env!("CARGO_MANIFEST_DIR"));
    let capability: Value = serde_json::from_slice(
        &fs::read(manifest_dir.join("capabilities/default.json")).expect("read desktop capability"),
    )
    .expect("parse desktop capability");
    let granted = capability["permissions"]
        .as_array()
        .expect("desktop capability permissions")
        .iter()
        .filter_map(Value::as_str)
        .collect::<BTreeSet<_>>();

    let acl_manifests: Value = serde_json::from_slice(
        &fs::read(manifest_dir.join("gen/schemas/acl-manifests.json"))
            .expect("read generated ACL manifests"),
    )
    .expect("parse generated ACL manifests");
    let application_permissions = acl_manifests["__app-acl__"]["permissions"]
        .as_object()
        .expect("generated application permissions");

    for permission in BRIDGE_PERMISSIONS {
        assert!(
            granted.contains(permission),
            "remote capability must grant {permission}"
        );
        assert!(
            application_permissions.contains_key(permission),
            "application manifest must generate {permission}"
        );
    }
}
