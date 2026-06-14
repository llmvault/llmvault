#!/usr/bin/env python3
import json
import os
import random
import shlex
import string
import sys
import time
import urllib.error
import urllib.request


EXPECTED_CONTROL_URL = "https://msb.usehivy.com"
IMAGES = [
    ("runtime", "ghcr.io/usehivy/hivy-sandboxes-runtime:v3.1.10"),
    ("developers", "ghcr.io/usehivy/hivy-sandboxes-runtime-developers:v3.1.10"),
]
ORG_ID = "org_live_e2e"
SIZE = "medium"
PREVIEW_PORT = 8000


def require_env(name):
    value = os.environ.get(name, "").strip()
    if not value:
        raise SystemExit(f"{name} is required")
    return value


def api_request(method, path, body=None, *, expected=(200,), timeout=300):
    control_url = os.environ["HIVY_MICROSANDBOX_CONTROL_URL"].rstrip("/")
    token = os.environ["HIVY_MICROSANDBOX_API_TOKEN"]
    data = None
    headers = {"Authorization": f"Bearer {token}"}
    if body is not None:
        data = json.dumps(body).encode("utf-8")
        headers["Content-Type"] = "application/json"
    req = urllib.request.Request(control_url + path, data=data, headers=headers, method=method)
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            payload = resp.read()
            if resp.status not in expected:
                raise RuntimeError(f"{method} {path} returned {resp.status}: {payload[:4096]!r}")
            if not payload:
                return {}
            return json.loads(payload.decode("utf-8"))
    except urllib.error.HTTPError as exc:
        payload = exc.read().decode("utf-8", "replace")
        if exc.code in expected:
            if not payload:
                return {}
            try:
                return json.loads(payload)
            except json.JSONDecodeError:
                return {}
        raise RuntimeError(f"{method} {path} returned {exc.code}: {payload}") from exc


def pick(payload, *names):
    for name in names:
        if name in payload:
            return payload[name]
    return None


def sandbox_id(payload):
    value = pick(payload, "id", "ID")
    if not value:
        raise RuntimeError(f"sandbox response did not include id: {payload}")
    return value


def snapshot_id(payload):
    value = pick(payload, "id", "ID")
    if not value:
        raise RuntimeError(f"snapshot response did not include id: {payload}")
    return value


def preview_url(payload, port):
    urls = pick(payload, "preview_urls", "PreviewURLs") or {}
    url = urls.get(str(port)) or urls.get(port)
    if not url:
        raise RuntimeError(f"sandbox response did not include preview URL for port {port}: {payload}")
    return url


def exec_cmd(sandbox, command, timeout=180):
    out = api_request(
        "POST",
        f"/v1/sandboxes/{sandbox}/exec",
        {"command": command, "timeout_seconds": timeout},
    )
    stdout = out.get("stdout", "")
    stderr = out.get("stderr", "")
    exit_code = int(out.get("exit_code", 0))
    if exit_code != 0:
        raise RuntimeError(f"exec failed in {sandbox} with code {exit_code}\nSTDOUT:\n{stdout}\nSTDERR:\n{stderr}")
    return stdout + stderr


def wait_for_preview(url, marker, timeout=120):
    deadline = time.time() + timeout
    last_error = None
    while time.time() < deadline:
        try:
            with urllib.request.urlopen(url, timeout=10) as resp:
                body = resp.read().decode("utf-8", "replace")
                if resp.status == 200 and marker in body:
                    return
                last_error = f"status={resp.status} body={body[:200]!r}"
        except Exception as exc:
            last_error = str(exc)
        time.sleep(2)
    raise RuntimeError(f"preview did not become ready at {url}: {last_error}")


def create_sandbox(name, image, marker, snapshot=None):
    body = {
        "org_id": ORG_ID,
        "name": name,
        "image_ref": image,
        "snapshot_id": snapshot or "",
        "size": SIZE,
        "env": {"HIVY_MICROSANDBOX_E2E_MARKER": marker},
        "metadata": {"purpose": "live-e2e", "marker": marker},
    }
    return api_request("POST", "/v1/sandboxes", body, expected=(201,), timeout=1200)


def delete_sandbox(sandbox):
    try:
        api_request("DELETE", f"/v1/sandboxes/{sandbox}", expected=(200, 404))
    except Exception as exc:
        print(f"cleanup warning: delete sandbox {sandbox}: {exc}", file=sys.stderr)


def delete_snapshot(snapshot):
    try:
        api_request("DELETE", f"/v1/snapshots/{snapshot}", expected=(200, 404))
    except Exception as exc:
        print(f"cleanup warning: delete snapshot {snapshot}: {exc}", file=sys.stderr)


def start_preview_server(sandbox, marker):
    quoted_marker = shlex.quote(marker)
    command = f"""
set -eu
rm -rf /tmp/hivy-preview
mkdir -p /tmp/hivy-preview
cat >/tmp/hivy-preview/server.mjs <<'JS'
import http from "node:http";
const marker = process.env.HIVY_MICROSANDBOX_E2E_MARKER || "";
const server = http.createServer((req, res) => {{
  res.writeHead(200, {{"content-type": "text/plain; charset=utf-8"}});
  res.end(`hivy microsandbox live e2e ${{marker}}\\n`);
}});
server.listen(8000, "0.0.0.0");
JS
HIVY_MICROSANDBOX_E2E_MARKER={quoted_marker} nohup node /tmp/hivy-preview/server.mjs >/tmp/hivy-preview.log 2>&1 &
echo $! >/tmp/hivy-preview.pid
sleep 2
"""
    exec_cmd(sandbox, command, timeout=30)


def validate_runtime_tools(sandbox):
    exec_cmd(
        sandbox,
        """
set -eu
command -v node
command -v npm
command -v git
command -v curl
docker info >/tmp/hivy-e2e-docker-info.txt
docker compose version
""",
        timeout=180,
    )


def run_image(label, image):
    suffix = "".join(random.choice(string.ascii_lowercase + string.digits) for _ in range(8))
    marker = f"{label}-{int(time.time())}-{suffix}"
    direct_sandbox = None
    snapshot = None
    snapshot_sandbox = None
    start = time.monotonic()
    print(f"\n== {label}: {image}", flush=True)
    try:
        direct = create_sandbox(f"e2e-direct-{label}-{suffix}", image, marker)
        direct_sandbox = sandbox_id(direct)
        print(f"created direct sandbox {direct_sandbox}", flush=True)
        validate_runtime_tools(direct_sandbox)
        print("validated exec, docker, compose, and runtime tools", flush=True)

        snapshot_body = {
            "org_id": ORG_ID,
            "name": f"e2e-snapshot-{label}-{suffix}",
            "base_image_ref": image,
            "size": SIZE,
            "commands": [
                f"set -eu; mkdir -p /opt/hivy-e2e; printf %s {shlex.quote(marker)} > /opt/hivy-e2e/marker.txt; echo marker={shlex.quote(marker)}",
                "set -eu; docker info >/opt/hivy-e2e/docker-info.txt; docker compose version >/opt/hivy-e2e/docker-compose-version.txt",
                "set -eu; node --version >/opt/hivy-e2e/node-version.txt; npm --version >/opt/hivy-e2e/npm-version.txt; git --version >/opt/hivy-e2e/git-version.txt",
            ],
            "env": {"HIVY_MICROSANDBOX_E2E_MARKER": marker},
        }
        snap = api_request("POST", "/v1/snapshots", snapshot_body, expected=(201,), timeout=1200)
        snapshot = snapshot_id(snap)
        logs = pick(snap, "logs", "Logs") or ""
        if marker not in logs:
            raise RuntimeError(f"snapshot logs did not include marker {marker}: {logs[-1000:]}")
        print(f"created snapshot {snapshot}", flush=True)

        from_snapshot = create_sandbox(f"e2e-from-snapshot-{label}-{suffix}", image, marker, snapshot=snapshot)
        snapshot_sandbox = sandbox_id(from_snapshot)
        url = preview_url(from_snapshot, PREVIEW_PORT)
        print(f"created snapshot sandbox {snapshot_sandbox}", flush=True)
        exec_cmd(
            snapshot_sandbox,
            f"set -eu; test \"$(cat /opt/hivy-e2e/marker.txt)\" = {shlex.quote(marker)}; docker info >/tmp/hivy-e2e-docker-info.txt; docker compose version",
            timeout=180,
        )
        start_preview_server(snapshot_sandbox, marker)
        wait_for_preview(url, marker)
        print(f"preview ready: {url}", flush=True)

        api_request("POST", f"/v1/sandboxes/{snapshot_sandbox}/stop", expected=(200,))
        print("stopped sandbox", flush=True)
        api_request("POST", f"/v1/sandboxes/{snapshot_sandbox}/start", expected=(200,))
        print("started sandbox", flush=True)
        exec_cmd(snapshot_sandbox, f"test \"$(cat /opt/hivy-e2e/marker.txt)\" = {shlex.quote(marker)}", timeout=60)
        start_preview_server(snapshot_sandbox, marker)
        wait_for_preview(url, marker)
        print("preview recovered after stop/start", flush=True)
        elapsed = time.monotonic() - start
        print(f"{label} passed in {elapsed:.1f}s", flush=True)
    finally:
        if snapshot_sandbox:
            delete_sandbox(snapshot_sandbox)
        if direct_sandbox:
            delete_sandbox(direct_sandbox)
        if snapshot:
            delete_snapshot(snapshot)


def main():
    if os.environ.get("HIVY_MICROSANDBOX_LIVE_E2E") != "1":
        raise SystemExit("set HIVY_MICROSANDBOX_LIVE_E2E=1 to run the live prod E2E")
    control_url = require_env("HIVY_MICROSANDBOX_CONTROL_URL").rstrip("/")
    if control_url != EXPECTED_CONTROL_URL:
        raise SystemExit(f"HIVY_MICROSANDBOX_CONTROL_URL must be {EXPECTED_CONTROL_URL} for this prod-only E2E")
    require_env("HIVY_MICROSANDBOX_API_TOKEN")
    total_start = time.monotonic()
    for label, image in IMAGES:
        run_image(label, image)
    print(f"\nall live E2E checks passed in {time.monotonic() - total_start:.1f}s", flush=True)


if __name__ == "__main__":
    main()
