fn main() {
    tauri_build::try_build(tauri_build::Attributes::new().app_manifest(
        tauri_build::AppManifest::new().commands(&[
            "desktop_cloud_url",
            "desktop_info",
            "runtime_request",
            "runtime_session_stream",
        ]),
    ))
    .expect("build Hivy desktop application");
}
