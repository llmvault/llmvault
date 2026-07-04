#!/usr/bin/env bash
# preview.sh — run a preview of this app inside the CURRENT (builder) sandbox
# with the app's real platform environment, fetched over an authenticated side
# channel so no secret ever passes through the model context.
#
#   Usage:  scripts/preview.sh <APP_ID> [PORT]     (make preview APP_ID=… PORT=…)
#
# Requires the ambient sandbox env HIVY_DRIVE_UPLOAD_URL and
# HIVY_DRIVE_UPLOAD_BEARER (present in every agent sandbox). Fetches
#   GET ${HIVY_DRIVE_UPLOAD_URL%/drive}/apps/$APP_ID/preview-env?port=$PORT
# writes the returned env to a 0600 file, (re)starts dist/server under
# systemd when booted (hivy-app-preview.service) or as a supervised
# background process otherwise, waits for /healthz, and prints two URLs as
# the last lines of stdout: a raw one (for the agent's own health checks —
# do not share it) and, as the true LAST line, the share URL with ?app=
# appended (share ONLY that one with the user; it tells the Hivy frontend
# which app to frame). All progress goes to stderr.
#
# SECRETS: the env values live only in the HTTP response, the 0600 env file,
# and the server process environment. Never print them.
set -euo pipefail

log() { echo "$*" >&2; }
die() { log "preview: $*"; exit 1; }

APP_ID="${1:-}"
PORT="${2:-3000}"
[ -n "$APP_ID" ] || die "usage: scripts/preview.sh <APP_ID> [PORT] (make preview APP_ID=<uuid>)"
case "$PORT" in ''|*[!0-9]*) die "PORT must be a number, got '$PORT'";; esac

[ -n "${HIVY_DRIVE_UPLOAD_URL:-}" ] || die "HIVY_DRIVE_UPLOAD_URL is not set (are you inside an agent sandbox?)"
[ -n "${HIVY_DRIVE_UPLOAD_BEARER:-}" ] || die "HIVY_DRIVE_UPLOAD_BEARER is not set (are you inside an agent sandbox?)"
command -v python3 >/dev/null 2>&1 || die "python3 is required"

TEMPLATE_DIR="$(cd "$(dirname "$0")/.." && pwd)"
SERVER_BIN="$TEMPLATE_DIR/dist/server"
[ -x "$SERVER_BIN" ] || die "dist/server is missing — run 'make server' (or 'make bundle') first"

STATE_DIR="${HIVY_PREVIEW_STATE_DIR:-/workspace/.hivy}"
ENV_FILE="$STATE_DIR/app-preview-$APP_ID.env"
PID_FILE="$STATE_DIR/app-preview-$APP_ID.pid"
LOG_FILE="$STATE_DIR/app-preview-$APP_ID.log"
umask 077
mkdir -p "$STATE_DIR"

# ── Fetch the preview env + URL over the side channel ────────────────────────
ENDPOINT="${HIVY_DRIVE_UPLOAD_URL%/drive}/apps/$APP_ID/preview-env?port=$PORT"
BODY_FILE="$(mktemp "$STATE_DIR/.preview-resp-XXXXXX")"
trap 'rm -f "$BODY_FILE"' EXIT
log "preview: fetching environment for app $APP_ID (port $PORT)"
HTTP_STATUS="$(curl -sS -o "$BODY_FILE" -w '%{http_code}' \
  -H "Authorization: Bearer $HIVY_DRIVE_UPLOAD_BEARER" "$ENDPOINT")" || die "request to preview-env endpoint failed"
if [ "$HTTP_STATUS" != "200" ]; then
  log "preview: endpoint returned HTTP $HTTP_STATUS:"
  cat "$BODY_FILE" >&2
  echo >&2
  exit 1
fi

# Parse the JSON: write the env file (systemd-EnvironmentFile-compatible
# KEY="value", 0600) and emit the preview URL. Values never touch stdout/logs.
PREVIEW_URL="$(python3 - "$BODY_FILE" "$ENV_FILE" <<'PY'
import json, os, sys
with open(sys.argv[1]) as f:
    data = json.load(f)
env = data.get("env") or {}
lines = []
for name in sorted(env):
    value = str(env[name])
    if "\n" in value or "\r" in value:
        raise SystemExit(f"env var {name} contains a newline")
    escaped = value.replace("\\", "\\\\").replace('"', '\\"')
    lines.append(f'{name}="{escaped}"\n')
fd = os.open(sys.argv[2], os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
with os.fdopen(fd, "w") as f:
    f.writelines(lines)
url = (data.get("preview_url") or "").strip()
if not url:
    raise SystemExit("response is missing preview_url")
print(url)
PY
)" || die "failed to parse the preview-env response"
chmod 600 "$ENV_FILE"

# ── Start (or restart) the preview server ────────────────────────────────────
if [ -d /run/systemd/system ] && command -v systemctl >/dev/null 2>&1; then
  # systemd is PID1 (microsandbox): supervise via a unit so the preview
  # survives crashes and sandbox sleep/wake, like hivy-app.service.
  log "preview: (re)starting hivy-app-preview.service via systemd"
  cat > /etc/systemd/system/hivy-app-preview.service <<UNIT
[Unit]
Description=Hivy app preview ($APP_ID)
After=network.target

[Service]
ExecStart=$SERVER_BIN
WorkingDirectory=$TEMPLATE_DIR
EnvironmentFile=$ENV_FILE
Restart=on-failure
RestartSec=2

[Install]
WantedBy=multi-user.target
UNIT
  systemctl daemon-reload
  systemctl enable hivy-app-preview.service >/dev/null 2>&1 || true
  systemctl restart hivy-app-preview.service
else
  # No systemd: kill any previous preview, then run detached in its own
  # session with the env file loaded, stdout/stderr appended to the log.
  if [ -f "$PID_FILE" ]; then
    OLD_PID="$(cat "$PID_FILE" 2>/dev/null || true)"
    if [ -n "$OLD_PID" ] && kill -0 "$OLD_PID" 2>/dev/null; then
      log "preview: stopping previous preview (pid $OLD_PID)"
      kill -- -"$OLD_PID" 2>/dev/null || kill "$OLD_PID" 2>/dev/null || true
      for _ in $(seq 1 20); do kill -0 "$OLD_PID" 2>/dev/null || break; sleep 0.25; done
      kill -0 "$OLD_PID" 2>/dev/null && { kill -9 -- -"$OLD_PID" 2>/dev/null || kill -9 "$OLD_PID" 2>/dev/null || true; }
    fi
    rm -f "$PID_FILE"
  fi
  log "preview: starting dist/server in the background (log: $LOG_FILE)"
  # python launches the server: env parsed from the 0600 file (not shell-
  # sourced, so values with $, backticks, or quotes are safe), new session
  # (setsid), output appended to the log, pid recorded.
  python3 - "$ENV_FILE" "$SERVER_BIN" "$TEMPLATE_DIR" "$LOG_FILE" "$PID_FILE" <<'PY'
import os, subprocess, sys
env_file, server_bin, workdir, log_file, pid_file = sys.argv[1:6]
env = {
    "PATH": os.environ.get("PATH", "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"),
    "HOME": os.environ.get("HOME", "/workspace"),
}
with open(env_file) as f:
    for line in f:
        line = line.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        name, _, raw = line.partition("=")
        if raw.startswith('"') and raw.endswith('"') and len(raw) >= 2:
            raw = raw[1:-1].replace('\\"', '"').replace("\\\\", "\\")
        env[name] = raw
with open(log_file, "ab") as log:
    proc = subprocess.Popen(
        [server_bin], cwd=workdir, env=env,
        stdout=log, stderr=log, start_new_session=True,
    )
with open(pid_file, "w") as f:
    f.write(str(proc.pid))
PY
fi

# ── Wait for the app to answer /healthz on the requested port ────────────────
log "preview: waiting for http://127.0.0.1:$PORT/healthz"
HEALTHY=""
for _ in $(seq 1 60); do
  if curl -fsS -o /dev/null --max-time 2 "http://127.0.0.1:$PORT/healthz" 2>/dev/null; then
    HEALTHY=1
    break
  fi
  sleep 0.5
done
if [ -z "$HEALTHY" ]; then
  log "preview: server did not become healthy on port $PORT within 30s"
  if [ -f "$LOG_FILE" ]; then
    log "preview: last log lines:"
    tail -n 25 "$LOG_FILE" >&2 || true
  elif command -v journalctl >/dev/null 2>&1; then
    journalctl -u hivy-app-preview.service -n 25 --no-pager >&2 || true
  fi
  exit 1
fi

log "preview: healthy"
log "preview: raw preview_url (for your own health checks — do not share this line): $PREVIEW_URL"
log "preview: share ONLY the URL on the next line with the user"
# The LAST line of stdout is the share URL: the raw preview_url with
# ?app=$APP_ID appended, so the Hivy frontend knows which app to frame.
echo "${PREVIEW_URL}/?app=${APP_ID}"
