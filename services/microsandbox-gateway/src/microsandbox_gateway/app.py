from __future__ import annotations

import argparse
import asyncio
import json
import logging
import re
import time
from dataclasses import dataclass, field
from typing import Any

from aiohttp import web

from .config import Config
from .control import ControlClient
from .store import (
    RedisStore,
    Store,
    delete_route,
    load_route,
    mark_activity,
    normalize_route,
    route_running,
    route_stale,
    store_route,
    upstream_for,
    wake_lock_key,
)

LOGGER = logging.getLogger("microsandbox_gateway")


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
    request_id = request.headers.get("X-Request-Id") or request.headers.get("X-Forwarded-Request-Id") or ""
    host = request.headers.get("X-Forwarded-Host") or request.headers.get("Host") or ""
    port, sandbox_id = parse_preview_host(host, cfg.base_domain)
    if not sandbox_id or not port:
        state.metrics.inc("lookup_invalid_host")
        return json_response(404, {"error": "invalid preview host"})

    try:
        route = await resolve_route(state, sandbox_id, port, request_id)
    except TimeoutError as exc:
        state.metrics.inc("lookup_wake_timeout")
        LOGGER.warning("sandbox wake timed out", extra={"sandbox_id": sandbox_id, "port": port, "error": str(exc)})
        return json_response(503, {"error": "sandbox wake timed out"}, {"Retry-After": "2"})
    except Exception as exc:
        state.metrics.inc("lookup_error")
        LOGGER.warning("sandbox lookup failed", extra={"sandbox_id": sandbox_id, "port": port, "error": str(exc)})
        return json_response(503, {"error": "sandbox unavailable", "detail": str(exc)}, {"Retry-After": "2"})

    upstream = upstream_for(route, port)
    if not upstream:
        state.metrics.inc("lookup_missing_port")
        return json_response(404, {"error": "port not previewable"})
    await report_activity(state, sandbox_id)
    state.metrics.inc("lookup_ok")
    return web.Response(status=204, headers={"X-Microsandbox-Upstream": upstream})


async def resolve_route(state: AppState, sandbox_id: str, port: int, request_id: str) -> dict[str, Any]:
    cfg = state.cfg
    route = await load_route(state.store, sandbox_id)
    runtime_port = port == cfg.runtime_port
    if runtime_port:
        return await ensure_ready(state, sandbox_id, port, "runtime_ready", request_id)

    if route is None or route_stale(route, cfg.route_stale_seconds) or not upstream_for(route, port):
        try:
            route = await state.control.route(sandbox_id)
            await store_route(state.store, route)
            state.metrics.inc("control_route_lookup")
        except Exception:
            if route is None:
                raise

    if not route_running(route):
        route = await ensure_ready(state, sandbox_id, port, "port_open", request_id)

    return route


async def ensure_ready(
    state: AppState,
    sandbox_id: str,
    port: int,
    readiness: str,
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
                readiness,
                cfg.wake_timeout_seconds,
                request_id,
            )
            await store_route(state.store, route)
            state.metrics.inc("ensure_ready_owner")
            return route
        finally:
            await state.store.delete(lock_key)

    state.metrics.inc("ensure_ready_waiter")
    deadline = time.time() + cfg.wake_timeout_seconds
    while time.time() < deadline:
        route = await load_route(state.store, sandbox_id)
        if route_running(route) and upstream_for(route, port):
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
                    readiness,
                    cfg.wake_timeout_seconds,
                    request_id,
                )
                await store_route(state.store, route)
                state.metrics.inc("ensure_ready_takeover")
                return route
            finally:
                await state.store.delete(lock_key)
        await asyncio.sleep(0.25)
    raise TimeoutError(f"waited {cfg.wake_timeout_seconds}s for {sandbox_id}")


async def report_activity(state: AppState, sandbox_id: str) -> None:
    should_report = await mark_activity(state.store, sandbox_id, state.cfg.activity_debounce_seconds)
    if not should_report:
        return

    async def send() -> None:
        try:
            await state.control.activity(sandbox_id, "gateway")
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
        for route in routes:
            await store_route(request.app[STATE_KEY].store, route)
        return json_response(200, {"stored": len(routes)})
    route = normalize_route(body, sandbox_id)
    await store_route(request.app[STATE_KEY].store, route)
    return json_response(200, route)


async def delete_route_handler(request: web.Request) -> web.Response:
    if not require_admin(request):
        return json_response(401, {"error": "unauthorized"})
    deleted = await delete_route(request.app[STATE_KEY].store, request.match_info["sandbox_id"])
    return json_response(200, {"deleted": deleted})


async def cleanup_resources(app: web.Application) -> None:
    state: AppState = app[STATE_KEY]
    store_close = getattr(state.store, "close", None)
    if store_close is not None:
        await store_close()
    control_close = getattr(state.control, "close", None)
    if control_close is not None:
        await control_close()


def create_app(cfg: Config, store: Store | None = None, control: ControlClient | None = None) -> web.Application:
    app = web.Application()
    app[STATE_KEY] = AppState(
        cfg=cfg,
        store=store or RedisStore(cfg.redis_url),
        control=control or ControlClient(cfg.control_url, cfg.control_token),
    )
    app.router.add_get("/health", health)
    app.router.add_get("/metrics", metrics)
    app.router.add_get("/v1/lookup", lookup)
    app.router.add_get("/v1/routes/{sandbox_id}", get_route)
    app.router.add_put("/v1/routes/{sandbox_id}", put_route)
    app.router.add_post("/v1/routes/{sandbox_id}", put_route)
    app.router.add_post("/v1/routes/bulk", put_route)
    app.router.add_delete("/v1/routes/{sandbox_id}", delete_route_handler)
    app.on_cleanup.append(cleanup_resources)
    return app


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--log-level", default="INFO")
    args = parser.parse_args()
    logging.basicConfig(level=getattr(logging, args.log_level.upper(), logging.INFO), format="%(message)s")
    cfg = Config.from_env()
    if not cfg.admin_token:
        raise SystemExit("HIVY_MICROSANDBOX_PREVIEW_CACHE_TOKEN is required")
    web.run_app(create_app(cfg), host=cfg.addr, port=cfg.port)


if __name__ == "__main__":
    main()
