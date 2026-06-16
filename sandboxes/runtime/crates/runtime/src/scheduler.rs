use std::sync::Arc;
use std::time::Duration;

use agent::rig_tool_registry::{emit_schedule_event, schedule_run_key, ScheduleRunPayload};
use chrono::Utc;
use domain::event_types;
use domain::{CronJob, InboundEvent, SessionId};
use outbound::OutboundEmitter;
use storage::CronJobRepo;
use tokio::sync::mpsc;
use tracing::{error, info};

const POLL_INTERVAL_SECONDS: u64 = 5;
const RUNNING_JOB_LEASE_SECONDS: i64 = 60 * 60;
const STALE_GRACE_MULTIPLIER: f64 = 0.5;

pub struct CronScheduler {
    repo: Arc<dyn CronJobRepo>,
    inbound_sink: mpsc::Sender<InboundEvent>,
    emitter: Option<Arc<OutboundEmitter>>,
}

impl CronScheduler {
    pub fn new(
        repo: Arc<dyn CronJobRepo>,
        inbound_sink: mpsc::Sender<InboundEvent>,
        emitter: Option<Arc<OutboundEmitter>>,
    ) -> Self {
        Self {
            repo,
            inbound_sink,
            emitter,
        }
    }

    pub async fn run(self) {
        let mut interval = tokio::time::interval(Duration::from_secs(POLL_INTERVAL_SECONDS));
        loop {
            interval.tick().await;
            self.recover_stale_running_jobs().await;
            let due = match self.repo.list_due().await {
                Ok(jobs) => jobs,
                Err(e) => {
                    error!(error = %e, "cron scheduler: list_due failed");
                    continue;
                }
            };
            for job in due {
                if let Some(final_job) = self.fast_forward_if_stale(job).await {
                    self.dispatch_job(final_job).await;
                }
            }
        }
    }

    async fn recover_stale_running_jobs(&self) {
        let before = Utc::now() - chrono::Duration::seconds(RUNNING_JOB_LEASE_SECONDS);
        match self.repo.reset_stale_running(before).await {
            Ok(0) => {}
            Ok(count) => {
                info!(
                    recovered = count,
                    lease_seconds = RUNNING_JOB_LEASE_SECONDS,
                    "cron: recovered stale running jobs"
                );
            }
            Err(e) => {
                error!(error = %e, "cron: failed to recover stale running jobs");
            }
        }
    }

    async fn fast_forward_if_stale(&self, job: CronJob) -> Option<CronJob> {
        let is_recurring = job.interval_seconds.map(|v| v > 0).unwrap_or(false);
        if !is_recurring {
            return Some(job);
        }
        let interval = job.interval_seconds?;
        let stale_threshold = (interval as f64 * STALE_GRACE_MULTIPLIER).max(120.0) as i64;
        let now = Utc::now();
        let lag = now.signed_duration_since(job.next_run_at).num_seconds();
        if lag > stale_threshold * 2 {
            // The job has been paused so long that running it now would be a
            // spurious "catch-up" fire. Skip this occurrence, but advance
            // next_run_at to the next future occurrence so the job keeps
            // running on schedule instead of being re-fetched and re-skipped
            // forever on every poll.
            let next = next_future_occurrence(job.next_run_at, interval, now);
            info!(
                job_id = %job.id,
                lag_seconds = lag,
                next_run_at = %next,
                "cron: fast-forwarding stale recurring job"
            );
            if let Err(e) = self.repo.update_next_run(&job.id, next).await {
                error!(
                    job_id = %job.id,
                    error = %e,
                    "cron: failed to advance next_run for stale job"
                );
            }
            None
        } else {
            Some(job)
        }
    }

    async fn dispatch_job(&self, job: CronJob) {
        let scheduled_at = job.next_run_at;
        let started_at = Utc::now();
        let run_key = schedule_run_key(&job.id, scheduled_at);
        let is_wake = job.session_continuation_id.is_some();
        if job.interval_seconds.is_none() && !is_wake {
            error!(
                job_id = %job.id,
                cron_expression = ?job.cron_expression,
                "cron: cron_expression schedules are not supported by this scheduler; pausing job"
            );
            let _ = self
                .repo
                .record_run(
                    &job.id,
                    started_at,
                    "error",
                    Some("cron_expression schedules are not supported by this runtime scheduler"),
                )
                .await;
            let _ = self
                .repo
                .set_state(&job.id, domain::cron::CronJobState::Paused)
                .await;
            return;
        }
        let is_recurring = job.interval_seconds.map(|v| v > 0).unwrap_or(false);
        let is_one_shot = job.interval_seconds == Some(0);
        let requires_claim = is_one_shot || is_wake;

        if requires_claim {
            match self
                .repo
                .claim_due_run(&job.id, Utc::now(), started_at)
                .await
            {
                Ok(true) => {}
                Ok(false) => {
                    info!(
                        job_id = %job.id,
                        "cron: skipped scheduled task because it was already claimed"
                    );
                    return;
                }
                Err(e) => {
                    error!(job_id = %job.id, error = %e, "cron: failed to claim due run");
                    return;
                }
            }
        }

        if is_recurring && !is_wake {
            let next = Utc::now() + chrono::Duration::seconds(job.interval_seconds.unwrap() as i64);
            if let Err(e) = self.repo.update_next_run(&job.id, next).await {
                error!(job_id = %job.id, error = %e, "cron: failed to advance next_run");
                return;
            }
        }

        let session_id = SessionId::from(
            job.session_continuation_id
                .clone()
                .unwrap_or_else(|| format!("{}-cron-{}", job.channel, job.id)),
        );
        let envelope_id = format!("cron-{}", Utc::now().timestamp_millis());

        if !requires_claim {
            if let Err(e) = self
                .repo
                .record_run(&job.id, started_at, "running", None)
                .await
            {
                error!(job_id = %job.id, error = %e, "cron: failed to record run start");
            }
        }
        let running_job = self
            .repo
            .get(&job.id)
            .await
            .ok()
            .flatten()
            .unwrap_or_else(|| {
                let mut job = job.clone();
                job.last_run_at = Some(started_at);
                job.last_status = Some("running".to_string());
                job
            });
        emit_schedule_event(
            self.emitter.clone(),
            event_types::SCHEDULE_RUN_STARTED,
            &running_job,
            &session_id,
            "scheduler",
            Some(ScheduleRunPayload {
                run_key: run_key.clone(),
                scheduled_at,
                started_at: Some(started_at),
                completed_at: None,
                duration_ms: None,
                error: None,
            }),
        )
        .await;

        let mut raw = scheduled_job_raw_metadata(&job);

        // A scheduled run is only *enqueued* here — the turn has not run yet.
        // Carry the run-lifecycle context so the turn handler can mark the run
        // completed/failed (and delete one-shot/wake jobs, advance repeat
        // counts) *after* the turn actually executes, instead of the scheduler
        // reporting success the moment it hands off.
        if let Some(obj) = raw.as_object_mut() {
            obj.insert(
                "schedule_run_key".to_string(),
                serde_json::json!(run_key.clone()),
            );
            obj.insert(
                "schedule_scheduled_at".to_string(),
                serde_json::json!(scheduled_at),
            );
            obj.insert(
                "schedule_started_at".to_string(),
                serde_json::json!(started_at),
            );
            obj.insert(
                "schedule_is_one_shot".to_string(),
                serde_json::json!(is_one_shot),
            );
            obj.insert("schedule_is_wake".to_string(), serde_json::json!(is_wake));
        }

        let inbound = InboundEvent {
            envelope_id: envelope_id.clone(),
            session_id: session_id.clone(),
            user: "cron".to_string(),
            user_display_name: Some("Scheduler".to_string()),
            text: job.task_prompt.clone(),
            attachments: Vec::new(),
            dynamic_context: Vec::new(),
            raw,
            is_direct_message: false,
            is_directly_addressed: true,
            link_previews: Vec::new(),
            agent_definition: None,
        };

        info!(
            job_id = %job.id,
            session = %session_id,
            channel = %job.channel,
            "cron: dispatching scheduled task"
        );

        if let Err(e) = self.inbound_sink.send(inbound).await {
            error!(job_id = %job.id, error = %e, "cron: failed to dispatch");
            let failed_at = Utc::now();
            if requires_claim {
                let _ = self
                    .repo
                    .set_state(&job.id, domain::cron::CronJobState::Active)
                    .await;
            }
            let _ = self
                .repo
                .record_run(&job.id, failed_at, "error", Some(&e.to_string()))
                .await;
            let failed_job = self
                .repo
                .get(&job.id)
                .await
                .ok()
                .flatten()
                .unwrap_or_else(|| {
                    let mut job = running_job.clone();
                    job.last_run_at = Some(failed_at);
                    job.last_status = Some("error".to_string());
                    job.last_error = Some(e.to_string());
                    job
                });
            emit_schedule_event(
                self.emitter.clone(),
                event_types::SCHEDULE_RUN_FAILED,
                &failed_job,
                &session_id,
                "scheduler",
                Some(ScheduleRunPayload {
                    run_key,
                    scheduled_at,
                    started_at: Some(started_at),
                    completed_at: Some(failed_at),
                    duration_ms: Some((failed_at - started_at).num_milliseconds()),
                    error: Some(e.to_string()),
                }),
            )
            .await;
            return;
        }

        // The run is now enqueued but NOT complete. The turn handler emits
        // SCHEDULE_RUN_COMPLETED/FAILED, deletes one-shot/wake jobs, and advances
        // repeat counts once the turn has actually executed (see
        // `complete_scheduled_run` in the handler).
        let _ = running_job;
    }
}

/// Compute the next occurrence strictly after `now` for an interval-based
/// recurring job, preserving the original schedule's phase relative to
/// `next_run_at`. Steps forward by whole intervals; falls back to `now +
/// interval` if the interval is non-positive or arithmetic overflows.
fn next_future_occurrence(
    next_run_at: chrono::DateTime<Utc>,
    interval_seconds: u64,
    now: chrono::DateTime<Utc>,
) -> chrono::DateTime<Utc> {
    let interval = interval_seconds as i64;
    if interval <= 0 {
        return now + chrono::Duration::seconds(1);
    }
    if next_run_at > now {
        return next_run_at;
    }
    let lag = now.signed_duration_since(next_run_at).num_seconds();
    // Number of whole intervals elapsed; +1 lands strictly in the future.
    let steps = lag / interval + 1;
    match next_run_at.checked_add_signed(chrono::Duration::seconds(steps * interval)) {
        Some(next) if next > now => next,
        _ => now + chrono::Duration::seconds(interval),
    }
}

fn scheduled_job_raw_metadata(job: &CronJob) -> serde_json::Value {
    if job.session_continuation_id.is_some() {
        return serde_json::json!({
            "source": "wake",
            "job_kind": "wake",
            "job_id": job.id,
        });
    }
    serde_json::json!({
        "source": "cron",
        "job_kind": "cron",
        "job_id": job.id,
    })
}

#[cfg(test)]
mod tests {
    use std::sync::atomic::{AtomicI64, Ordering};
    use std::sync::{Arc, Mutex};

    use async_trait::async_trait;
    use chrono::{DateTime, Duration as ChronoDuration, Utc};
    use domain::cron::CronJobState;
    use domain::CronJob;
    use storage::repos::{CronJobRepo, Result as StorageResult};
    use tokio::sync::mpsc;

    use super::{
        next_future_occurrence, scheduled_job_raw_metadata, CronScheduler,
        RUNNING_JOB_LEASE_SECONDS,
    };

    fn test_job(id: &str) -> CronJob {
        CronJob {
            id: id.to_string(),
            description: "test".to_string(),
            channel: "C123".to_string(),
            task_prompt: "do work".to_string(),
            cron_expression: None,
            interval_seconds: Some(0),
            repeat_count: None,
            repeat_completed: 0,
            state: CronJobState::Active,
            next_run_at: Utc::now(),
            last_run_at: None,
            last_status: None,
            last_error: None,
            session_continuation_id: None,
            created_at: Utc::now(),
            created_by_session: "parent-session".to_string(),
        }
    }

    #[test]
    fn wake_metadata_is_not_subagent_task_metadata() {
        let mut job = test_job("wake-1");
        job.session_continuation_id = Some("parent-session".to_string());

        let raw = scheduled_job_raw_metadata(&job);

        assert_eq!(raw["source"], "wake");
        assert_eq!(raw["job_kind"], "wake");
        assert_eq!(raw["job_id"], "wake-1");
        assert!(raw.get("parent_session_id").is_none());
        assert!(raw.get("subagent_task_goal").is_none());
        assert!(raw.get("agent_name").is_none());
    }

    /// Minimal repo that records the last `update_next_run` it received and is a
    /// no-op for everything else, so we can assert the stale branch advances the
    /// schedule instead of dropping it.
    struct RecordingCronRepo {
        update_next_run_calls: AtomicI64,
        last_next_run: std::sync::Mutex<Option<DateTime<Utc>>>,
        reset_stale_running_calls: AtomicI64,
        last_reset_before: std::sync::Mutex<Option<DateTime<Utc>>>,
    }

    impl RecordingCronRepo {
        fn new() -> Self {
            Self {
                update_next_run_calls: AtomicI64::new(0),
                last_next_run: std::sync::Mutex::new(None),
                reset_stale_running_calls: AtomicI64::new(0),
                last_reset_before: std::sync::Mutex::new(None),
            }
        }
    }

    #[async_trait]
    impl CronJobRepo for RecordingCronRepo {
        async fn create(&self, _job: &CronJob) -> StorageResult<()> {
            Ok(())
        }
        async fn get(&self, _id: &str) -> StorageResult<Option<CronJob>> {
            Ok(None)
        }
        async fn list_all(&self) -> StorageResult<Vec<CronJob>> {
            Ok(Vec::new())
        }
        async fn list_due(&self) -> StorageResult<Vec<CronJob>> {
            Ok(Vec::new())
        }
        async fn update_prompt(&self, _id: &str, _task_prompt: String) -> StorageResult<()> {
            Ok(())
        }
        async fn update_interval(&self, _id: &str, _interval_seconds: u64) -> StorageResult<()> {
            Ok(())
        }
        async fn update_next_run(
            &self,
            _id: &str,
            next_run_at: DateTime<Utc>,
        ) -> StorageResult<()> {
            self.update_next_run_calls.fetch_add(1, Ordering::SeqCst);
            *self.last_next_run.lock().unwrap() = Some(next_run_at);
            Ok(())
        }
        async fn set_state(&self, _id: &str, _state: CronJobState) -> StorageResult<()> {
            Ok(())
        }
        async fn claim_due_run(
            &self,
            _id: &str,
            _now: DateTime<Utc>,
            _started_at: DateTime<Utc>,
        ) -> StorageResult<bool> {
            Ok(true)
        }
        async fn reset_stale_running(&self, before: DateTime<Utc>) -> StorageResult<u64> {
            self.reset_stale_running_calls
                .fetch_add(1, Ordering::SeqCst);
            *self.last_reset_before.lock().unwrap() = Some(before);
            Ok(2)
        }
        async fn record_run(
            &self,
            _id: &str,
            _run_at: DateTime<Utc>,
            _status: &str,
            _error: Option<&str>,
        ) -> StorageResult<()> {
            Ok(())
        }
        async fn increment_repeat(&self, _id: &str) -> StorageResult<()> {
            Ok(())
        }
        async fn delete(&self, _id: &str) -> StorageResult<()> {
            Ok(())
        }
    }

    fn scheduler_with_repo(repo: Arc<RecordingCronRepo>) -> CronScheduler {
        let (tx, _rx) = mpsc::channel(8);
        CronScheduler::new(repo, tx, None)
    }

    #[test]
    fn next_future_occurrence_advances_strictly_into_future_preserving_phase() {
        let interval = 3600u64; // 1 hour
        let next_run_at = "2020-01-01T00:00:00Z".parse::<DateTime<Utc>>().unwrap();
        // Six and a half hours after the scheduled run.
        let now = next_run_at + ChronoDuration::seconds(6 * 3600 + 1800);

        let next = next_future_occurrence(next_run_at, interval, now);

        assert!(next > now, "next occurrence must be strictly in the future");
        // Phase is preserved: result is on an interval boundary from next_run_at.
        let delta = next.signed_duration_since(next_run_at).num_seconds();
        assert_eq!(delta % interval as i64, 0);
        // The first boundary strictly after `now` is +7h.
        assert_eq!(next, next_run_at + ChronoDuration::seconds(7 * 3600));
    }

    #[test]
    fn next_future_occurrence_keeps_future_next_run_unchanged() {
        let interval = 600u64;
        let now = "2020-01-01T00:00:00Z".parse::<DateTime<Utc>>().unwrap();
        let next_run_at = now + ChronoDuration::seconds(120);
        assert_eq!(
            next_future_occurrence(next_run_at, interval, now),
            next_run_at
        );
    }

    #[tokio::test]
    async fn long_paused_recurring_job_advances_next_run_and_is_skipped() {
        let repo = Arc::new(RecordingCronRepo::new());
        let scheduler = scheduler_with_repo(repo.clone());

        // A recurring job (10-minute interval) whose next_run_at is two days in
        // the past — a long sandbox pause. lag far exceeds 2x the stale
        // threshold, so the occurrence must be skipped but rescheduled.
        let interval = 600u64;
        let mut job = test_job("recurring-1");
        job.interval_seconds = Some(interval);
        job.next_run_at = Utc::now() - ChronoDuration::days(2);

        let result = scheduler.fast_forward_if_stale(job).await;

        assert!(
            result.is_none(),
            "stale recurring occurrence should be skipped, not dispatched"
        );
        assert_eq!(
            repo.update_next_run_calls.load(Ordering::SeqCst),
            1,
            "stale job must persist a new next_run_at"
        );
        let persisted = repo
            .last_next_run
            .lock()
            .unwrap()
            .expect("next_run_at must be written");
        assert!(
            persisted > Utc::now(),
            "persisted next_run_at must be in the future so the job runs again"
        );
    }

    #[tokio::test]
    async fn fresh_recurring_job_is_dispatched_without_rescheduling() {
        let repo = Arc::new(RecordingCronRepo::new());
        let scheduler = scheduler_with_repo(repo.clone());

        let mut job = test_job("recurring-2");
        job.interval_seconds = Some(600);
        // Due only a few seconds ago — within the stale grace window.
        job.next_run_at = Utc::now() - ChronoDuration::seconds(5);

        let result = scheduler.fast_forward_if_stale(job).await;

        assert!(result.is_some(), "fresh due job should be dispatched");
        assert_eq!(
            repo.update_next_run_calls.load(Ordering::SeqCst),
            0,
            "fresh jobs are advanced by dispatch_job, not fast_forward_if_stale"
        );
    }

    #[tokio::test]
    async fn scheduler_sweeps_stale_running_jobs_with_lease_cutoff() {
        let repo = Arc::new(RecordingCronRepo::new());
        let scheduler = scheduler_with_repo(repo.clone());

        scheduler.recover_stale_running_jobs().await;

        assert_eq!(repo.reset_stale_running_calls.load(Ordering::SeqCst), 1);
        let before = repo
            .last_reset_before
            .lock()
            .unwrap()
            .expect("recovery cutoff should be passed to repo");
        let expected = Utc::now() - ChronoDuration::seconds(RUNNING_JOB_LEASE_SECONDS);
        assert!((before - expected).num_seconds().abs() < 2);
    }

    struct ClaimingCronRepo {
        job: Mutex<CronJob>,
    }

    impl ClaimingCronRepo {
        fn new(job: CronJob) -> Self {
            Self {
                job: Mutex::new(job),
            }
        }

        fn state(&self) -> CronJobState {
            self.job.lock().unwrap().state
        }

        fn last_status(&self) -> Option<String> {
            self.job.lock().unwrap().last_status.clone()
        }

        fn last_error(&self) -> Option<String> {
            self.job.lock().unwrap().last_error.clone()
        }
    }

    #[async_trait]
    impl CronJobRepo for ClaimingCronRepo {
        async fn create(&self, _job: &CronJob) -> StorageResult<()> {
            Ok(())
        }
        async fn get(&self, _id: &str) -> StorageResult<Option<CronJob>> {
            Ok(Some(self.job.lock().unwrap().clone()))
        }
        async fn list_all(&self) -> StorageResult<Vec<CronJob>> {
            Ok(vec![self.job.lock().unwrap().clone()])
        }
        async fn list_due(&self) -> StorageResult<Vec<CronJob>> {
            let job = self.job.lock().unwrap();
            if job.state == CronJobState::Active && job.next_run_at <= Utc::now() {
                Ok(vec![job.clone()])
            } else {
                Ok(Vec::new())
            }
        }
        async fn update_prompt(&self, _id: &str, _task_prompt: String) -> StorageResult<()> {
            Ok(())
        }
        async fn update_interval(&self, _id: &str, _interval_seconds: u64) -> StorageResult<()> {
            Ok(())
        }
        async fn update_next_run(
            &self,
            _id: &str,
            next_run_at: DateTime<Utc>,
        ) -> StorageResult<()> {
            self.job.lock().unwrap().next_run_at = next_run_at;
            Ok(())
        }
        async fn set_state(&self, _id: &str, state: CronJobState) -> StorageResult<()> {
            self.job.lock().unwrap().state = state;
            Ok(())
        }
        async fn claim_due_run(
            &self,
            _id: &str,
            now: DateTime<Utc>,
            started_at: DateTime<Utc>,
        ) -> StorageResult<bool> {
            let mut job = self.job.lock().unwrap();
            if job.state != CronJobState::Active || job.next_run_at > now {
                return Ok(false);
            }
            job.state = CronJobState::Running;
            job.last_run_at = Some(started_at);
            job.last_status = Some("running".to_string());
            Ok(true)
        }
        async fn record_run(
            &self,
            _id: &str,
            run_at: DateTime<Utc>,
            status: &str,
            error: Option<&str>,
        ) -> StorageResult<()> {
            let mut job = self.job.lock().unwrap();
            job.last_run_at = Some(run_at);
            job.last_status = Some(status.to_string());
            job.last_error = error.map(ToString::to_string);
            Ok(())
        }
        async fn increment_repeat(&self, _id: &str) -> StorageResult<()> {
            Ok(())
        }
        async fn delete(&self, _id: &str) -> StorageResult<()> {
            Ok(())
        }
    }

    #[tokio::test]
    async fn wake_dispatch_claims_job_before_enqueue_so_stale_due_clone_is_skipped() {
        let mut job = test_job("wake-claim-dispatch");
        job.interval_seconds = None;
        job.next_run_at = Utc::now() - ChronoDuration::seconds(1);
        job.session_continuation_id = Some("session-continue".to_string());
        let stale_due_clone = job.clone();
        let repo = Arc::new(ClaimingCronRepo::new(job));
        let (tx, mut rx) = mpsc::channel(8);
        let scheduler = CronScheduler::new(repo.clone(), tx, None);

        scheduler.dispatch_job(stale_due_clone.clone()).await;

        let dispatched = rx.try_recv().expect("wake should dispatch once");
        assert_eq!(dispatched.session_id.as_str(), "session-continue");
        assert_eq!(repo.state(), CronJobState::Running);

        scheduler.dispatch_job(stale_due_clone).await;

        assert!(
            rx.try_recv().is_err(),
            "claimed wake must not dispatch again from a stale due row"
        );
    }

    #[tokio::test]
    async fn wake_dispatch_send_failure_resets_claim_and_records_error() {
        let mut job = test_job("wake-send-failure");
        job.interval_seconds = None;
        job.next_run_at = Utc::now() - ChronoDuration::seconds(1);
        job.session_continuation_id = Some("session-continue".to_string());
        let repo = Arc::new(ClaimingCronRepo::new(job.clone()));
        let (tx, rx) = mpsc::channel(1);
        drop(rx);
        let scheduler = CronScheduler::new(repo.clone(), tx, None);

        scheduler.dispatch_job(job).await;

        assert_eq!(repo.state(), CronJobState::Active);
        assert_eq!(repo.last_status().as_deref(), Some("error"));
        assert!(
            repo.last_error()
                .as_deref()
                .unwrap_or_default()
                .contains("channel closed"),
            "send error should be persisted for operators"
        );
    }

    #[tokio::test]
    async fn cron_expression_without_interval_is_paused_instead_of_repeated() {
        let mut job = test_job("cron-expression-only");
        job.interval_seconds = None;
        job.cron_expression = Some("0 9 * * *".to_string());
        job.next_run_at = Utc::now() - ChronoDuration::seconds(1);
        let repo = Arc::new(ClaimingCronRepo::new(job.clone()));
        let (tx, mut rx) = mpsc::channel(1);
        let scheduler = CronScheduler::new(repo.clone(), tx, None);

        scheduler.dispatch_job(job).await;

        assert!(rx.try_recv().is_err(), "unsupported cron must not dispatch");
        assert_eq!(repo.state(), CronJobState::Paused);
        assert_eq!(repo.last_status().as_deref(), Some("error"));
        assert!(repo
            .last_error()
            .as_deref()
            .unwrap_or_default()
            .contains("cron_expression schedules are not supported"));
    }
}
