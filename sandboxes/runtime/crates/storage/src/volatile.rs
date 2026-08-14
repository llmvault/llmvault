use std::sync::Arc;

use async_trait::async_trait;
use tokio::sync::RwLock;

use crate::{ConfigRepo, ConfigSnapshot, Result};

/// Memory-only configuration storage for desktop runtimes.
///
/// Desktop configuration can contain short-lived proxy credentials and
/// decrypted MCP headers. Keeping it out of SQLite ensures those values leave
/// no plaintext copy on disk; the desktop client rehydrates the snapshot from
/// the authenticated Hivy API after each launch.
#[derive(Clone, Default)]
pub struct VolatileConfigRepo {
    snapshot: Arc<RwLock<Option<ConfigSnapshot>>>,
}

impl VolatileConfigRepo {
    pub fn new() -> Self {
        Self::default()
    }
}

#[async_trait]
impl ConfigRepo for VolatileConfigRepo {
    async fn load(&self) -> Result<Option<ConfigSnapshot>> {
        Ok(self.snapshot.read().await.clone())
    }

    async fn upsert(&self, snapshot: &ConfigSnapshot) -> Result<()> {
        *self.snapshot.write().await = Some(snapshot.clone());
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use domain::WorkspaceConfig;
    use std::collections::HashMap;

    #[tokio::test]
    async fn stores_snapshot_only_for_the_process_lifetime() {
        let repo = VolatileConfigRepo::new();
        let snapshot = ConfigSnapshot {
            definition: serde_json::from_value(serde_json::json!({
                "agent": { "name": "Test" },
                "model": {
                    "provider": "openai_compatible",
                    "base_url": "https://example.test/v1",
                    "model_id": "test-model",
                    "api_key_env": "TEST_API_KEY"
                }
            }))
            .unwrap(),
            runtime_env: HashMap::from([("SECRET".to_string(), "value".to_string())]),
            workspace: WorkspaceConfig::default(),
        };

        repo.upsert(&snapshot).await.unwrap();
        assert_eq!(
            repo.load().await.unwrap().unwrap().runtime_env["SECRET"],
            "value"
        );
        assert!(VolatileConfigRepo::new().load().await.unwrap().is_none());
    }
}
