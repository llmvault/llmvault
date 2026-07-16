from __future__ import annotations

import json
import time
from datetime import datetime
from typing import Any, Protocol

import redis.asyncio as redis

ROUTE_PREFIX = "microsandbox:preview-route:"
ALIAS_PREFIX = "microsandbox:preview-alias:"
ACTIVITY_DEBOUNCE_PREFIX = "microsandbox:preview-activity-debounce:"
WAKE_LOCK_PREFIX = "microsandbox:preview-wake:"


class Store(Protocol):
    async def ping(self) -> bool: ...
    async def get_json(self, key: str) -> dict[str, Any] | None: ...
    async def set_json(self, key: str, value: dict[str, Any], ttl: int = 0) -> None: ...
    async def delete(self, key: str) -> int: ...
    async def set_value(self, key: str, value: str, ttl: int = 0, nx: bool = False) -> bool: ...
    async def set_route_json(self, key: str, value: dict[str, Any], ttl: int = 0) -> bool: ...
    async def invalidate_route_json(
        self, key: str, generation: int, guest_port: int, upstream: str, now: int
    ) -> dict[str, Any] | None: ...


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

    async def set_route_json(self, key: str, value: dict[str, Any], ttl: int = 0) -> bool:
        raw = json.dumps(value, separators=(",", ":"))
        result = await self._client.eval(
            """
            local current_raw = redis.call('GET', KEYS[1])
            if current_raw then
                local current = cjson.decode(current_raw)
                local incoming = cjson.decode(ARGV[1])
                local current_generation = tonumber(current['route_generation'] or 0)
                local incoming_generation = tonumber(incoming['route_generation'] or 0)
                local current_status = tostring(current['status'] or 'running')
                local incoming_status = tostring(incoming['status'] or 'running')
                local current_running = current_status == 'running' or current_status == 'ready' or current_status == ''
                local incoming_running = incoming_status == 'running' or incoming_status == 'ready' or incoming_status == ''
                if incoming_generation < current_generation then
                    return 0
                end
                if incoming_generation == current_generation and not current_running and incoming_running then
                    return 0
                end
            end
            local ttl = tonumber(ARGV[2])
            if ttl and ttl > 0 then
                redis.call('SET', KEYS[1], ARGV[1], 'EX', ttl)
            else
                redis.call('SET', KEYS[1], ARGV[1])
            end
            return 1
            """,
            1,
            key,
            raw,
            ttl,
        )
        return bool(result)

    async def invalidate_route_json(
        self, key: str, generation: int, guest_port: int, upstream: str, now: int
    ) -> dict[str, Any] | None:
        result = await self._client.eval(
            """
            local current_raw = redis.call('GET', KEYS[1])
            if not current_raw then
                return ''
            end
            local current = cjson.decode(current_raw)
            if tonumber(current['route_generation'] or 0) ~= tonumber(ARGV[1]) then
                return ''
            end
            local status = tostring(current['status'] or 'running')
            if status ~= 'running' and status ~= 'ready' and status ~= '' then
                return ''
            end
            local route_upstream = tostring((current['upstreams'] or {})[ARGV[2]] or '')
            route_upstream = string.gsub(route_upstream, '^http://', '')
            route_upstream = string.gsub(route_upstream, '^https://', '')
            route_upstream = string.gsub(route_upstream, '/$', '')
            local expected_upstream = string.gsub(ARGV[3], '^http://', '')
            expected_upstream = string.gsub(expected_upstream, '^https://', '')
            expected_upstream = string.gsub(expected_upstream, '/$', '')
            if route_upstream == '' or route_upstream ~= expected_upstream then
                return ''
            end
            local now = tonumber(ARGV[4])
            current['status'] = 'unavailable'
            current['lease_expires_at'] = now - 1
            current['next_activity_after'] = now - 1
            current['updated_at'] = now
            local tombstone = cjson.encode(current)
            redis.call('SET', KEYS[1], tombstone)
            return tombstone
            """,
            1,
            key,
            generation,
            str(guest_port),
            upstream,
            now,
        )
        if not result:
            return None
        return json.loads(result)

    async def close(self) -> None:
        await self._client.aclose()


def route_key(sandbox_id: str) -> str:
    return ROUTE_PREFIX + sandbox_id


def activity_debounce_key(sandbox_id: str) -> str:
    return ACTIVITY_DEBOUNCE_PREFIX + sandbox_id


def wake_lock_key(sandbox_id: str) -> str:
    return WAKE_LOCK_PREFIX + sandbox_id


def alias_key(alias: str) -> str:
    return ALIAS_PREFIX + alias.strip().lower()


def normalize_alias(raw: dict[str, Any], alias: str | None = None) -> dict[str, Any]:
    mapping = dict(raw)
    if alias:
        mapping["alias"] = alias
    name = str(mapping.get("alias", "")).strip().lower()
    if not name:
        raise ValueError("alias is required")
    sandbox_id = str(mapping.get("sandbox_id", "")).strip()
    if not sandbox_id:
        raise ValueError("sandbox_id is required")
    port = int(mapping.get("port") or 0)
    if port <= 0 or port > 65535:
        raise ValueError("valid port is required")
    return {
        "alias": name,
        "sandbox_id": sandbox_id,
        "port": port,
        "updated_at": int(mapping.get("updated_at") or time.time()),
    }


async def load_alias(store: Store, alias: str) -> dict[str, Any] | None:
    return await store.get_json(alias_key(alias))


async def store_alias(store: Store, mapping: dict[str, Any]) -> None:
    # Alias mappings are persistent (no TTL): the alias→sandbox binding only
    # changes on repoint/delete, and a cold sandbox must remain wakeable via it.
    mapping = normalize_alias(mapping)
    await store.set_json(alias_key(mapping["alias"]), mapping)


async def delete_alias(store: Store, alias: str) -> bool:
    return bool(await store.delete(alias_key(alias)))


async def load_route(store: Store, sandbox_id: str) -> dict[str, Any] | None:
    return await store.get_json(route_key(sandbox_id))


async def store_route(store: Store, route: dict[str, Any]) -> bool:
    route = normalize_route(route)
    ttl = int(route.get("ttl_seconds") or 0)
    if ttl <= 0 and route_running(route):
        ttl = route_cache_ttl_seconds(route)
    return await store.set_route_json(route_key(route["sandbox_id"]), route, ttl)


async def invalidate_route(
    store: Store, sandbox_id: str, generation: int, guest_port: int, upstream: str
) -> dict[str, Any] | None:
    return await store.invalidate_route_json(
        route_key(sandbox_id), generation, guest_port, upstream, int(time.time())
    )


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
    route["route_generation"] = int(route.get("route_generation") or 0)
    route["updated_at"] = int(route.get("updated_at") or time.time())
    return route


def route_running(route: dict[str, Any] | None) -> bool:
    if not route:
        return False
    return (route.get("status") or "running") in ("running", "ready", "")


def route_replaces(current: dict[str, Any] | None, incoming: dict[str, Any]) -> bool:
    if not current:
        return True
    current_generation = int(current.get("route_generation") or 0)
    incoming_generation = int(incoming.get("route_generation") or 0)
    if incoming_generation < current_generation:
        return False
    if incoming_generation == current_generation and not route_running(current) and route_running(incoming):
        return False
    return True


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
