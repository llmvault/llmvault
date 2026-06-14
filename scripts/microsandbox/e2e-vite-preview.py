#!/usr/bin/env python3
import argparse
import json
import os
import pathlib
import shlex
import subprocess
import sys
import time
import urllib.error
import urllib.parse
import urllib.request


DEFAULT_IMAGE = "node:22-alpine"
DEFAULT_SIZE = "medium"


def env_required(name):
    value = os.environ.get(name, "").strip()
    if not value:
        raise SystemExit(f"{name} is required")
    return value


def request(control_url, token, method, path, body=None, timeout=120):
    data = None
    headers = {"Authorization": "Bearer " + token}
    if body is not None:
        data = json.dumps(body).encode()
        headers["Content-Type"] = "application/json"
    req = urllib.request.Request(control_url + path, data=data, method=method, headers=headers)
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            raw = resp.read().decode()
            parsed = json.loads(raw) if raw else None
            return resp.status, parsed
    except urllib.error.HTTPError as exc:
        raw = exc.read().decode()
        try:
            parsed = json.loads(raw) if raw else None
        except json.JSONDecodeError:
            parsed = raw
        return exc.code, parsed


def exec_command(control_url, token, sandbox_id, command, timeout):
    status, body = request(
        control_url,
        token,
        "POST",
        f"/v1/sandboxes/{sandbox_id}/exec",
        {"command": command, "timeout_seconds": timeout},
        timeout=timeout + 45,
    )
    if status >= 300:
        raise RuntimeError(f"exec HTTP {status}: {body}")
    if body.get("exit_code") != 0:
        raise RuntimeError(
            "exec failed with exit code {code}\nSTDOUT:\n{stdout}\nSTDERR:\n{stderr}".format(
                code=body.get("exit_code"),
                stdout=body.get("stdout", ""),
                stderr=body.get("stderr", ""),
            )
        )
    return body


def curl_status(url, resolve_ip):
    parsed = urllib.parse.urlparse(url)
    cmd = ["curl", "-skS", "--max-time", "10", "-o", "/dev/null", "-w", "%{http_code}"]
    if resolve_ip:
        cmd.extend(["--resolve", f"{parsed.hostname}:443:{resolve_ip}"])
    cmd.append(url)
    out = subprocess.check_output(cmd, stderr=subprocess.STDOUT, text=True, timeout=15)
    return out.strip()[-3:]


def wait_for_status(url, want, resolve_ip, timeout_seconds):
    deadline = time.monotonic() + timeout_seconds
    last = None
    while time.monotonic() < deadline:
        try:
            last = curl_status(url, resolve_ip)
        except (subprocess.CalledProcessError, subprocess.TimeoutExpired) as exc:
            last = exc.output[-200:] if getattr(exc, "output", None) else type(exc).__name__
        if last == want:
            return
        time.sleep(1)
    raise RuntimeError(f"{url} did not return {want}; last result was {last}")


def mark(timings, name):
    timings.append((name, time.monotonic()))
    print(f"[{name}]", flush=True)


def main():
    parser = argparse.ArgumentParser(description="Run the flagship Microsandbox Vite preview E2E.")
    parser.add_argument("--size", default=os.environ.get("HIVY_MICROSANDBOX_E2E_SIZE", DEFAULT_SIZE))
    parser.add_argument("--image", default=os.environ.get("HIVY_MICROSANDBOX_E2E_IMAGE", DEFAULT_IMAGE))
    parser.add_argument("--org-id", default=os.environ.get("HIVY_MICROSANDBOX_E2E_ORG_ID", "org_e2e"))
    parser.add_argument("--name", default=os.environ.get("HIVY_MICROSANDBOX_E2E_NAME", "vite-preview-e2e"))
    parser.add_argument("--preview-resolve-ip", default=os.environ.get("HIVY_MICROSANDBOX_E2E_PREVIEW_RESOLVE_IP", ""))
    parser.add_argument("--keep", action="store_true", help="leave the sandbox running instead of deleting it")
    args = parser.parse_args()

    control_url = env_required("HIVY_MICROSANDBOX_CONTROL_URL").rstrip("/")
    token = env_required("HIVY_MICROSANDBOX_API_TOKEN")
    timings = []
    sandbox_id = ""
    preview_url = ""

    try:
        mark(timings, "start")
        status, body = request(
            control_url,
            token,
            "POST",
            "/v1/sandboxes",
            {
                "org_id": args.org_id,
                "name": args.name,
                "image_ref": args.image,
                "size": args.size,
                "metadata": {"purpose": "flagship-vite-preview-e2e"},
            },
            timeout=240,
        )
        if status != 201:
            raise RuntimeError(f"create returned HTTP {status}: {body}")
        sandbox_id = body.get("ID") or body.get("id")
        preview_url = (body.get("preview_urls") or {})["5173"]
        mark(timings, "sandbox_created")
        print(f"sandbox_id={sandbox_id}")
        print(f"preview_url={preview_url}")

        preview_host = urllib.parse.urlparse(preview_url).hostname
        setup = f"""
set -e
cd /workspace
npm create vite@latest vite-app -- --template react
cd vite-app
npm install
node - <<'NODE'
const fs = require('fs')
const path = 'vite.config.js'
let s = fs.readFileSync(path, 'utf8')
s = s.replace(/plugins: \\[react\\(\\)\\],?/, "plugins: [react()],\\n  server: {{ allowedHosts: ['{preview_host}'] }},")
fs.writeFileSync(path, s)
NODE
nohup npm run dev -- --host 0.0.0.0 --port 5173 >/tmp/vite.log 2>&1 & echo $! >/tmp/vite.pid
sleep 3
cat /tmp/vite.log
"""
        exec_command(control_url, token, sandbox_id, setup, timeout=900)
        mark(timings, "vite_started")
        wait_for_status(preview_url, "200", args.preview_resolve_ip, timeout_seconds=90)
        mark(timings, "preview_live")

        status, body = request(control_url, token, "POST", f"/v1/sandboxes/{sandbox_id}/stop", timeout=120)
        if status != 200:
            raise RuntimeError(f"stop returned HTTP {status}: {body}")
        mark(timings, "sandbox_stopped")
        wait_for_status(preview_url, "503", args.preview_resolve_ip, timeout_seconds=45)
        mark(timings, "preview_stopped")

        status, body = request(control_url, token, "POST", f"/v1/sandboxes/{sandbox_id}/start", timeout=120)
        if status != 200:
            raise RuntimeError(f"start returned HTTP {status}: {body}")
        mark(timings, "sandbox_started")
        restart = """
set -e
cd /workspace/vite-app
nohup npm run dev -- --host 0.0.0.0 --port 5173 >/tmp/vite.log 2>&1 & echo $! >/tmp/vite.pid
sleep 3
cat /tmp/vite.log
"""
        exec_command(control_url, token, sandbox_id, restart, timeout=120)
        wait_for_status(preview_url, "200", args.preview_resolve_ip, timeout_seconds=90)
        mark(timings, "preview_live_after_wake")

        if not args.keep:
            status, body = request(control_url, token, "DELETE", f"/v1/sandboxes/{sandbox_id}", timeout=120)
            if status != 200:
                raise RuntimeError(f"delete returned HTTP {status}: {body}")
            mark(timings, "sandbox_deleted")
            wait_for_status(preview_url, "404", args.preview_resolve_ip, timeout_seconds=45)
            mark(timings, "preview_deleted")

        first = timings[0][1]
        print("\nTiming:")
        for name, ts in timings:
            print(f"{name}: +{ts - first:.3f}s")
        print(f"total_seconds: {timings[-1][1] - first:.3f}")
    except Exception:
        if sandbox_id and not args.keep:
            print(f"cleanup: deleting {sandbox_id}", file=sys.stderr)
            request(control_url, token, "DELETE", f"/v1/sandboxes/{sandbox_id}", timeout=120)
        raise


if __name__ == "__main__":
    main()
