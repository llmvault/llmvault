from __future__ import annotations

import json
import time
from datetime import datetime
from typing import Any, Protocol

import redis.asyncio as redis

ROUTE_PREFIX = "microsandbox:preview-route:"
ACTIVITY_DEBOUNCE_PREFIX = "microsandbox:preview-activity-debounce:"
WAKE_LOCK_PREFIX = "microsandbox:preview-wake:"


class Store(Protocol):
    async def ping(self) -> bool: ...
    async def get_json(self, key: str) -> dict[str, Any] | None: ...
    async def set_json(self, key: str, value: dict[str, Any], ttl: int = 0) -> None: ...
    async def delete(self, key: str) -> int: ...
    async def set_value(self, key: str, value: str, ttl: int = 0, nx: bool = False) -> bool: ...


class RedisStore:
    def __init__(self, url: str) -> None:
        self._client = redis.Redis.from_url(url, decode_responses=True)

    async def ping(self) -> bool:
        return bool(await self._client.ping())

    async def get_json(self, key: str) -> dict[str, Any] | None:
        raw = await self._client.get(key)
        if not raw:
            return None
        return json.loads(raw)

    async def set_json(self, key: str, value: dict[str, Any], ttl: int = 0) -> None:
        raw = json.dumps(value, separators=(",", ":"))
        if ttl > 0:
            await self._client.set(key, raw, ex=ttl)
        else:
            await self._client.set(key, raw)

    async def delete(self, key: str) -> int:
        return int(await self._client.delete(key))

    async def set_value(self, key: str, value: str, ttl: int = 0, nx: bool = False) -> bool:
        result = await self._client.set(key, value, ex=ttl or None, nx=nx)
        return bool(result)

    async def close(self) -> None:
        await self._client.aclose()


def route_key(sandbox_id: str) -> str:
    return ROUTE_PREFIX + sandbox_id


def activity_debounce_key(sandbox_id: str) -> str:
    return ACTIVITY_DEBOUNCE_PREFIX + sandbox_id


def wake_lock_key(sandbox_id: str) -> str:
    return WAKE_LOCK_PREFIX + sandbox_id


async def load_route(store: Store, sandbox_id: str) -> dict[str, Any] | None:
    return await store.get_json(route_key(sandbox_id))


async def store_route(store: Store, route: dict[str, Any]) -> None:
    route = normalize_route(route)
    ttl = int(route.get("ttl_seconds") or 0)
    if ttl <= 0:
        ttl = route_cache_ttl_seconds(route)
    await store.set_json(route_key(route["sandbox_id"]), route, ttl)


async def delete_route(store: Store, sandbox_id: str) -> bool:
    return bool(await store.delete(route_key(sandbox_id)))


async def acquire_activity_report(store: Store, sandbox_id: str, debounce_seconds: int) -> bool:
    now = str(int(time.time()))
    return await store.set_value(
        activity_debounce_key(sandbox_id),
        now,
        ttl=debounce_seconds,
        nx=True,
    )


def normalize_route(raw: dict[str, Any], sandbox_id: str | None = None) -> dict[str, Any]:
    route = dict(raw)
    if sandbox_id:
        route["sandbox_id"] = sandbox_id
    sid = str(route.get("sandbox_id", "")).strip()
    if not sid:
        raise ValueError("sandbox_id is required")
    upstreams = route.get("upstreams") or {}
    normalized_upstreams: dict[str, str] = {}
    for guest_port, upstream in upstreams.items():
        key = str(int(guest_port))
        value = str(upstream).strip().rstrip("/")
        if not value:
            continue
        if not value.startswith(("http://", "https://")):
            value = "http://" + value
        normalized_upstreams[key] = value
    if not normalized_upstreams:
        raise ValueError("upstreams is required")
    route["sandbox_id"] = sid
    route["upstreams"] = normalized_upstreams
    route["status"] = str(route.get("status") or "running")
    route["updated_at"] = int(route.get("updated_at") or time.time())
    return route


def route_running(route: dict[str, Any] | None) -> bool:
    if not route:
        return False
    return (route.get("status") or "running") in ("running", "ready", "")


def route_lease_valid(route: dict[str, Any] | None, now: float | None = None) -> bool:
    if not route:
        return False
    expires_at = parse_route_time(route.get("lease_expires_at"))
    if expires_at is None:
        return False
    return expires_at > (now if now is not None else time.time())


def route_activity_due(route: dict[str, Any] | None, now: float | None = None) -> bool:
    if not route:
        return False
    next_after = parse_route_time(route.get("next_activity_after"))
    if next_after is None:
        return True
    return next_after <= (now if now is not None else time.time())


def route_cache_ttl_seconds(route: dict[str, Any]) -> int:
    expires_at = parse_route_time(route.get("lease_expires_at"))
    if expires_at is None:
        return 0
    return max(1, int(expires_at - time.time()) + 60)


def parse_route_time(value: Any) -> float | None:
    if value is None or value == "":
        return None
    if isinstance(value, (int, float)):
        return float(value)
    raw = str(value).strip()
    if not raw:
        return None
    if raw.isdigit():
        return float(raw)
    if raw.endswith("Z"):
        raw = raw[:-1] + "+00:00"
    try:
        return datetime.fromisoformat(raw).timestamp()
    except ValueError:
        return None


def upstream_for(route: dict[str, Any] | None, port: int) -> str:
    if not route:
        return ""
    upstream = (route.get("upstreams") or {}).get(str(port))
    if not upstream:
        return ""
    value = str(upstream)
    if value.startswith("http://"):
        return value.removeprefix("http://")
    if value.startswith("https://"):
        return value.removeprefix("https://")
    return value
