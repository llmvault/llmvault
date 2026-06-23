use std::collections::HashMap;
use std::sync::Arc;

use anyhow::{anyhow, Result};
use async_trait::async_trait;
use domain::ImageGenerationConfig;
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};
use serde_json::{json, Value};

use crate::{schema_for, JsonTool, ToolDefinition};

#[derive(Debug, Deserialize, Serialize, JsonSchema)]
pub struct ImageGenerationArgs {
    #[serde(default)]
    pub prompt: String,
    #[serde(default)]
    pub description: String,
    #[serde(default)]
    pub reference_asset_ids: Vec<String>,
    #[serde(default)]
    pub aspect_ratio: String,
    #[serde(default, rename = "type")]
    pub image_type: String,
    #[serde(default)]
    pub count: Option<u32>,
}

pub struct ImageGenerationTool {
    name: String,
    description: String,
    config: ImageGenerationConfig,
    runtime_env: Arc<HashMap<String, String>>,
    session_id: String,
}

impl ImageGenerationTool {
    pub fn new(
        name: impl Into<String>,
        description: impl Into<String>,
        mode: impl Into<String>,
        mut config: ImageGenerationConfig,
        runtime_env: Arc<HashMap<String, String>>,
        session_id: impl Into<String>,
    ) -> Self {
        if config.mode.trim().is_empty() {
            config.mode = mode.into();
        }
        Self {
            name: name.into(),
            description: description.into(),
            config,
            runtime_env,
            session_id: session_id.into(),
        }
    }

    pub fn into_tool(self) -> Arc<dyn JsonTool> {
        Arc::new(self)
    }

    async fn execute(&self, args: Value) -> Result<Value> {
        let parsed: ImageGenerationArgs =
            serde_json::from_value(args).map_err(|e| anyhow!("invalid arguments: {e}"))?;
        let prompt = parsed.prompt.trim();
        let description = parsed.description.trim();
        if prompt.is_empty() && description.is_empty() {
            return Err(anyhow!("prompt or description is required"));
        }
        let max_refs = self.config.max_reference_assets.max(1) as usize;
        if parsed.reference_asset_ids.len() > max_refs {
            return Err(anyhow!(
                "reference_asset_ids contains {} assets; max is {}",
                parsed.reference_asset_ids.len(),
                max_refs
            ));
        }
        let max_count = self.config.max_count.max(1);
        if parsed.count.unwrap_or(1) > max_count {
            return Err(anyhow!("count must be between 1 and {max_count}"));
        }

        let endpoint = self.env_value(&self.config.endpoint_env, "endpoint_env")?;
        let bearer = self.env_value(&self.config.auth_env, "auth_env")?;
        let payload = json!({
            "mode": self.config.mode.clone(),
            "prompt": parsed.prompt,
            "description": parsed.description,
            "reference_asset_ids": parsed.reference_asset_ids,
            "aspect_ratio": parsed.aspect_ratio,
            "type": parsed.image_type,
            "count": parsed.count.unwrap_or(1),
            "_hivy_session_id": self.session_id,
        });

        let response = reqwest::Client::new()
            .post(endpoint)
            .bearer_auth(bearer)
            .json(&payload)
            .send()
            .await
            .map_err(|e| anyhow!("image generation request failed: {e}"))?;
        let status = response.status();
        let body = response
            .text()
            .await
            .map_err(|e| anyhow!("read image generation response: {e}"))?;
        if !status.is_success() {
            return Err(anyhow!(
                "image generation failed with status {}: {}",
                status.as_u16(),
                body
            ));
        }
        let value: Value = serde_json::from_str(&body)
            .map_err(|e| anyhow!("decode image generation response: {e}"))?;
        if !value.is_array() {
            return Err(anyhow!("image generation response must be an array"));
        }
        Ok(value)
    }

    fn env_value(&self, key: &str, label: &str) -> Result<String> {
        let key = key.trim();
        if key.is_empty() {
            return Err(anyhow!("image generation {label} is not configured"));
        }
        let value = self
            .runtime_env
            .get(key)
            .cloned()
            .or_else(|| std::env::var(key).ok())
            .unwrap_or_default();
        if value.trim().is_empty() {
            return Err(anyhow!("required environment variable {key} is not set"));
        }
        Ok(value)
    }
}

#[async_trait]
impl JsonTool for ImageGenerationTool {
    fn definition(&self) -> ToolDefinition {
        ToolDefinition {
            name: self.name.clone(),
            description: self.description.clone(),
            parameters: schema_for::<ImageGenerationArgs>(),
        }
    }

    async fn call(&self, args: Value) -> Result<Value> {
        self.execute(args).await
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[tokio::test]
    async fn rejects_empty_prompt_and_description() {
        let tool = ImageGenerationTool::new(
            "generate_image",
            "generate",
            "raster",
            ImageGenerationConfig::default(),
            Arc::new(HashMap::new()),
            "session-1",
        );

        let err = tool
            .execute(json!({"prompt": "", "description": ""}))
            .await
            .expect_err("empty prompt should fail");

        assert!(err.to_string().contains("prompt or description"));
    }
}
