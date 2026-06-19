use dashmap::DashMap;
use std::collections::HashMap;
use std::path::PathBuf;
use std::sync::{Arc, Mutex};
use std::time::Duration;

use tokio::io::AsyncReadExt;
use tokio::process::Command;

pub struct BackgroundProcessStatus {
    pub running: bool,
    pub exit_code: Option<i32>,
    pub output: String,
    pub next_cursor: usize,
    pub truncated: bool,
}

struct BackgroundProcess {
    status: Arc<Mutex<BackgroundProcessState>>,
    started_at: std::time::Instant,
    session_id: Option<String>,
    pid: Option<u32>,
}

pub struct ProcessRegistry {
    processes: DashMap<String, BackgroundProcess>,
}

struct BackgroundProcessState {
    running: bool,
    exit_code: Option<i32>,
    output: Vec<u8>,
    base_cursor: usize,
    max_output_bytes: usize,
    truncated: bool,
}

impl ProcessRegistry {
    pub fn new() -> Self {
        Self {
            processes: DashMap::new(),
        }
    }

    pub fn spawn(
        &self,
        command: &str,
        workdir: PathBuf,
        env: HashMap<String, String>,
        timeout_seconds: u64,
        max_output_bytes: u64,
        session_id: Option<String>,
    ) -> Result<String, String> {
        let process_id = format!("bash-{}", uuid::Uuid::new_v4());
        let status = Arc::new(Mutex::new(BackgroundProcessState {
            running: true,
            exit_code: None,
            output: Vec::new(),
            base_cursor: 0,
            max_output_bytes: max_output_bytes.clamp(4096, 16 * 1024 * 1024) as usize,
            truncated: false,
        }));

        let status_clone = status.clone();
        let command_owned = command.to_string();
        let (child, pid) = spawn_background_child(&command_owned, &workdir, &env)?;

        self.processes.insert(
            process_id.clone(),
            BackgroundProcess {
                status: status_clone,
                started_at: std::time::Instant::now(),
                session_id,
                pid,
            },
        );

        tokio::spawn(run_background_process(child, status, timeout_seconds));
        Ok(process_id)
    }

    pub fn status(
        &self,
        process_id: &str,
        cursor: Option<usize>,
    ) -> Option<BackgroundProcessStatus> {
        let entry = self.processes.get(process_id)?;
        if entry.started_at.elapsed() > Duration::from_secs(1800) {
            drop(entry);
            self.processes.remove(process_id);
            return None;
        }
        let inner = entry.status.lock().unwrap();
        let start = cursor.unwrap_or(inner.base_cursor).max(inner.base_cursor);
        let relative = start
            .saturating_sub(inner.base_cursor)
            .min(inner.output.len());
        let output = String::from_utf8_lossy(&inner.output[relative..]).to_string();
        Some(BackgroundProcessStatus {
            running: inner.running,
            exit_code: inner.exit_code,
            output,
            next_cursor: inner.base_cursor + inner.output.len(),
            truncated: inner.truncated || cursor.is_some_and(|cursor| cursor < inner.base_cursor),
        })
    }

    pub fn cancel(&self, process_id: &str) -> bool {
        let Some(entry) = self.processes.get(process_id) else {
            return false;
        };
        if let Some(pid) = entry.pid {
            kill_process_group(pid);
        }
        if let Ok(mut status) = entry.status.lock() {
            status.running = false;
            status.exit_code = Some(-1);
        }
        true
    }

    pub fn running_for_session(&self, session_id: &str) -> usize {
        self.processes
            .iter()
            .filter(|entry| {
                entry.session_id.as_deref() == Some(session_id)
                    && entry.status.lock().is_ok_and(|status| status.running)
            })
            .count()
    }
}

impl Default for ProcessRegistry {
    fn default() -> Self {
        Self::new()
    }
}

fn spawn_background_child(
    command: &str,
    workdir: &PathBuf,
    env: &HashMap<String, String>,
) -> Result<(tokio::process::Child, Option<u32>), String> {
    let mut cmd = shell_command(command);
    cmd.current_dir(workdir);
    cmd.kill_on_drop(true);
    for (k, v) in env {
        cmd.env(k, v);
    }
    cmd.stdout(std::process::Stdio::piped());
    cmd.stderr(std::process::Stdio::piped());
    #[cfg(unix)]
    {
        cmd.process_group(0);
    }
    let child = cmd
        .spawn()
        .map_err(|error| format!("spawn error: {error}"))?;
    let pid = child.id();
    Ok((child, pid))
}

fn shell_command(command: &str) -> Command {
    if cfg!(target_os = "windows") {
        let mut c = Command::new("cmd");
        c.arg("/C").arg(command);
        c
    } else {
        let mut c = Command::new("bash");
        c.arg("-lc").arg(command);
        c
    }
}

async fn run_background_process(
    mut child: tokio::process::Child,
    status: Arc<Mutex<BackgroundProcessState>>,
    timeout_seconds: u64,
) {
    let pid = child.id();
    let stdout = child.stdout.take();
    let stderr = child.stderr.take();

    if let Some(stdout) = stdout {
        tokio::spawn(copy_process_output(stdout, status.clone()));
    }

    if let Some(stderr) = stderr {
        tokio::spawn(copy_process_output(stderr, status.clone()));
    }

    let timeout = tokio::time::sleep(Duration::from_secs(timeout_seconds));
    tokio::pin!(timeout);
    let exit_status = tokio::select! {
        r = child.wait() => r,
        _ = &mut timeout => {
            if let Some(pid) = pid {
                kill_process_group(pid);
            }
            let _ = child.start_kill();
            child.wait().await
        }
    };

    let mut s = status.lock().unwrap();
    s.running = false;
    s.exit_code = exit_status.ok().and_then(|e| e.code());
}

async fn copy_process_output<R>(mut reader: R, status: Arc<Mutex<BackgroundProcessState>>)
where
    R: tokio::io::AsyncRead + Unpin,
{
    let mut chunk = [0u8; 4096];
    loop {
        match reader.read(&mut chunk).await {
            Ok(0) => break,
            Ok(n) => append_output(&status, &chunk[..n]),
            Err(_) => break,
        }
    }
}

fn append_output(status: &Arc<Mutex<BackgroundProcessState>>, bytes: &[u8]) {
    let mut state = status.lock().unwrap();
    state.output.extend_from_slice(bytes);
    if state.output.len() > state.max_output_bytes {
        let drop = state.output.len() - state.max_output_bytes;
        state.output.drain(..drop);
        state.base_cursor += drop;
        state.truncated = true;
    }
}

fn kill_process_group(pid: u32) {
    #[cfg(unix)]
    {
        let _ = std::process::Command::new("kill")
            .arg("-TERM")
            .arg(format!("-{pid}"))
            .status();
    }
    #[cfg(not(unix))]
    {
        let _ = pid;
    }
}
