use std::collections::HashMap;
use std::sync::Arc;
use std::time::Duration;

use reqwest::Client;
use serde_json::json;
use tokio::sync::oneshot;
use tracing::warn;

#[derive(Clone)]
pub struct RuntimeActivityReporter {
    control_url: String,
    sandbox_id: String,
    probe_token: String,
    heartbeat_interval: Duration,
    client: Client,
}

pub struct RuntimeActivityGuard {
    stop: Option<oneshot::Sender<()>>,
}

impl RuntimeActivityReporter {
    pub fn from_env(env: &HashMap<String, String>) -> Option<Arc<Self>> {
        let control_url = env
            .get("HIVY_MICROSANDBOX_CONTROL_URL")
            .map(|value| value.trim().trim_end_matches('/').to_string())
            .filter(|value| !value.is_empty())?;
        let sandbox_id = env
            .get("HIVY_MICROSANDBOX_ID")
            .or_else(|| env.get("HIVY_SANDBOX_ID"))
            .map(|value| value.trim().to_string())
            .filter(|value| !value.is_empty())?;
        let probe_token = env
            .get("HIVY_INFRA_PROBE_TOKEN")
            .map(|value| value.trim().to_string())
            .filter(|value| !value.is_empty())?;
        Some(Arc::new(Self {
            control_url,
            sandbox_id,
            probe_token,
            heartbeat_interval: Duration::from_secs(30),
            client: Client::new(),
        }))
    }

    pub fn busy_period(self: &Arc<Self>) -> RuntimeActivityGuard {
        let (tx, mut rx) = oneshot::channel();
        let reporter = self.clone();
        tokio::spawn(async move {
            reporter.report(true).await;
            let mut interval = tokio::time::interval(reporter.heartbeat_interval);
            loop {
                tokio::select! {
                    _ = interval.tick() => reporter.report(true).await,
                    _ = &mut rx => {
                        reporter.report(false).await;
                        return;
                    }
                }
            }
        });
        RuntimeActivityGuard { stop: Some(tx) }
    }

    async fn report(&self, busy: bool) {
        let url = format!(
            "{}/v1/sandboxes/{}/activity",
            self.control_url, self.sandbox_id
        );
        let result = self
            .client
            .post(url)
            .bearer_auth(&self.probe_token)
            .json(&json!({
                "source": "runtime",
                "runtime_busy": busy,
            }))
            .send()
            .await;
        match result {
            Ok(response) if response.status().is_success() => {}
            Ok(response) => warn!(
                status = %response.status(),
                "runtime activity report rejected"
            ),
            Err(error) => warn!(%error, "runtime activity report failed"),
        }
    }
}

impl Drop for RuntimeActivityGuard {
    fn drop(&mut self) {
        if let Some(stop) = self.stop.take() {
            let _ = stop.send(());
        }
    }
}
