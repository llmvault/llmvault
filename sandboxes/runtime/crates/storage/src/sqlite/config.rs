use std::sync::Arc;

use async_trait::async_trait;
use sqlx::SqlitePool;

use crate::repos::{ConfigRepo, ConfigSnapshot, Result};

use super::{SqliteStore, SqliteWriteGateway};

pub struct SqliteConfigRepo {
    pool: Arc<SqlitePool>,
    writer: Arc<SqliteWriteGateway>,
}

impl SqliteConfigRepo {
    pub fn new(store: &SqliteStore) -> Self {
        Self {
            pool: store.read_pool(),
            writer: store.writer(),
        }
    }
}

#[async_trait]
impl ConfigRepo for SqliteConfigRepo {
    async fn load(&self) -> Result<Option<ConfigSnapshot>> {
        let row: Option<(String, String, String)> = sqlx::query_as(
            "SELECT definition_json, runtime_env_json, workspace_json FROM agent_config WHERE id = 1",
        )
        .fetch_optional(self.pool.as_ref())
        .await?;
        match row {
            Some((definition_json, runtime_env_json, workspace_json)) => Ok(Some(ConfigSnapshot {
                definition: serde_json::from_str(&definition_json)?,
                runtime_env: serde_json::from_str(&runtime_env_json)?,
                workspace: serde_json::from_str(&workspace_json)?,
            })),
            None => Ok(None),
        }
    }

    async fn upsert(&self, snapshot: &ConfigSnapshot) -> Result<()> {
        self.writer.upsert_config(snapshot).await
    }
}
