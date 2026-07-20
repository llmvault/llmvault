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
"""

from __future__ import annotations

import argparse
import asyncio
import json
import math
import os
import secrets
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
            after_sequence=row["followup"]["sequence"],
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
    raise AssertionError(f"unsupported command: {args.command}")


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except KeyboardInterrupt:
        raise SystemExit(130) from None
    except Exception as exc:
        print(f"error: {type(exc).__name__}: {exc}", file=sys.stderr)
        raise SystemExit(1) from None
