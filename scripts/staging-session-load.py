#!/usr/bin/env python3
"""Run reproducible session lifecycle load tests against Hivy staging.

The harness keeps account credentials in a chmod-0600 local fixture, never
prints JWTs, and writes token-free JSON results under .ignored/ by default.

Typical use:
  python3 scripts/staging-session-load.py accounts --account-count 5
  python3 scripts/staging-session-load.py approve
  python3 scripts/staging-session-load.py prepare
  python3 scripts/staging-session-load.py run --sessions 10 --create-concurrency 10

Rate-shaped examples:
  ... run --sessions 20 --create-concurrency 20
  ... run --sessions 30 --create-concurrency 10 --create-rate-per-second 2
  ... run --sessions 15 --create-concurrency 5 --max-starts-per-minute 15

Monitored 100-sandbox burst:
  ... burst --kube-tunnel-host K8S_HOST \
      --runner-host runner0=RUNNER0_IP --runner-host runner1=RUNNER1_IP
"""

from __future__ import annotations

import argparse
import asyncio
import concurrent.futures
import contextlib
import json
import math
import os
import random
import secrets
import shlex
import socket
import statistics
import subprocess
import sys
import tempfile
import time
import urllib.error
import urllib.request
from dataclasses import asdict, dataclass
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Awaitable, Callable, Iterable, TypeVar


ROOT = Path(__file__).resolve().parents[1]
DEFAULT_ACCOUNTS = ROOT / ".ignored" / "staging-session-load" / "accounts.json"
DEFAULT_RESULTS = ROOT / ".ignored" / "staging-session-load" / "results"
DEFAULT_KUBECONFIG = ROOT / "kubernetes" / "config" / "kubeconfigs" / "k8s0" / "local.yaml"
T = TypeVar("T")


@dataclass
class HTTPResult:
    status: int
    duration_ms: float
    body: Any = None
    error: str = ""

    @property
    def ok(self) -> bool:
        return 200 <= self.status < 300 and not self.error


def utc_now() -> str:
    return datetime.now(timezone.utc).isoformat()


def safe_json(raw: bytes) -> Any:
    if not raw:
        return None
    try:
        return json.loads(raw)
    except json.JSONDecodeError:
        return raw.decode("utf-8", errors="replace")[:4000]


def api_request(
    base_url: str,
    method: str,
    path: str,
    *,
    token: str = "",
    org_id: str = "",
    body: Any = None,
    timeout: float = 180,
) -> HTTPResult:
    headers = {"Accept": "application/json", "User-Agent": "hivy-staging-loadtest/1"}
    payload = None
    if body is not None:
        payload = json.dumps(body, separators=(",", ":")).encode("utf-8")
        headers["Content-Type"] = "application/json"
    if token:
        headers["Authorization"] = f"Bearer {token}"
    if org_id:
        headers["X-Org-ID"] = org_id
    request = urllib.request.Request(
        base_url.rstrip("/") + path,
        data=payload,
        headers=headers,
        method=method,
    )
    started = time.perf_counter()
    try:
        with urllib.request.urlopen(request, timeout=timeout) as response:
            raw = response.read()
            return HTTPResult(
                status=response.status,
                duration_ms=round((time.perf_counter() - started) * 1000, 2),
                body=safe_json(raw),
            )
    except urllib.error.HTTPError as exc:
        raw = exc.read()
        return HTTPResult(
            status=exc.code,
            duration_ms=round((time.perf_counter() - started) * 1000, 2),
            body=safe_json(raw),
            error=f"HTTP {exc.code}",
        )
    except Exception as exc:  # Network and timeout failures are result data.
        return HTTPResult(
            status=0,
            duration_ms=round((time.perf_counter() - started) * 1000, 2),
            error=f"{type(exc).__name__}: {exc}",
        )


async def api_request_async(*args: Any, **kwargs: Any) -> HTTPResult:
    return await asyncio.to_thread(api_request, *args, **kwargs)


def write_private_json(path: Path, value: Any) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    fd, temporary = tempfile.mkstemp(prefix=path.name + ".", dir=path.parent)
    try:
        os.fchmod(fd, 0o600)
        with os.fdopen(fd, "w", encoding="utf-8") as handle:
            json.dump(value, handle, indent=2, sort_keys=True)
            handle.write("\n")
        os.replace(temporary, path)
        path.chmod(0o600)
    finally:
        if os.path.exists(temporary):
            os.unlink(temporary)


def load_accounts(path: Path) -> dict[str, Any]:
    if not path.exists():
        return {"version": 1, "created_at": utc_now(), "accounts": []}
    with path.open("r", encoding="utf-8") as handle:
        value = json.load(handle)
    if not isinstance(value.get("accounts"), list):
        raise ValueError(f"invalid accounts fixture: {path}")
    return value


def ensure_account_records(fixture: dict[str, Any], count: int, email_domain: str) -> None:
    accounts = fixture["accounts"]
    batch = fixture.get("batch")
    if not batch:
        batch = datetime.now(timezone.utc).strftime("%Y%m%d%H%M%S") + secrets.token_hex(2)
        fixture["batch"] = batch
    while len(accounts) < count:
        index = len(accounts) + 1
        accounts.append(
            {
                "email": f"hivy-loadtest-{batch}-{index:02d}@{email_domain}",
                "password": secrets.token_urlsafe(24),
                "name": f"Hivy Load Test {index:02d}",
            }
        )


def public_http(result: HTTPResult) -> dict[str, Any]:
    body = result.body
    if isinstance(body, dict):
        body = dict(body)
        for key in ("access_token", "refresh_token", "token"):
            body.pop(key, None)
    return {
        "status": result.status,
        "duration_ms": result.duration_ms,
        "body": body,
        "error": result.error,
    }


async def login(base_url: str, account: dict[str, Any]) -> tuple[HTTPResult, str]:
    result = await api_request_async(
        base_url,
        "POST",
        "/auth/login",
        body={"email": account["email"], "password": account["password"]},
        timeout=30,
    )
    token = ""
    if result.ok and isinstance(result.body, dict):
        token = str(result.body.get("access_token", ""))
        user = result.body.get("user") or {}
        orgs = result.body.get("orgs") or []
        account["user_id"] = user.get("id", account.get("user_id"))
        account["email_confirmed"] = bool(user.get("email_confirmed"))
        if orgs:
            account["org_id"] = orgs[0].get("id", account.get("org_id"))
    return result, token


async def command_accounts(args: argparse.Namespace) -> int:
    fixture = load_accounts(args.accounts_file)
    ensure_account_records(fixture, args.account_count, args.email_domain)
    write_private_json(args.accounts_file, fixture)

    async def register(account: dict[str, Any]) -> dict[str, Any]:
        result = await api_request_async(
            args.base_url,
            "POST",
            "/auth/register",
            body={
                "email": account["email"],
                "password": account["password"],
                "name": account["name"],
            },
            timeout=30,
        )
        if result.ok and isinstance(result.body, dict):
            user = result.body.get("user") or {}
            orgs = result.body.get("orgs") or []
            account["user_id"] = user.get("id")
            account["email_confirmed"] = bool(user.get("email_confirmed"))
            if orgs:
                account["org_id"] = orgs[0].get("id")
        return {"email": account["email"], **public_http(result)}

    results = await asyncio.gather(*(register(account) for account in fixture["accounts"][: args.account_count]))
    write_private_json(args.accounts_file, fixture)
    print(json.dumps({"accounts_file": str(args.accounts_file), "results": results}, indent=2))
    return 0 if all(item["status"] in (201, 409) for item in results) else 1


def sql_literal(value: str) -> str:
    return "'" + value.replace("'", "''") + "'"


def command_approve(args: argparse.Namespace) -> int:
    fixture = load_accounts(args.accounts_file)
    accounts = fixture["accounts"][: args.account_count]
    if not accounts:
        raise ValueError("no accounts exist; run the accounts command first")
    emails = ",".join(sql_literal(account["email"]) for account in accounts)
    sql = (
        "UPDATE users SET email_confirmed_at = COALESCE(email_confirmed_at, NOW()) "
        f"WHERE email IN ({emails}) RETURNING email;"
    )
    command = [
        "kubectl",
        "--kubeconfig",
        str(args.kubeconfig),
        "-n",
        args.namespace,
        "exec",
        args.postgres_pod,
        "-c",
        "postgres",
        "--",
        "psql",
        "-U",
        "postgres",
        "-d",
        args.postgres_database,
        "-v",
        "ON_ERROR_STOP=1",
        "-Atqc",
        sql,
    ]
    completed = subprocess.run(command, check=False, capture_output=True, text=True)
    if completed.stdout:
        print(completed.stdout, end="")
    if completed.stderr:
        print(completed.stderr, file=sys.stderr, end="")
    return completed.returncode


async def command_prepare(args: argparse.Namespace) -> int:
    fixture = load_accounts(args.accounts_file)
    accounts = fixture["accounts"][: args.account_count]
    if len(accounts) < args.account_count:
        raise ValueError("not enough accounts; run the accounts command first")

    logins = await asyncio.gather(*(login(args.base_url, account) for account in accounts))
    summaries: list[dict[str, Any]] = []
    for account, (login_result, token) in zip(accounts, logins):
        summary: dict[str, Any] = {"email": account["email"], "login": public_http(login_result)}
        if not token:
            summaries.append(summary)
            continue
        if not account.get("email_confirmed"):
            summary["error"] = "email is not confirmed; run the approve command"
            summaries.append(summary)
            continue
        org_id = str(account.get("org_id", ""))
        models = await api_request_async(
            args.base_url,
            "GET",
            "/v1/agents/models",
            token=token,
            org_id=org_id,
            timeout=30,
        )
        model_ids = {
            item.get("id")
            for item in (models.body if isinstance(models.body, list) else [])
            if isinstance(item, dict)
        }
        summary["models"] = {
            "status": models.status,
            "duration_ms": models.duration_ms,
            "count": len(model_ids),
            "error": models.error,
        }
        summary["model_available"] = args.model in model_ids
        if args.model not in model_ids:
            summary["error"] = f"model {args.model!r} is not selectable for this org"
            summaries.append(summary)
            continue
        teams = await api_request_async(
            args.base_url,
            "GET",
            "/v1/orgs/current/teams?limit=100",
            token=token,
            org_id=org_id,
            timeout=30,
        )
        team_rows = teams.body.get("data", []) if teams.ok and isinstance(teams.body, dict) else []
        if team_rows:
            account["team_id"] = team_rows[0]["id"]
            summary["team"] = {"status": teams.status, "id": account["team_id"], "created": False}
        else:
            created = await api_request_async(
                args.base_url,
                "POST",
                "/v1/orgs/current/teams",
                token=token,
                org_id=org_id,
                body={"name": "Load Test Team"},
                timeout=60,
            )
            if created.ok and isinstance(created.body, dict):
                account["team_id"] = (created.body.get("team") or {}).get("id")
            summary["team"] = {
                "status": created.status,
                "duration_ms": created.duration_ms,
                "id": account.get("team_id"),
                "created": created.ok,
                "error": created.error,
            }
        summaries.append(summary)
    write_private_json(args.accounts_file, fixture)
    print(json.dumps({"accounts_file": str(args.accounts_file), "accounts": summaries}, indent=2))
    return 0 if all(item.get("model_available") and item.get("team", {}).get("id") for item in summaries) else 1


def start_interval(args: argparse.Namespace) -> float:
    intervals = [0.0]
    if args.create_rate_per_second > 0:
        intervals.append(1.0 / args.create_rate_per_second)
    if args.max_starts_per_minute > 0:
        intervals.append(60.0 / args.max_starts_per_minute)
    return max(intervals)


async def paced_map(
    items: Iterable[T],
    operation: Callable[[T, int], Awaitable[Any]],
    *,
    concurrency: int,
    interval: float = 0.0,
) -> list[Any]:
    values = list(items)
    semaphore = asyncio.Semaphore(max(1, concurrency))
    epoch = time.perf_counter()

    async def one(item: T, index: int) -> Any:
        target = epoch + index * interval
        delay = target - time.perf_counter()
        if delay > 0:
            await asyncio.sleep(delay)
        async with semaphore:
            return await operation(item, index)

    return await asyncio.gather(*(one(item, index) for index, item in enumerate(values)))


def extract_text(payload: Any) -> str:
    if isinstance(payload, dict):
        for key in ("text", "content", "message"):
            value = payload.get(key)
            if isinstance(value, str) and value.strip():
                return value.strip()
        for value in payload.values():
            found = extract_text(value)
            if found:
                return found
    if isinstance(payload, list):
        for value in payload:
            found = extract_text(value)
            if found:
                return found
    return ""


async def poll_for_final(
    base_url: str,
    token: str,
    org_id: str,
    session_id: str,
    *,
    after_sequence: int,
    timeout: float,
    poll_interval: float,
) -> dict[str, Any]:
    started = time.perf_counter()
    polls = 0
    last_events: list[dict[str, Any]] = []
    terminal_error = ""
    while time.perf_counter() - started < timeout:
        result = await api_request_async(
            base_url,
            "GET",
            f"/v1/sessions/{session_id}/events?limit=100",
            token=token,
            org_id=org_id,
            timeout=30,
        )
        polls += 1
        if result.ok and isinstance(result.body, dict):
            events = [event for event in result.body.get("data", []) if isinstance(event, dict)]
            last_events = events
            relevant = [
                event for event in events if int(event.get("sequence_number") or 0) > after_sequence
            ]
            finals = [event for event in relevant if event.get("event_type") == "final"]
            if finals:
                event = min(finals, key=lambda value: int(value.get("sequence_number") or 0))
                return {
                    "outcome": "responded",
                    "latency_ms": round((time.perf_counter() - started) * 1000, 2),
                    "polls": polls,
                    "event": event,
                    "text": extract_text(event.get("payload")),
                    "events": relevant,
                }
            failures = [
                event
                for event in relevant
                if event.get("event_type") in ("error", "turn_failed")
            ]
            if failures:
                failure = min(failures, key=lambda value: int(value.get("sequence_number") or 0))
                terminal_error = extract_text(failure.get("payload")) or failure.get("event_type", "error")
                return {
                    "outcome": "failed",
                    "latency_ms": round((time.perf_counter() - started) * 1000, 2),
                    "polls": polls,
                    "error": terminal_error,
                    "event": failure,
                    "events": relevant,
                }
        elif result.status in (401, 403, 404):
            terminal_error = result.error or f"events returned {result.status}"
            break
        await asyncio.sleep(poll_interval)
    return {
        "outcome": "timeout" if not terminal_error else "failed",
        "latency_ms": round((time.perf_counter() - started) * 1000, 2),
        "polls": polls,
        "error": terminal_error or f"no final event within {timeout:.0f}s",
        "events": last_events,
    }


def response_sequence_boundary(response: dict[str, Any]) -> int:
    """Return the highest durable event sequence observed for one response poll."""
    sequences = [
        int(event.get("sequence_number") or 0)
        for event in [response.get("event"), *(response.get("events") or [])]
        if isinstance(event, dict)
    ]
    return max(sequences, default=0)


def percentile(values: list[float], fraction: float) -> float | None:
    if not values:
        return None
    ordered = sorted(values)
    index = max(0, math.ceil(fraction * len(ordered)) - 1)
    return round(ordered[index], 2)


def latency_summary(values: list[float]) -> dict[str, Any]:
    if not values:
        return {"count": 0}
    return {
        "count": len(values),
        "min_ms": round(min(values), 2),
        "mean_ms": round(statistics.fmean(values), 2),
        "p50_ms": percentile(values, 0.50),
        "p95_ms": percentile(values, 0.95),
        "max_ms": round(max(values), 2),
    }


def numeric_summary(values: list[float]) -> dict[str, Any]:
    if not values:
        return {"count": 0}
    return {
        "count": len(values),
        "min": round(min(values), 2),
        "mean": round(statistics.fmean(values), 2),
        "p50": percentile(values, 0.50),
        "p95": percentile(values, 0.95),
        "max": round(max(values), 2),
    }


TEST_CHARACTERS = "ABCD"


def burst_message(round_index: int) -> tuple[str, str]:
    character = TEST_CHARACTERS[round_index % len(TEST_CHARACTERS)]
    return character, f"This is a test - reply only with character {character}"


def exact_character_reply(text: str, expected: str) -> bool:
    return text.strip() == expected


def build_burst_plan(
    sessions: int,
    first_wave_size: int,
    min_rounds: int,
    max_rounds: int,
    gap_min: float,
    gap_max: float,
    rng: random.Random,
) -> tuple[list[dict[str, Any]], float]:
    gap = rng.uniform(gap_min, gap_max)
    plan = []
    for index in range(sessions):
        plan.append(
            {
                "index": index + 1,
                "wave": 1 if index < first_wave_size else 2,
                "start_offset_seconds": 0.0 if index < first_wave_size else gap,
                "rounds_planned": rng.randint(min_rounds, max_rounds),
            }
        )
    return plan, gap


def http_meta(result: HTTPResult) -> dict[str, Any]:
    value = {
        "status": result.status,
        "duration_ms": result.duration_ms,
        "error": result.error,
    }
    if not result.ok and result.body is not None:
        value["response_body"] = result.body
    return value


def json_field(value: dict[str, Any], snake_case: str, go_name: str) -> Any:
    return value.get(snake_case, value.get(go_name))


def compact_poll_result(result: dict[str, Any], expected: str) -> dict[str, Any]:
    event = result.get("event") if isinstance(result.get("event"), dict) else {}
    text = str(result.get("text", ""))
    return {
        "outcome": result.get("outcome", "unknown"),
        "latency_ms": result.get("latency_ms"),
        "polls": result.get("polls", 0),
        "text": text,
        "expected": expected,
        "exact_match": result.get("outcome") == "responded" and exact_character_reply(text, expected),
        "error": result.get("error", ""),
        "sequence": int(event.get("sequence_number") or 0),
        "event_at": event.get("event_at"),
    }


def tcp_open(host: str, port: int) -> bool:
    with contextlib.closing(socket.socket(socket.AF_INET, socket.SOCK_STREAM)) as sock:
        sock.settimeout(0.25)
        return sock.connect_ex((host, port)) == 0


def available_local_port() -> int:
    with contextlib.closing(socket.socket(socket.AF_INET, socket.SOCK_STREAM)) as sock:
        sock.bind(("127.0.0.1", 0))
        return int(sock.getsockname()[1])


def run_quiet(command: list[str], timeout: float = 30) -> subprocess.CompletedProcess[str]:
    return subprocess.run(command, check=False, capture_output=True, text=True, timeout=timeout)


class BurstInfrastructure:
    def __init__(self, args: argparse.Namespace):
        self.args = args
        self.kube_tunnel: subprocess.Popen[str] | None = None
        self.control_forward: subprocess.Popen[str] | None = None
        self.control_url = ""
        self.control_token = ""

    def __enter__(self) -> "BurstInfrastructure":
        self.ensure_kubernetes()
        self.ensure_control_forward()
        return self

    def __exit__(self, _exc_type: Any, _exc: Any, _traceback: Any) -> None:
        self.close()

    def kubectl(self, *values: str) -> list[str]:
        return ["kubectl", "--kubeconfig", str(self.args.kubeconfig), *values]

    def ensure_kubernetes(self) -> None:
        ready = run_quiet(self.kubectl("get", "--raw=/readyz"), timeout=5)
        if ready.returncode == 0:
            return
        if not self.args.kube_tunnel_host:
            raise RuntimeError("Kubernetes is unavailable and --kube-tunnel-host was not provided")
        self.kube_tunnel = subprocess.Popen(
            [
                "ssh",
                "-i",
                str(self.args.ssh_key),
                "-o",
                "BatchMode=yes",
                "-o",
                "ConnectTimeout=10",
                "-o",
                "ExitOnForwardFailure=yes",
                "-o",
                "ServerAliveInterval=30",
                "-N",
                "-L",
                f"127.0.0.1:{self.args.kube_local_port}:127.0.0.1:6443",
                f"root@{self.args.kube_tunnel_host}",
            ],
            stdin=subprocess.DEVNULL,
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
            text=True,
        )
        for _ in range(30):
            if self.kube_tunnel.poll() is not None:
                break
            ready = run_quiet(self.kubectl("get", "--raw=/readyz"), timeout=5)
            if ready.returncode == 0:
                return
            time.sleep(0.5)
        raise RuntimeError("failed to establish Kubernetes API tunnel")

    def ensure_control_forward(self) -> None:
        token_result = run_quiet(
            self.kubectl(
                "-n",
                self.args.namespace,
                "exec",
                "deploy/backend-api",
                "-c",
                "api",
                "--",
                "printenv",
                "HIVY_MICROSANDBOX_CONTROL_API_TOKEN",
            )
        )
        self.control_token = token_result.stdout.strip()
        if token_result.returncode != 0 or not self.control_token:
            raise RuntimeError("failed to read the Microsandbox control API token")
        port = available_local_port()
        self.control_url = f"http://127.0.0.1:{port}"
        self.control_forward = subprocess.Popen(
            self.kubectl(
                "-n",
                "production",
                "port-forward",
                "service/microsandbox-control",
                f"{port}:8080",
            ),
            stdin=subprocess.DEVNULL,
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
            text=True,
        )
        for _ in range(30):
            if self.control_forward.poll() is not None:
                break
            if tcp_open("127.0.0.1", port):
                health = api_request(self.control_url, "GET", "/health", timeout=5)
                if health.ok:
                    return
            time.sleep(0.25)
        raise RuntimeError("failed to establish Microsandbox control port-forward")

    def close(self) -> None:
        self.control_token = ""
        for process in (self.control_forward, self.kube_tunnel):
            if process is None or process.poll() is not None:
                continue
            process.terminate()
            try:
                process.wait(timeout=5)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait(timeout=5)


HOST_METRICS_SCRIPT = r'''import json,time
def cpu():
    fields=list(map(int,open("/proc/stat").readline().split()[1:]))
    return fields,sum(fields)
previous,total_previous=cpu()
while True:
    time.sleep(1)
    current,total=cpu()
    delta=max(1,total-total_previous)
    idle=(current[3]-previous[3])+(current[4]-previous[4])
    iowait=current[4]-previous[4]
    mem={line.split(":",1)[0]:int(line.split()[1]) for line in open("/proc/meminfo")}
    load=open("/proc/loadavg").read().split()
    stat=open("/proc/stat").read().splitlines()
    values={line.split()[0]:int(line.split()[1]) for line in stat if line.startswith(("procs_running ","procs_blocked ","ctxt "))}
    print(json.dumps({"timestamp":time.time(),"cpu_busy_pct":round(100*(delta-idle)/delta,2),"cpu_iowait_pct":round(100*iowait/delta,2),"mem_available_mb":round(mem.get("MemAvailable",0)/1024,2),"load1":float(load[0]),"load5":float(load[1]),"load15":float(load[2]),"procs_running":values.get("procs_running",0),"procs_blocked":values.get("procs_blocked",0),"context_switches":values.get("ctxt",0)},separators=(",",":")),flush=True)
    previous,total_previous=current,total
'''


def parse_runner_hosts(values: list[str]) -> list[tuple[str, str]]:
    hosts = []
    for value in values:
        name, separator, address = value.partition("=")
        if not separator or not name.strip() or not address.strip():
            raise ValueError(f"invalid --runner-host {value!r}; expected name=address")
        hosts.append((name.strip(), address.strip()))
    return hosts


async def collect_host_metrics(
    name: str,
    address: str,
    ssh_key: Path,
    stop: asyncio.Event,
    samples: list[dict[str, Any]],
    errors: list[str],
) -> None:
    remote_command = "python3 -u -c " + shlex.quote(HOST_METRICS_SCRIPT)
    process = await asyncio.create_subprocess_exec(
        "ssh",
        "-i",
        str(ssh_key),
        "-o",
        "BatchMode=yes",
        "-o",
        "ConnectTimeout=10",
        "-o",
        "ServerAliveInterval=15",
        f"root@{address}",
        remote_command,
        stdout=asyncio.subprocess.PIPE,
        stderr=asyncio.subprocess.PIPE,
    )
    try:
        if process.stdout is None:
            raise RuntimeError("host metrics stdout is unavailable")
        while not stop.is_set():
            try:
                raw = await asyncio.wait_for(process.stdout.readline(), timeout=2)
            except asyncio.TimeoutError:
                if process.returncode is not None:
                    break
                continue
            if not raw:
                break
            try:
                sample = json.loads(raw)
                sample["runner"] = name
                samples.append(sample)
            except json.JSONDecodeError:
                continue
    except Exception as exc:
        errors.append(f"host metrics {name}: {type(exc).__name__}: {exc}")
    finally:
        if process.returncode is None:
            process.terminate()
            try:
                await asyncio.wait_for(process.wait(), timeout=5)
            except asyncio.TimeoutError:
                process.kill()
                await process.wait()
        if process.returncode not in (None, 0, -15, 255) and process.stderr is not None:
            stderr = (await process.stderr.read()).decode("utf-8", errors="replace").strip()
            errors.append(f"host metrics {name} exited {process.returncode}: {stderr[:500]}")


def project_runner(value: dict[str, Any]) -> dict[str, Any]:
    fields = {
        "id": "ID",
        "name": "Name",
        "status": "Status",
        "drain": "Drain",
        "total_cpu": "TotalCPU",
        "total_memory_mb": "TotalMemoryMB",
        "total_disk_gb": "TotalDiskGB",
        "reserved_cpu": "ReservedCPU",
        "reserved_memory_mb": "ReservedMemoryMB",
        "reserved_disk_gb": "ReservedDiskGB",
        "cpu_overcommit": "CPUOvercommit",
        "memory_overcommit": "MemoryOvercommit",
        "disk_overcommit": "DiskOvercommit",
        "last_heartbeat_at": "LastHeartbeatAt",
    }
    return {key: value.get(key, value.get(go_key)) for key, go_key in fields.items()}


async def collect_runner_samples(
    infrastructure: BurstInfrastructure,
    interval: float,
    stop: asyncio.Event,
    samples: list[dict[str, Any]],
    errors: list[str],
) -> None:
    while not stop.is_set():
        result = await api_request_async(
            infrastructure.control_url,
            "GET",
            "/v1/runners",
            token=infrastructure.control_token,
            timeout=10,
        )
        if result.ok and isinstance(result.body, dict):
            runners = [project_runner(value) for value in result.body.get("data", []) if isinstance(value, dict)]
            samples.append({"observed_at": utc_now(), "runners": runners})
        else:
            errors.append(f"runner sample: status={result.status} error={result.error}")
        try:
            await asyncio.wait_for(stop.wait(), timeout=interval)
        except asyncio.TimeoutError:
            pass


async def command_run(args: argparse.Namespace) -> int:
    fixture = load_accounts(args.accounts_file)
    accounts = fixture["accounts"][: args.account_count]
    if len(accounts) < args.account_count:
        raise ValueError("not enough accounts; run accounts, approve, and prepare first")
    if any(not account.get("team_id") for account in accounts):
        raise ValueError("one or more accounts have no team_id; run prepare first")

    run_id = datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%SZ") + "-" + secrets.token_hex(2)
    result: dict[str, Any] = {
        "run_id": run_id,
        "started_at": utc_now(),
        "config": {
            "base_url": args.base_url,
            "account_count": args.account_count,
            "sessions": args.sessions,
            "model": args.model,
            "initial_message": args.initial_message,
            "followup_message": args.followup_message,
            "create_concurrency": args.create_concurrency,
            "create_rate_per_second": args.create_rate_per_second,
            "max_starts_per_minute": args.max_starts_per_minute,
            "idle_wait_seconds": args.idle_wait_seconds,
            "response_timeout_seconds": args.response_timeout,
        },
        "accounts": [],
        "sessions": [],
    }

    login_pairs = await asyncio.gather(*(login(args.base_url, account) for account in accounts))
    tokens: dict[str, str] = {}
    for account, (login_result, token) in zip(accounts, login_pairs):
        result["accounts"].append(
            {"email": account["email"], "org_id": account.get("org_id"), "login": public_http(login_result)}
        )
        if token:
            tokens[account["email"]] = token
    if len(tokens) != len(accounts):
        result["finished_at"] = utc_now()
        result["fatal_error"] = "one or more accounts could not log in"
        output = args.results_dir / f"{run_id}.json"
        write_private_json(output, result)
        print(json.dumps({"result_file": str(output), "fatal_error": result["fatal_error"]}, indent=2))
        return 1

    assignments: list[dict[str, Any]] = []
    for index in range(args.sessions):
        account = accounts[index % len(accounts)]
        assignments.append({"index": index + 1, "account": account})

    async def create_agent(assignment: dict[str, Any], index: int) -> dict[str, Any]:
        account = assignment["account"]
        response = await api_request_async(
            args.base_url,
            "POST",
            "/v1/agents",
            token=tokens[account["email"]],
            org_id=str(account.get("org_id", "")),
            body={
                "name": f"Load Test {run_id} Agent {index + 1:02d}",
                "team_id": account["team_id"],
                "model": args.model,
                "instructions": "Respond briefly and directly. Do not take actions unless explicitly asked.",
                "sandbox_size": args.sandbox_size,
            },
            timeout=60,
        )
        agent_id = ""
        if response.ok and isinstance(response.body, dict):
            agent_id = str((response.body.get("agent") or {}).get("id", ""))
        return {"agent_id": agent_id, "http": public_http(response)}

    agent_results = await paced_map(
        assignments,
        create_agent,
        concurrency=min(args.create_concurrency, max(1, args.sessions)),
    )
    for assignment, agent in zip(assignments, agent_results):
        assignment["agent"] = agent

    create_epoch = utc_now()

    async def create_session(assignment: dict[str, Any], _index: int) -> dict[str, Any]:
        account = assignment["account"]
        agent_id = assignment["agent"]["agent_id"]
        if not agent_id:
            return {"session_id": "", "sandbox_id": "", "http": {"status": 0, "error": "agent create failed"}}
        response = await api_request_async(
            args.base_url,
            "POST",
            "/v1/sessions",
            token=tokens[account["email"]],
            org_id=str(account.get("org_id", "")),
            body={
                "agent_id": agent_id,
                "text": args.initial_message,
                "model_definition": {"model_id": args.model},
            },
            timeout=args.create_timeout,
        )
        session = response.body.get("session", {}) if response.ok and isinstance(response.body, dict) else {}
        event = response.body.get("event", {}) if response.ok and isinstance(response.body, dict) else {}
        return {
            "session_id": str(session.get("id", "")),
            "sandbox_id": str(session.get("sandbox_id", "")),
            "model": session.get("model"),
            "queued": response.body.get("queued") if response.ok and isinstance(response.body, dict) else None,
            "initial_sequence": int(event.get("sequence_number") or 0),
            "http": public_http(response),
        }

    session_creates = await paced_map(
        assignments,
        create_session,
        concurrency=args.create_concurrency,
        interval=start_interval(args),
    )

    rows: list[dict[str, Any]] = []
    for assignment, created in zip(assignments, session_creates):
        account = assignment["account"]
        rows.append(
            {
                "index": assignment["index"],
                "account_email": account["email"],
                "org_id": account.get("org_id"),
                "team_id": account.get("team_id"),
                "agent_id": assignment["agent"]["agent_id"],
                "agent_create": assignment["agent"]["http"],
                **created,
            }
        )
    result["create_phase_started_at"] = create_epoch
    result["sessions"] = rows

    async def initial_response(row: dict[str, Any]) -> dict[str, Any]:
        if not row["session_id"]:
            return {"outcome": "not_created"}
        return await poll_for_final(
            args.base_url,
            tokens[row["account_email"]],
            row["org_id"],
            row["session_id"],
            after_sequence=row["initial_sequence"],
            timeout=args.response_timeout,
            poll_interval=args.poll_interval,
        )

    initial_results = await asyncio.gather(*(initial_response(row) for row in rows))
    for row, response in zip(rows, initial_results):
        row["initial_response"] = response
        row["followup_after_sequence"] = response_sequence_boundary(response)

    async def followup(row: dict[str, Any]) -> dict[str, Any]:
        if not row["session_id"]:
            return {"accepted": False, "http": {"status": 0, "error": "session was not created"}}
        response = await api_request_async(
            args.base_url,
            "POST",
            f"/v1/sessions/{row['session_id']}/messages",
            token=tokens[row["account_email"]],
            org_id=row["org_id"],
            body={"text": args.followup_message},
            timeout=60,
        )
        event = response.body.get("event", {}) if response.ok and isinstance(response.body, dict) else {}
        return {
            "accepted": response.ok,
            "sequence": int(event.get("sequence_number") or 0),
            "queued": response.body.get("queued") if response.ok and isinstance(response.body, dict) else None,
            "http": public_http(response),
        }

    result["followup_phase_started_at"] = utc_now()
    followups = await asyncio.gather(*(followup(row) for row in rows))
    for row, followup_result in zip(rows, followups):
        row["followup"] = followup_result

    async def followup_response(row: dict[str, Any]) -> dict[str, Any]:
        if not row["followup"].get("accepted"):
            return {"outcome": "not_accepted"}
        return await poll_for_final(
            args.base_url,
            tokens[row["account_email"]],
            row["org_id"],
            row["session_id"],
            after_sequence=max(
                row["followup"]["sequence"],
                row["followup_after_sequence"],
            ),
            timeout=args.response_timeout,
            poll_interval=args.poll_interval,
        )

    followup_responses = await asyncio.gather(*(followup_response(row) for row in rows))
    for row, response in zip(rows, followup_responses):
        row["followup_response"] = response

    result["idle_wait_started_at"] = utc_now()
    await asyncio.sleep(args.idle_wait_seconds)
    result["sandbox_check_started_at"] = utc_now()

    async def sandbox_state(row: dict[str, Any]) -> dict[str, Any]:
        if not row["sandbox_id"]:
            return {"status": "unavailable", "http": {"status": 0, "error": "sandbox id missing"}}
        response = await api_request_async(
            args.base_url,
            "GET",
            f"/v1/sandboxes/{row['sandbox_id']}",
            token=tokens[row["account_email"]],
            org_id=row["org_id"],
            timeout=30,
        )
        status = response.body.get("status", "unknown") if response.ok and isinstance(response.body, dict) else "error"
        return {"status": status, "http": public_http(response)}

    states = await asyncio.gather(*(sandbox_state(row) for row in rows))
    for row, state in zip(rows, states):
        row["sandbox_after_idle_wait"] = state

    all_create_latencies = [row["http"]["duration_ms"] for row in rows]
    successful_create_latencies = [
        row["http"]["duration_ms"] for row in rows if 200 <= row["http"]["status"] < 300
    ]
    failed_create_latencies = [
        row["http"]["duration_ms"] for row in rows if not 200 <= row["http"]["status"] < 300
    ]
    initial_latencies = [
        row["initial_response"]["latency_ms"]
        for row in rows
        if row["initial_response"].get("outcome") == "responded"
    ]
    followup_post_latencies = [
        row["followup"]["http"]["duration_ms"]
        for row in rows
        if row["followup"].get("accepted")
    ]
    followup_response_latencies = [
        row["followup_response"]["latency_ms"]
        for row in rows
        if row["followup_response"].get("outcome") == "responded"
    ]
    sandbox_statuses: dict[str, int] = {}
    create_http_statuses: dict[str, int] = {}
    for row in rows:
        status = row["sandbox_after_idle_wait"]["status"]
        sandbox_statuses[status] = sandbox_statuses.get(status, 0) + 1
        http_status = str(row["http"]["status"])
        create_http_statuses[http_status] = create_http_statuses.get(http_status, 0) + 1
    result["summary"] = {
        "agents_created": sum(1 for row in rows if row["agent_id"]),
        "sessions_created": sum(1 for row in rows if row["session_id"]),
        "sessions_failed": sum(1 for row in rows if not row["session_id"]),
        "initial_responses": sum(1 for row in rows if row["initial_response"].get("outcome") == "responded"),
        "followups_accepted": sum(1 for row in rows if row["followup"].get("accepted")),
        "followup_responses": sum(1 for row in rows if row["followup_response"].get("outcome") == "responded"),
        "sandbox_statuses_after_idle_wait": sandbox_statuses,
        "session_create_http_statuses": create_http_statuses,
        "session_create_latency": latency_summary(all_create_latencies),
        "session_create_success_latency": latency_summary(successful_create_latencies),
        "session_create_failure_latency": latency_summary(failed_create_latencies),
        "initial_response_latency": latency_summary(initial_latencies),
        "followup_post_latency": latency_summary(followup_post_latencies),
        "followup_response_latency": latency_summary(followup_response_latencies),
    }
    result["finished_at"] = utc_now()
    output = args.results_dir / f"{run_id}.json"
    write_private_json(output, result)
    print(json.dumps({"result_file": str(output), "run_id": run_id, "summary": result["summary"]}, indent=2))
    return 0 if result["summary"]["sessions_created"] == args.sessions else 1


def summarize_host_metrics(samples: list[dict[str, Any]]) -> dict[str, Any]:
    output: dict[str, Any] = {}
    names = sorted({str(sample.get("runner", "")) for sample in samples if sample.get("runner")})
    for name in names:
        rows = [sample for sample in samples if sample.get("runner") == name]
        output[name] = {
            "samples": len(rows),
            "cpu_busy_pct": numeric_summary([float(row["cpu_busy_pct"]) for row in rows]),
            "cpu_iowait_pct": numeric_summary([float(row["cpu_iowait_pct"]) for row in rows]),
            "min_mem_available_mb": min((float(row["mem_available_mb"]) for row in rows), default=None),
            "max_load1": max((float(row["load1"]) for row in rows), default=None),
            "max_procs_running": max((int(row["procs_running"]) for row in rows), default=None),
            "max_procs_blocked": max((int(row["procs_blocked"]) for row in rows), default=None),
        }
    return output


def summarize_runner_samples(samples: list[dict[str, Any]]) -> dict[str, Any]:
    output: dict[str, Any] = {}
    names = sorted(
        {
            str(runner.get("name", ""))
            for sample in samples
            for runner in sample.get("runners", [])
            if runner.get("name")
        }
    )
    for name in names:
        rows = [
            runner
            for sample in samples
            for runner in sample.get("runners", [])
            if runner.get("name") == name
        ]
        output[name] = {
            "samples": len(rows),
            "statuses": sorted({str(row.get("status", "")) for row in rows}),
            "max_reserved_cpu": max((int(row.get("reserved_cpu") or 0) for row in rows), default=0),
            "max_reserved_memory_mb": max(
                (int(row.get("reserved_memory_mb") or 0) for row in rows), default=0
            ),
            "max_reserved_disk_gb": max(
                (int(row.get("reserved_disk_gb") or 0) for row in rows), default=0
            ),
        }
    return output


def burst_assertions(rows: list[dict[str, Any]], expected_sessions: int) -> dict[str, bool]:
    rounds = [round_result for row in rows for round_result in row.get("rounds", [])]
    planned = sum(int(row.get("rounds_planned") or 0) for row in rows)
    stopped = sum(1 for row in rows if row.get("sandbox_after_idle_wait", {}).get("status") == "stopped")
    upstream_stopped = sum(
        1 for row in rows if row.get("control_after_idle_wait", {}).get("status") == "stopped"
    )
    return {
        "all_agents_created": sum(1 for row in rows if row.get("agent_id")) == expected_sessions,
        "all_sessions_created": sum(1 for row in rows if row.get("session_id")) == expected_sessions,
        "all_messages_accepted": sum(1 for value in rounds if value.get("accepted")) == planned,
        "all_messages_responded": sum(
            1 for value in rounds if value.get("response", {}).get("outcome") == "responded"
        )
        == planned,
        "all_outputs_exact": sum(
            1 for value in rounds if value.get("response", {}).get("exact_match")
        )
        == planned,
        "all_sandboxes_asleep": stopped == expected_sessions,
        "all_control_sandboxes_stopped": upstream_stopped == expected_sessions,
    }


async def command_burst_run(
    args: argparse.Namespace,
    infrastructure: BurstInfrastructure,
) -> int:
    fixture = load_accounts(args.accounts_file)
    accounts = fixture["accounts"][: args.account_count]
    if len(accounts) < args.account_count or any(not account.get("team_id") for account in accounts):
        raise ValueError("burst account preparation is incomplete")

    seed = args.seed if args.seed is not None else secrets.randbits(63)
    rng = random.Random(seed)
    plan, wave_gap = build_burst_plan(
        args.sessions,
        args.first_wave_size,
        args.min_messages,
        args.max_messages,
        args.wave_gap_min,
        args.wave_gap_max,
        rng,
    )
    idle_wait = rng.uniform(args.idle_wait_min, args.idle_wait_max)
    run_id = datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%SZ") + "-burst-" + secrets.token_hex(2)
    output = args.results_dir / f"{run_id}.json"
    result: dict[str, Any] = {
        "run_id": run_id,
        "started_at": utc_now(),
        "config": {
            "base_url": args.base_url,
            "account_count": args.account_count,
            "sessions": args.sessions,
            "first_wave_size": args.first_wave_size,
            "second_wave_size": args.sessions - args.first_wave_size,
            "wave_gap_seconds": round(wave_gap, 3),
            "min_messages": args.min_messages,
            "max_messages": args.max_messages,
            "idle_wait_seconds": round(idle_wait, 3),
            "model": args.model,
            "sandbox_size": args.sandbox_size,
            "seed": seed,
            "poll_interval_seconds": args.poll_interval,
            "response_timeout_seconds": args.response_timeout,
        },
        "accounts": [],
        "sessions": [],
        "runner_samples": [],
        "host_samples": [],
        "monitor_errors": [],
    }

    login_pairs = await asyncio.gather(*(login(args.base_url, account) for account in accounts))
    tokens: dict[str, str] = {}
    for account, (login_result, token) in zip(accounts, login_pairs):
        result["accounts"].append(
            {"email": account["email"], "org_id": account.get("org_id"), "login": http_meta(login_result)}
        )
        if token:
            tokens[account["email"]] = token
    if len(tokens) != len(accounts):
        result["fatal_error"] = "one or more accounts could not log in"
        result["finished_at"] = utc_now()
        write_private_json(output, result)
        print(json.dumps({"result_file": str(output), "fatal_error": result["fatal_error"]}, indent=2))
        return 1

    assignments: list[dict[str, Any]] = []
    for item in plan:
        account = accounts[(item["index"] - 1) % len(accounts)]
        assignments.append({**item, "account": account})

    async def create_agent(assignment: dict[str, Any], index: int) -> dict[str, Any]:
        account = assignment["account"]
        response = await api_request_async(
            args.base_url,
            "POST",
            "/v1/agents",
            token=tokens[account["email"]],
            org_id=str(account.get("org_id", "")),
            body={
                "name": f"Burst Test {run_id} Agent {index + 1:03d}",
                "team_id": account["team_id"],
                "model": args.model,
                "instructions": (
                    "For every message, output exactly the one requested character and nothing else. "
                    "Never call a tool."
                ),
                "sandbox_size": args.sandbox_size,
                "tools": {},
            },
            timeout=60,
        )
        agent = response.body.get("agent", {}) if response.ok and isinstance(response.body, dict) else {}
        return {"agent_id": str(agent.get("id", "")), "http": http_meta(response)}

    print(f"[{utc_now()}] creating {args.sessions} isolated nano agents", flush=True)
    agent_results = await paced_map(
        assignments,
        create_agent,
        concurrency=min(args.agent_create_concurrency, args.sessions),
    )
    for assignment, agent in zip(assignments, agent_results):
        assignment["agent"] = agent

    stop_monitor = asyncio.Event()
    monitor_tasks: list[asyncio.Task[None]] = [
        asyncio.create_task(
            collect_runner_samples(
                infrastructure,
                args.monitor_interval,
                stop_monitor,
                result["runner_samples"],
                result["monitor_errors"],
            )
        )
    ]
    for name, address in parse_runner_hosts(args.runner_host):
        monitor_tasks.append(
            asyncio.create_task(
                collect_host_metrics(
                    name,
                    address,
                    args.ssh_key,
                    stop_monitor,
                    result["host_samples"],
                    result["monitor_errors"],
                )
            )
        )

    epoch = time.perf_counter()
    result["wave_started_at"] = utc_now()
    print(
        f"[{utc_now()}] starting wave 1 ({args.first_wave_size}); "
        f"wave 2 ({args.sessions - args.first_wave_size}) scheduled at +{wave_gap:.3f}s",
        flush=True,
    )

    async def conversation(assignment: dict[str, Any]) -> dict[str, Any]:
        target = epoch + float(assignment["start_offset_seconds"])
        delay = target - time.perf_counter()
        if delay > 0:
            await asyncio.sleep(delay)
        started = time.perf_counter()
        account = assignment["account"]
        token = tokens[account["email"]]
        row: dict[str, Any] = {
            "index": assignment["index"],
            "wave": assignment["wave"],
            "scheduled_offset_ms": round(float(assignment["start_offset_seconds"]) * 1000, 2),
            "actual_start_offset_ms": round((started - epoch) * 1000, 2),
            "rounds_planned": assignment["rounds_planned"],
            "account_email": account["email"],
            "org_id": account.get("org_id"),
            "team_id": account.get("team_id"),
            "agent_id": assignment["agent"]["agent_id"],
            "agent_create": assignment["agent"]["http"],
            "session_id": "",
            "sandbox_id": "",
            "rounds": [],
        }
        if not row["agent_id"]:
            row["create_error"] = "agent was not created"
            return row

        expected, prompt = burst_message(0)
        create = await api_request_async(
            args.base_url,
            "POST",
            "/v1/sessions",
            token=token,
            org_id=str(account.get("org_id", "")),
            body={
                "agent_id": row["agent_id"],
                "text": prompt,
                "model_definition": {"model_id": args.model},
            },
            timeout=args.create_timeout,
        )
        session = create.body.get("session", {}) if create.ok and isinstance(create.body, dict) else {}
        event = create.body.get("event", {}) if create.ok and isinstance(create.body, dict) else {}
        row["session_id"] = str(session.get("id", ""))
        row["sandbox_id"] = str(session.get("sandbox_id", ""))
        row["session_create"] = http_meta(create)
        first_round: dict[str, Any] = {
            "round": 1,
            "kind": "session_create",
            "prompt": prompt,
            "expected": expected,
            "accepted": create.ok,
            "post": http_meta(create),
        }
        row["rounds"].append(first_round)
        if not row["session_id"]:
            first_round["response"] = {"outcome": "not_created", "exact_match": False}
            return row

        raw_response = await poll_for_final(
            args.base_url,
            token,
            str(account.get("org_id", "")),
            row["session_id"],
            after_sequence=int(event.get("sequence_number") or 0),
            timeout=args.response_timeout,
            poll_interval=args.poll_interval,
        )
        first_round["response"] = compact_poll_result(raw_response, expected)
        boundary = response_sequence_boundary(raw_response)
        if raw_response.get("outcome") != "responded":
            return row

        for round_index in range(1, int(assignment["rounds_planned"])):
            expected, prompt = burst_message(round_index)
            post = await api_request_async(
                args.base_url,
                "POST",
                f"/v1/sessions/{row['session_id']}/messages",
                token=token,
                org_id=str(account.get("org_id", "")),
                body={"text": prompt},
                timeout=60,
            )
            post_event = post.body.get("event", {}) if post.ok and isinstance(post.body, dict) else {}
            round_result: dict[str, Any] = {
                "round": round_index + 1,
                "kind": "message",
                "prompt": prompt,
                "expected": expected,
                "accepted": post.ok,
                "post": http_meta(post),
            }
            row["rounds"].append(round_result)
            if not post.ok:
                round_result["response"] = {"outcome": "not_accepted", "exact_match": False}
                break
            raw_response = await poll_for_final(
                args.base_url,
                token,
                str(account.get("org_id", "")),
                row["session_id"],
                after_sequence=max(int(post_event.get("sequence_number") or 0), boundary),
                timeout=args.response_timeout,
                poll_interval=args.poll_interval,
            )
            round_result["response"] = compact_poll_result(raw_response, expected)
            boundary = response_sequence_boundary(raw_response)
            if raw_response.get("outcome") != "responded":
                break
        return row

    try:
        rows = await asyncio.gather(*(conversation(assignment) for assignment in assignments))
        result["sessions"] = rows
        result["conversations_finished_at"] = utc_now()
        print(
            f"[{utc_now()}] conversations complete; waiting {idle_wait:.3f}s before sleep assertion",
            flush=True,
        )
        result["idle_wait_started_at"] = utc_now()
        await asyncio.sleep(idle_wait)
        result["sandbox_check_started_at"] = utc_now()

        async def sandbox_state(row: dict[str, Any]) -> dict[str, Any]:
            if not row.get("sandbox_id"):
                return {"status": "unavailable", "external_id": "", "http": {"status": 0}}
            response = await api_request_async(
                args.base_url,
                "GET",
                f"/v1/sandboxes/{row['sandbox_id']}",
                token=tokens[row["account_email"]],
                org_id=str(row["org_id"]),
                timeout=30,
            )
            body = response.body if response.ok and isinstance(response.body, dict) else {}
            return {
                "status": body.get("status", "error"),
                "external_id": str(body.get("external_id", "")),
                "last_active_at": body.get("last_active_at"),
                "http": http_meta(response),
            }

        states = await asyncio.gather(*(sandbox_state(row) for row in rows))
        for row, state in zip(rows, states):
            row["sandbox_after_idle_wait"] = state

        async def control_state(row: dict[str, Any]) -> dict[str, Any]:
            external_id = row.get("sandbox_after_idle_wait", {}).get("external_id", "")
            if not external_id:
                return {
                    "index": row["index"],
                    "sandbox_id": "",
                    "status": "unavailable",
                    "http": {"status": 0, "duration_ms": 0, "error": "external id missing"},
                }
            response = await api_request_async(
                infrastructure.control_url,
                "GET",
                f"/v1/sandboxes/{external_id}",
                token=infrastructure.control_token,
                timeout=10,
            )
            body = response.body if response.ok and isinstance(response.body, dict) else {}
            return {
                "index": row["index"],
                "sandbox_id": external_id,
                "runner_id": json_field(body, "runner_id", "RunnerID"),
                "status": json_field(body, "status", "Status"),
                "cpu": json_field(body, "cpu", "CPU"),
                "memory_mb": json_field(body, "memory_mb", "MemoryMB"),
                "disk_gb": json_field(body, "disk_gb", "DiskGB"),
                "auto_sleep_after_seconds": json_field(
                    body, "auto_sleep_after_seconds", "AutoSleepAfterSeconds"
                ),
                "last_wake_at": json_field(body, "last_wake_at", "LastWakeAt"),
                "stopped_at": json_field(body, "stopped_at", "StoppedAt"),
                "sleep_after_at": json_field(body, "sleep_after_at", "SleepAfterAt"),
                "http": http_meta(response),
            }

        placements = await asyncio.gather(*(control_state(row) for row in rows))
        result["placements"] = placements
        for row, placement in zip(rows, placements):
            row["control_after_idle_wait"] = {
                "status": placement.get("status", "unavailable"),
                "http": placement.get("http", {}),
            }
    finally:
        stop_monitor.set()
        await asyncio.gather(*monitor_tasks, return_exceptions=True)

    rows = result["sessions"]
    rounds = [round_result for row in rows for round_result in row.get("rounds", [])]
    statuses: dict[str, int] = {}
    control_statuses: dict[str, int] = {}
    create_statuses: dict[str, int] = {}
    for row in rows:
        status = str(row.get("sandbox_after_idle_wait", {}).get("status", "unavailable"))
        statuses[status] = statuses.get(status, 0) + 1
        control_status = str(row.get("control_after_idle_wait", {}).get("status", "unavailable"))
        control_statuses[control_status] = control_statuses.get(control_status, 0) + 1
        create_status = str(row.get("session_create", {}).get("status", 0))
        create_statuses[create_status] = create_statuses.get(create_status, 0) + 1
    placement_counts: dict[str, int] = {}
    for placement in result.get("placements", []):
        runner_id = str(placement.get("runner_id") or "unknown")
        placement_counts[runner_id] = placement_counts.get(runner_id, 0) + 1
    assertions = burst_assertions(rows, args.sessions)
    waves: dict[str, Any] = {}
    for wave in (1, 2):
        wave_rows = [row for row in rows if row.get("wave") == wave]
        starts = [float(row["actual_start_offset_ms"]) for row in wave_rows]
        waves[str(wave)] = {
            "sessions": len(wave_rows),
            "sessions_created": sum(1 for row in wave_rows if row.get("session_id")),
            "start_offsets_ms": numeric_summary(starts),
            "start_skew_ms": round(max(starts) - min(starts), 2) if starts else None,
            "session_create_latency": latency_summary(
                [
                    float(row["session_create"]["duration_ms"])
                    for row in wave_rows
                    if row.get("session_create")
                ]
            ),
        }
    response_by_round: dict[str, Any] = {}
    for round_number in range(1, args.max_messages + 1):
        values = [
            float(value["response"]["latency_ms"])
            for value in rounds
            if value.get("round") == round_number
            and value.get("response", {}).get("outcome") == "responded"
        ]
        if values:
            response_by_round[str(round_number)] = latency_summary(values)
    created_rows = [row for row in rows if row.get("session_id")]
    completed_created_sessions = sum(
        1
        for row in created_rows
        if len(row.get("rounds", [])) == int(row.get("rounds_planned") or 0)
        and all(
            value.get("accepted")
            and value.get("response", {}).get("outcome") == "responded"
            and value.get("response", {}).get("exact_match")
            for value in row.get("rounds", [])
        )
    )
    result["summary"] = {
        "agents_created": sum(1 for row in rows if row.get("agent_id")),
        "sessions_created": sum(1 for row in rows if row.get("session_id")),
        "messages_planned": sum(int(row.get("rounds_planned") or 0) for row in rows),
        "messages_attempted": len(rounds),
        "messages_accepted": sum(1 for value in rounds if value.get("accepted")),
        "responses_received": sum(
            1 for value in rounds if value.get("response", {}).get("outcome") == "responded"
        ),
        "exact_outputs": sum(1 for value in rounds if value.get("response", {}).get("exact_match")),
        "created_sessions_completed_full_plan": completed_created_sessions,
        "session_create_http_statuses": create_statuses,
        "sandbox_statuses_after_idle_wait": statuses,
        "control_statuses_after_idle_wait": control_statuses,
        "placements": placement_counts,
        "waves": waves,
        "session_create_latency": latency_summary(
            [float(row["session_create"]["duration_ms"]) for row in rows if row.get("session_create")]
        ),
        "message_post_latency": latency_summary(
            [
                float(value["post"]["duration_ms"])
                for value in rounds
                if value.get("kind") == "message" and value.get("post")
            ]
        ),
        "model_response_latency": latency_summary(
            [
                float(value["response"]["latency_ms"])
                for value in rounds
                if value.get("response", {}).get("outcome") == "responded"
            ]
        ),
        "model_response_latency_by_round": response_by_round,
        "runner_capacity": summarize_runner_samples(result["runner_samples"]),
        "host_metrics": summarize_host_metrics(result["host_samples"]),
        "monitor_errors": len(result["monitor_errors"]),
        "assertions": assertions,
        "passed": all(assertions.values()) and not result["monitor_errors"],
    }
    result["finished_at"] = utc_now()
    write_private_json(output, result)
    print(json.dumps({"result_file": str(output), "run_id": run_id, "summary": result["summary"]}, indent=2))
    return 0 if result["summary"]["passed"] else 1


def command_burst_all(args: argparse.Namespace) -> int:
    with BurstInfrastructure(args) as infrastructure:
        print(f"[{utc_now()}] ensuring {args.account_count} reusable staging accounts", flush=True)
        if asyncio.run(command_accounts(args)) != 0:
            return 1
        if command_approve(args) != 0:
            return 1
        if asyncio.run(command_prepare(args)) != 0:
            return 1
        async def run_with_pool() -> int:
            loop = asyncio.get_running_loop()
            executor = concurrent.futures.ThreadPoolExecutor(max_workers=args.http_workers)
            loop.set_default_executor(executor)
            return await command_burst_run(args, infrastructure)

        return asyncio.run(run_with_pool())


def add_common(parser: argparse.ArgumentParser) -> None:
    parser.add_argument("--base-url", default="https://staging.api.usehivy.com")
    parser.add_argument("--accounts-file", type=Path, default=DEFAULT_ACCOUNTS)
    parser.add_argument("--account-count", type=int, default=5)
    parser.add_argument("--model", default="mimo-v2.5-pro")


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    subparsers = parser.add_subparsers(dest="command", required=True)

    accounts = subparsers.add_parser("accounts", help="create persistent account fixtures and register them")
    add_common(accounts)
    accounts.add_argument("--email-domain", default="example.test")

    approve = subparsers.add_parser("approve", help="confirm fixture emails directly in staging PostgreSQL")
    add_common(approve)
    approve.add_argument("--kubeconfig", type=Path, default=DEFAULT_KUBECONFIG)
    approve.add_argument("--namespace", default="staging")
    approve.add_argument("--postgres-pod", default="backend-postgres-1")
    approve.add_argument("--postgres-database", default="hivy")

    prepare = subparsers.add_parser("prepare", help="authenticate accounts and provision one reusable team each")
    add_common(prepare)

    run = subparsers.add_parser("run", help="run the concurrent session/message/idle lifecycle test")
    add_common(run)
    run.add_argument("--sessions", type=int, default=10)
    run.add_argument("--sandbox-size", default="small")
    run.add_argument("--initial-message", default="hello hivy")
    run.add_argument("--followup-message", default="list your tools")
    run.add_argument("--create-concurrency", type=int, default=10)
    run.add_argument("--create-rate-per-second", type=float, default=0.0)
    run.add_argument("--max-starts-per-minute", type=float, default=0.0)
    run.add_argument("--create-timeout", type=float, default=180.0)
    run.add_argument("--response-timeout", type=float, default=240.0)
    run.add_argument("--poll-interval", type=float, default=2.0)
    run.add_argument("--idle-wait-seconds", type=float, default=22.0)
    run.add_argument("--results-dir", type=Path, default=DEFAULT_RESULTS)

    burst = subparsers.add_parser(
        "burst",
        help="prepare accounts and run the monitored two-wave conversational burst test",
    )
    add_common(burst)
    burst.add_argument("--sessions", type=int, default=100)
    burst.add_argument("--first-wave-size", type=int, default=50)
    burst.add_argument("--min-messages", type=int, default=3)
    burst.add_argument("--max-messages", type=int, default=5)
    burst.add_argument("--wave-gap-min", type=float, default=3.0)
    burst.add_argument("--wave-gap-max", type=float, default=5.0)
    burst.add_argument("--idle-wait-min", type=float, default=20.0)
    burst.add_argument("--idle-wait-max", type=float, default=25.0)
    burst.add_argument("--sandbox-size", default="nano")
    burst.add_argument("--agent-create-concurrency", type=int, default=50)
    burst.add_argument("--http-workers", type=int, default=160)
    burst.add_argument("--create-timeout", type=float, default=180.0)
    burst.add_argument("--response-timeout", type=float, default=240.0)
    burst.add_argument("--poll-interval", type=float, default=1.0)
    burst.add_argument("--monitor-interval", type=float, default=1.0)
    burst.add_argument("--seed", type=int)
    burst.add_argument("--results-dir", type=Path, default=DEFAULT_RESULTS)
    burst.add_argument("--email-domain", default="example.test")
    burst.add_argument("--kubeconfig", type=Path, default=DEFAULT_KUBECONFIG)
    burst.add_argument("--namespace", default="staging")
    burst.add_argument("--postgres-pod", default="backend-postgres-1")
    burst.add_argument("--postgres-database", default="hivy")
    burst.add_argument("--kube-local-port", type=int, default=16443)
    burst.add_argument(
        "--kube-tunnel-host",
        default=os.getenv("HIVY_LOADTEST_KUBE_TUNNEL_HOST", ""),
    )
    burst.add_argument(
        "--ssh-key",
        type=Path,
        default=Path(os.getenv("HIVY_LOADTEST_SSH_KEY", "~/.ssh/usehivy")).expanduser(),
    )
    burst.add_argument(
        "--runner-host",
        action="append",
        default=[],
        help="runner SSH target in name=address form; repeat for every runner",
    )
    return parser


def main() -> int:
    args = build_parser().parse_args()
    if args.account_count < 1:
        raise ValueError("account-count must be at least 1")
    if args.command == "accounts":
        return asyncio.run(command_accounts(args))
    if args.command == "approve":
        return command_approve(args)
    if args.command == "prepare":
        return asyncio.run(command_prepare(args))
    if args.command == "run":
        if args.sessions < 1 or args.create_concurrency < 1:
            raise ValueError("sessions and create-concurrency must be at least 1")
        return asyncio.run(command_run(args))
    if args.command == "burst":
        if args.sessions < 2 or not 0 < args.first_wave_size < args.sessions:
            raise ValueError("burst requires at least two sessions and a first-wave-size within the total")
        if args.min_messages < 1 or args.max_messages < args.min_messages:
            raise ValueError("message bounds must satisfy 1 <= min-messages <= max-messages")
        if args.wave_gap_min < 0 or args.wave_gap_max < args.wave_gap_min:
            raise ValueError("wave gap bounds are invalid")
        if args.idle_wait_min < 0 or args.idle_wait_max < args.idle_wait_min:
            raise ValueError("idle wait bounds are invalid")
        if args.http_workers < args.first_wave_size:
            raise ValueError("http-workers must be at least first-wave-size for a concurrent wave")
        if not args.runner_host:
            raise ValueError("provide --runner-host name=address for each runner")
        parse_runner_hosts(args.runner_host)
        return command_burst_all(args)
    raise AssertionError(f"unsupported command: {args.command}")


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except KeyboardInterrupt:
        raise SystemExit(130) from None
    except Exception as exc:
        print(f"error: {type(exc).__name__}: {exc}", file=sys.stderr)
        raise SystemExit(1) from None
