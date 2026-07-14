from __future__ import annotations

import argparse
import asyncio
import json
import logging
import os
import re
import time
from collections import OrderedDict
from dataclasses import dataclass, field
from typing import Any

from aiohttp import web
from posthog import Posthog

from .config import Config
from .control import ControlClient
from .store import (
    RedisStore,
    Store,
    acquire_activity_report,
    delete_alias,
    delete_route,
    load_alias,
    load_route,
    normalize_alias,
    normalize_route,
    route_activity_due,
    route_lease_valid,
    route_running,
    store_alias,
    store_route,
    upstream_for,
    wake_lock_key,
)

LOGGER = logging.getLogger("microsandbox_gateway")

_LOG_RECORD_BASE_FIELDS = set(logging.makeLogRecord({}).__dict__)


def _init_posthog() -> Posthog | None:
    token = os.getenv("POSTHOG_PROJECT_TOKEN")
    if not token:
        return None
    return Posthog(
        token,
        host=os.getenv("POSTHOG_HOST", "https://us.i.posthog.com"),
        enable_exception_autocapture=True,
    )


class JsonLogFormatter(logging.Formatter):
    def format(self, record: logging.LogRecord) -> str:
        payload: dict[str, Any] = {
            "time": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime(record.created)),
            "level": record.levelname.lower(),
            "message": record.getMessage(),
            "logger": record.name,
        }
        for key, value in record.__dict__.items():
            if key in _LOG_RECORD_BASE_FIELDS or key.startswith("_"):
                continue
            try:
                json.dumps(value)
                payload[key] = value
            except TypeError:
                payload[key] = str(value)
        if record.exc_info:
            payload["exception"] = self.formatException(record.exc_info)
        return json.dumps(payload, separators=(",", ":"), sort_keys=True)


@dataclass
class Metrics:
    counters: dict[str, int] = field(default_factory=dict)

    def inc(self, name: str) -> None:
        self.counters[name] = self.counters.get(name, 0) + 1

    def text(self) -> str:
        lines = []
        for name, value in sorted(self.counters.items()):
            lines.append(f"hivy_microsandbox_gateway_{name}_total {value}")
        return "\n".join(lines) + ("\n" if lines else "")


@dataclass
class AppState:
    cfg: Config
    store: Store
    control: ControlClient
    metrics: Metrics = field(default_factory=Metrics)
    posthog_client: Posthog | None = field(default=None)
    route_cache: LocalRouteCache = field(init=False)

    def __post_init__(self) -> None:
        self.route_cache = LocalRouteCache(self.cfg.route_cache_size)


@dataclass
class ResolveResult:
    route: dict[str, Any]
    source: str


class LocalRouteCache:
    def __init__(self, max_size: int) -> None:
        self.max_size = max(1, max_size)
        self._routes: OrderedDict[str, dict[str, Any]] = OrderedDict()

    def get(self, sandbox_id: str) -> dict[str, Any] | None:
        route = self._routes.get(sandbox_id)
        if route is None:
            return None
        self._routes.move_to_end(sandbox_id)
        return dict(route)

    def set(self, route: dict[str, Any]) -> None:
        sandbox_id = str(route.get("sandbox_id") or "")
        if not sandbox_id:
            return
        self._routes[sandbox_id] = dict(route)
        self._routes.move_to_end(sandbox_id)
        while len(self._routes) > self.max_size:
            self._routes.popitem(last=False)

    def delete(self, sandbox_id: str) -> None:
        self._routes.pop(sandbox_id, None)


STATE_KEY = web.AppKey("state", AppState)


def json_response(status: int, payload: dict[str, Any] | None = None, headers: dict[str, str] | None = None) -> web.Response:
    body = b""
    if payload is not None:
        body = json.dumps(payload, separators=(",", ":")).encode("utf-8")
    return web.Response(
        status=status,
        body=body,
        headers=headers,
        content_type="application/json" if payload is not None else "text/plain",
    )


def bearer(request: web.Request) -> str:
    auth = request.headers.get("Authorization", "")
    if auth.startswith("Bearer "):
        return auth[len("Bearer ") :]
    return ""


def require_admin(request: web.Request) -> bool:
    cfg: Config = request.app[STATE_KEY].cfg
    return bool(cfg.admin_token and bearer(request) == cfg.admin_token)


def parse_preview_host(host: str, base_domain: str) -> tuple[int | None, str | None]:
    host = host.split(":", 1)[0].strip(".").lower()
    suffix = "." + base_domain.strip(".").lower()
    if not host.endswith(suffix):
        return None, None
    first_label = host[: -len(suffix)].split(".")[-1]
    match = re.match(r"^([0-9]{1,5})-(.+)$", first_label)
    if not match:
        return None, None
    port = int(match.group(1))
    if port <= 0 or port > 65535:
        return None, None
    return port, match.group(2)


def parse_alias_host(host: str, base_domain: str) -> str | None:
    """Extract an alias label from an alias host `{alias}.{base_domain}`.

    Returns None when the host is not on the base domain, is the bare domain,
    carries extra labels, or looks like a `{port}-{sandbox}` preview host.
    """
    host = host.split(":", 1)[0].strip(".").lower()
    suffix = "." + base_domain.strip(".").lower()
    if not host.endswith(suffix):
        return None
    label = host[: -len(suffix)]
    if not label or "." in label:
        return None
    if re.match(r"^[0-9]{1,5}-", label):
        return None
    return label


async def health(request: web.Request) -> web.Response:
    state: AppState = request.app[STATE_KEY]
    try:
        await state.store.ping()
    except Exception as exc:
        return json_response(503, {"status": "error", "error": str(exc)})
    return json_response(200, {"status": "ok"})


async def metrics(request: web.Request) -> web.Response:
    state: AppState = request.app[STATE_KEY]
    return web.Response(text=state.metrics.text(), content_type="text/plain")


async def lookup(request: web.Request) -> web.Response:
    state: AppState = request.app[STATE_KEY]
    cfg = state.cfg
    started = time.monotonic()
    request_id = request.headers.get("X-Request-Id") or request.headers.get("X-Forwarded-Request-Id") or ""
    host = request.headers.get("X-Forwarded-Host") or request.headers.get("Host") or ""
    port, sandbox_id = parse_preview_host(host, cfg.base_domain)
    if not sandbox_id or not port:
        # Not a {port}-{sandbox} preview host: try resolving it as an alias.
        # An alias maps to (sandbox_id, port); from there the wake/lease/activity
        # machinery is identical to the preview path (it keys on the sandbox).
        alias = parse_alias_host(host, cfg.base_domain)
        if alias:
            mapping = await load_alias(state.store, alias)
            if not mapping:
                # Redis miss: control is the source of truth for alias bindings,
                # so fall back to it and lazily backfill Redis. On a control miss
                # the mapping stays None and we keep the invalid-host behavior.
                mapping = await load_alias_from_control(state, alias)
            if mapping:
                sandbox_id = str(mapping.get("sandbox_id") or "")
                port = int(mapping.get("port") or 0)
    if not sandbox_id or not port:
        state.metrics.inc("lookup_invalid_host")
        return json_response(404, {"error": "invalid preview host"})

    try:
        result = await resolve_route(state, sandbox_id, port, request_id)
        route = result.route
    except TimeoutError as exc:
        state.metrics.inc("lookup_wake_timeout")
        LOGGER.warning("sandbox wake timed out", extra={"sandbox_id": sandbox_id, "port": port, "error": str(exc)})
        if state.posthog_client is not None:
            state.posthog_client.capture(
                distinct_id=sandbox_id,
                event="sandbox_wake_timed_out",
                properties={"port": port, "wake_timeout_seconds": state.cfg.wake_timeout_seconds},
            )
        return json_response(503, {"error": "sandbox wake timed out"}, {"Retry-After": "2"})
    except Exception as exc:
        state.metrics.inc("lookup_error")
        LOGGER.warning("sandbox lookup failed", extra={"sandbox_id": sandbox_id, "port": port, "error": str(exc)})
        if state.posthog_client is not None:
            state.posthog_client.capture(
                distinct_id=sandbox_id,
                event="sandbox_lookup_failed",
                properties={"port": port},
            )
        return json_response(503, {"error": "sandbox unavailable", "detail": str(exc)}, {"Retry-After": "2"})

    upstream = upstream_for(route, port)
    if not upstream:
        state.metrics.inc("lookup_missing_port")
        return json_response(404, {"error": "port not previewable"})
    report_activity_if_due(state, route)
    state.metrics.inc("lookup_ok")
    LOGGER.info(
        "lookup ok",
        extra={
            "lookup_result": result.source,
            "sandbox_id": sandbox_id,
            "guest_port": port,
            "request_id": request_id,
            "duration_ms": int((time.monotonic() - started) * 1000),
            "lease_seconds": remaining_lease_seconds(route),
        },
    )
    return web.Response(status=204, headers={"X-Microsandbox-Upstream": upstream})


async def load_alias_from_control(state: AppState, alias: str) -> dict[str, Any] | None:
    """Resolve an alias via the control plane on a Redis miss and lazily backfill
    the gateway store so the next lookup is a Redis hit. Returns None when control
    is unconfigured, errors, or has no such alias (in which case the caller keeps
    the invalid-host behavior)."""
    try:
        mapping = await state.control.alias(alias)
    except Exception as exc:
        state.metrics.inc("alias_control_error")
        LOGGER.warning("alias control fallback failed", extra={"alias": alias, "error": str(exc)})
        return None
    if not mapping:
        return None
    try:
        await store_alias(
            state.store,
            {
                "alias": alias,
                "sandbox_id": mapping.get("sandbox_id"),
                "port": mapping.get("port"),
            },
        )
        state.metrics.inc("alias_control_backfill")
    except Exception as exc:
        # Backfill is an optimization; resolve with the control mapping regardless.
        LOGGER.warning("alias backfill failed", extra={"alias": alias, "error": str(exc)})
    return mapping


async def resolve_route(state: AppState, sandbox_id: str, port: int, request_id: str) -> ResolveResult:
    route = state.route_cache.get(sandbox_id)
    if route_usable_for_port(route, port):
        state.metrics.inc("lookup_memory_hit")
        return ResolveResult(route=route, source="memory_hit")

    route = await load_route(state.store, sandbox_id)
    if route_usable_for_port(route, port):
        state.route_cache.set(route)
        state.metrics.inc("lookup_redis_hit")
        return ResolveResult(route=route, source="redis_hit")

    route = await ensure_ready(state, sandbox_id, port, request_id)
    return ResolveResult(route=route, source="ensure_ready")


def route_usable_for_port(route: dict[str, Any] | None, port: int) -> bool:
    return route_running(route) and bool(upstream_for(route, port)) and route_lease_valid(route)


async def ensure_ready(
    state: AppState,
    sandbox_id: str,
    port: int,
    request_id: str,
) -> dict[str, Any]:
    cfg = state.cfg
    lock_key = wake_lock_key(sandbox_id)
    acquired = await state.store.set_value(
        lock_key,
        str(int(time.time())),
        ttl=cfg.wake_timeout_seconds + 5,
        nx=True,
    )
    if acquired:
        try:
            route = await state.control.ensure_ready(
                sandbox_id,
                port,
                cfg.wake_timeout_seconds,
                request_id,
            )
            await store_route(state.store, route)
            state.route_cache.set(route)
            state.metrics.inc("ensure_ready_owner")
            if state.posthog_client is not None:
                state.posthog_client.capture(
                    distinct_id=sandbox_id,
                    event="sandbox_woken",
                    properties={"port": port, "role": "owner"},
                )
            return route
        finally:
            await state.store.delete(lock_key)

    state.metrics.inc("ensure_ready_waiter")
    deadline = time.time() + cfg.wake_timeout_seconds
    while time.time() < deadline:
        route = await load_route(state.store, sandbox_id)
        if route_usable_for_port(route, port):
            state.route_cache.set(route)
            return route
        acquired = await state.store.set_value(
            lock_key,
            str(int(time.time())),
            ttl=cfg.wake_timeout_seconds + 5,
            nx=True,
        )
        if acquired:
            try:
                route = await state.control.ensure_ready(
                    sandbox_id,
                    port,
                    cfg.wake_timeout_seconds,
                    request_id,
                )
                await store_route(state.store, route)
                state.route_cache.set(route)
                state.metrics.inc("ensure_ready_takeover")
                if state.posthog_client is not None:
                    state.posthog_client.capture(
                        distinct_id=sandbox_id,
                        event="sandbox_woken",
                        properties={"port": port, "role": "takeover"},
                    )
                return route
            finally:
                await state.store.delete(lock_key)
        await asyncio.sleep(0.25)
    raise TimeoutError(f"waited {cfg.wake_timeout_seconds}s for {sandbox_id}")


def remaining_lease_seconds(route: dict[str, Any]) -> int:
    value = route.get("lease_expires_at")
    from .store import parse_route_time

    expires_at = parse_route_time(value)
    if expires_at is None:
        return 0
    return max(0, int(expires_at - time.time()))


def report_activity_if_due(state: AppState, route: dict[str, Any]) -> None:
    if not route_activity_due(route):
        return
    sandbox_id = str(route.get("sandbox_id") or "")
    if not sandbox_id:
        return

    async def send() -> None:
        try:
            acquired = await acquire_activity_report(state.store, sandbox_id, state.cfg.activity_debounce_seconds)
            if not acquired:
                return
            routes = await state.control.activity_bulk([sandbox_id], "gateway")
            for refreshed in routes:
                await store_route(state.store, refreshed)
                state.route_cache.set(refreshed)
            state.metrics.inc("activity_reported")
        except Exception as exc:
            state.metrics.inc("activity_error")
            LOGGER.warning("activity report failed", extra={"sandbox_id": sandbox_id, "error": str(exc)})

    asyncio.create_task(send())


async def get_route(request: web.Request) -> web.Response:
    if not require_admin(request):
        return json_response(401, {"error": "unauthorized"})
    sandbox_id = request.match_info["sandbox_id"]
    route = await load_route(request.app[STATE_KEY].store, sandbox_id)
    if not route:
        return json_response(404, {"error": "route not found"})
    return json_response(200, route)


async def put_route(request: web.Request) -> web.Response:
    if not require_admin(request):
        return json_response(401, {"error": "unauthorized"})
    body = await request.json()
    sandbox_id = request.match_info.get("sandbox_id")
    if request.path == "/v1/routes/bulk":
        routes = [normalize_route(item) for item in body.get("routes", [])]
        state = request.app[STATE_KEY]
        for route in routes:
            await store_route(state.store, route)
            state.route_cache.set(route)
            if state.posthog_client is not None:
                state.posthog_client.capture(
                    distinct_id=str(route.get("sandbox_id", "")),
                    event="sandbox_route_stored",
                    properties={"bulk": True},
                )
        return json_response(200, {"stored": len(routes)})
    route = normalize_route(body, sandbox_id)
    state = request.app[STATE_KEY]
    await store_route(state.store, route)
    state.route_cache.set(route)
    if state.posthog_client is not None:
        state.posthog_client.capture(
            distinct_id=str(route.get("sandbox_id", "")),
            event="sandbox_route_stored",
            properties={"bulk": False},
        )
    return json_response(200, route)


async def delete_route_handler(request: web.Request) -> web.Response:
    if not require_admin(request):
        return json_response(401, {"error": "unauthorized"})
    sandbox_id = request.match_info["sandbox_id"]
    state = request.app[STATE_KEY]
    deleted = await delete_route(state.store, sandbox_id)
    state.route_cache.delete(sandbox_id)
    if deleted and state.posthog_client is not None:
        state.posthog_client.capture(
            distinct_id=sandbox_id,
            event="sandbox_route_deleted",
            properties={},
        )
    return json_response(200, {"deleted": deleted})


async def get_alias(request: web.Request) -> web.Response:
    if not require_admin(request):
        return json_response(401, {"error": "unauthorized"})
    alias = request.match_info["alias"]
    mapping = await load_alias(request.app[STATE_KEY].store, alias)
    if not mapping:
        return json_response(404, {"error": "alias not found"})
    return json_response(200, mapping)


async def put_alias(request: web.Request) -> web.Response:
    if not require_admin(request):
        return json_response(401, {"error": "unauthorized"})
    body = await request.json()
    state = request.app[STATE_KEY]
    if request.path == "/v1/aliases/bulk":
        mappings = [normalize_alias(item) for item in body.get("aliases", [])]
        for mapping in mappings:
            await store_alias(state.store, mapping)
            if state.posthog_client is not None:
                state.posthog_client.capture(
                    distinct_id=str(mapping.get("sandbox_id", "")),
                    event="sandbox_alias_created",
                    properties={"bulk": True},
                )
        return json_response(200, {"stored": len(mappings)})
    mapping = normalize_alias(body, request.match_info.get("alias"))
    await store_alias(state.store, mapping)
    if state.posthog_client is not None:
        state.posthog_client.capture(
            distinct_id=str(mapping.get("sandbox_id", "")),
            event="sandbox_alias_created",
            properties={"bulk": False},
        )
    return json_response(200, mapping)


async def delete_alias_handler(request: web.Request) -> web.Response:
    if not require_admin(request):
        return json_response(401, {"error": "unauthorized"})
    alias = request.match_info["alias"]
    state = request.app[STATE_KEY]
    deleted = await delete_alias(state.store, alias)
    if deleted and state.posthog_client is not None:
        state.posthog_client.capture(
            distinct_id=alias,
            event="sandbox_alias_deleted",
            properties={},
        )
    return json_response(200, {"deleted": deleted})


async def cleanup_resources(app: web.Application) -> None:
    state: AppState = app[STATE_KEY]
    store_close = getattr(state.store, "close", None)
    if store_close is not None:
        await store_close()
    control_close = getattr(state.control, "close", None)
    if control_close is not None:
        await control_close()
    if state.posthog_client is not None:
        state.posthog_client.shutdown()


def create_app(cfg: Config, store: Store | None = None, control: ControlClient | None = None) -> web.Application:
    app = web.Application()
    app[STATE_KEY] = AppState(
        cfg=cfg,
        store=store or RedisStore(cfg.redis_url),
        control=control or ControlClient(cfg.control_url, cfg.control_token),
        posthog_client=_init_posthog(),
    )
    app.router.add_get("/health", health)
    app.router.add_get("/metrics", metrics)
    app.router.add_get("/v1/lookup", lookup)
    app.router.add_get("/v1/routes/{sandbox_id}", get_route)
    app.router.add_put("/v1/routes/{sandbox_id}", put_route)
    app.router.add_post("/v1/routes/{sandbox_id}", put_route)
    app.router.add_post("/v1/routes/bulk", put_route)
    app.router.add_delete("/v1/routes/{sandbox_id}", delete_route_handler)
    app.router.add_post("/v1/aliases/bulk", put_alias)
    app.router.add_get("/v1/aliases/{alias}", get_alias)
    app.router.add_put("/v1/aliases/{alias}", put_alias)
    app.router.add_post("/v1/aliases/{alias}", put_alias)
    app.router.add_delete("/v1/aliases/{alias}", delete_alias_handler)
    app.on_cleanup.append(cleanup_resources)
    return app


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--log-level", default="INFO")
    args = parser.parse_args()
    handler = logging.StreamHandler()
    handler.setFormatter(JsonLogFormatter())
    logging.basicConfig(level=getattr(logging, args.log_level.upper(), logging.INFO), handlers=[handler], force=True)
    cfg = Config.from_env()
    if not cfg.admin_token:
        raise SystemExit("HIVY_MICROSANDBOX_PREVIEW_CACHE_TOKEN is required")
    web.run_app(create_app(cfg), host=cfg.addr, port=cfg.port)


if __name__ == "__main__":
    main()
