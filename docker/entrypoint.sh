#!/bin/sh
# Entrypoint for the Hivy API container.
#
# Responsibilities
# ----------------
# 1. Start nginx (master runs as root to bind :80; workers drop to the
#    "nginx" system user, as configured by "user nginx;" in nginx.conf).
# 2. Start the Go API binary as the non-root "hivy" user via su-exec.
# 3. Exit the container — with the first-dying process's exit code — when
#    either child dies, so Railway / Docker restarts and alerts on failures.
# 4. Forward SIGTERM/SIGINT to both children so nginx drains in-flight
#    requests gracefully (SIGQUIT) before the container is torn down.

set -e

# ── start nginx ───────────────────────────────────────────────────────────────
# "daemon off;" keeps the master process in the foreground so we can track it
# as a background job.  nginx master must run as root to bind port 80; worker
# processes automatically drop to the "nginx" user (set in nginx.conf).
nginx -g "daemon off;" &
NGINX_PID=$!

# ── start the API binary ──────────────────────────────────────────────────────
# su-exec replaces itself with /hivy running as the non-root "hivy" user.
su-exec hivy /hivy serve &
HIVY_PID=$!

# ── signal forwarding ─────────────────────────────────────────────────────────
# SIGTERM: drain nginx (SIGQUIT = graceful finish in-flight requests) then
# forward SIGTERM to the Go process (which has its own 30 s drain window).
_term() {
    echo "entrypoint: signal received — draining both processes" >&2
    kill -QUIT "$NGINX_PID" 2>/dev/null || true
    kill -TERM "$HIVY_PID"  2>/dev/null || true
}
trap _term TERM INT

# ── monitor: exit when either child dies ─────────────────────────────────────
# We wait for any background job; when one exits we kill the other and
# propagate the exit code so the orchestrator sees a non-zero code on failure.
EXIT_CODE=0
while true; do
    # "wait -n" (bash) is not available in plain sh; use a polling approach
    # compatible with /bin/sh (busybox).
    if ! kill -0 "$NGINX_PID" 2>/dev/null; then
        # Use "|| EXIT_CODE=$?" rather than ";" so a non-zero child exit does
        # not trip "set -e" and abort before the graceful drain below runs.
        wait "$NGINX_PID" 2>/dev/null || EXIT_CODE=$?
        echo "entrypoint: nginx exited (code $EXIT_CODE) — stopping API" >&2
        kill -TERM "$HIVY_PID" 2>/dev/null || true
        wait "$HIVY_PID" 2>/dev/null || true
        break
    fi
    if ! kill -0 "$HIVY_PID" 2>/dev/null; then
        # Use "|| EXIT_CODE=$?" rather than ";" so a non-zero child exit does
        # not trip "set -e" and abort before the graceful drain below runs.
        wait "$HIVY_PID" 2>/dev/null || EXIT_CODE=$?
        echo "entrypoint: hivy exited (code $EXIT_CODE) — stopping nginx" >&2
        kill -QUIT "$NGINX_PID" 2>/dev/null || true
        wait "$NGINX_PID" 2>/dev/null || true
        break
    fi
    # Sleep briefly to avoid a busy-poll tight loop, then re-check.
    sleep 1 &
    wait $! 2>/dev/null || true   # the sleep job; interruptible by the trap
done

exit "$EXIT_CODE"
