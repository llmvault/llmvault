from __future__ import annotations

import asyncio
from typing import Any

import pytest
from aiohttp.test_utils import TestClient, TestServer

from microsandbox_gateway.app import create_app, parse_preview_host
from microsandbox_gateway.config import Config
from microsandbox_gateway.store import route_key


class MemoryStore:
    def __init__(self) -> None:
        self.values: dict[str, Any] = {}

    async def ping(self) -> bool:
        return True

    async def get_json(self, key: str) -> dict[str, Any] | None:
        value = self.values.get(key)
        if value is None:
            return None
        return dict(value)

    async def set_json(self, key: str, value: dict[str, Any], ttl: int = 0) -> None:
        self.values[key] = dict(value)

    async def delete(self, key: str) -> int:
        existed = key in self.values
        self.values.pop(key, None)
        return int(existed)

    async def set_value(self, key: str, value: str, ttl: int = 0, nx: bool = False) -> bool:
        if nx and key in self.values:
            return False
        self.values[key] = value
        return True


class FakeControl:
    def __init__(self) -> None:
        self.route_calls = 0
        self.ensure_calls: list[tuple[str, int, str]] = []
        self.ensure_delay = 0.0
        self.activity_calls: list[str] = []
        self.route_payload = {
            "sandbox_id": "sbx_test",
            "status": "running",
            "upstreams": {"3000": "http://10.0.0.2:43000", "7080": "http://10.0.0.2:47080"},
            "updated_at": 9999999999,
        }

    async def route(self, sandbox_id: str) -> dict[str, Any]:
        self.route_calls += 1
        return dict(self.route_payload, sandbox_id=sandbox_id)

    async def ensure_ready(
        self,
        sandbox_id: str,
        guest_port: int,
        readiness: str,
        timeout_seconds: int,
        request_id: str,
    ) -> dict[str, Any]:
        if self.ensure_delay > 0:
            await asyncio.sleep(self.ensure_delay)
        self.ensure_calls.append((sandbox_id, guest_port, readiness))
        return dict(self.route_payload, sandbox_id=sandbox_id)

    async def activity(self, sandbox_id: str, source: str = "gateway") -> None:
        self.activity_calls.append(sandbox_id)


def cfg() -> Config:
    return Config(
        addr="127.0.0.1",
        port=8091,
        redis_url="redis://127.0.0.1:6379/0",
        admin_token="admin-token",
        base_domain="preview.test",
        control_url="http://control.test",
        control_token="control-token",
        wake_timeout_seconds=2,
        activity_debounce_seconds=5,
        route_stale_seconds=30,
        runtime_port=7080,
    )


@pytest.fixture
async def client() -> tuple[TestClient, MemoryStore, FakeControl]:
    store = MemoryStore()
    control = FakeControl()
    app = create_app(cfg(), store=store, control=control)  # type: ignore[arg-type]
    test_client = TestClient(TestServer(app))
    await test_client.start_server()
    try:
        yield test_client, store, control
    finally:
        await test_client.close()


def test_parse_preview_host() -> None:
    assert parse_preview_host("3000-sbx_123.preview.test", "preview.test") == (3000, "sbx_123")
    assert parse_preview_host("preview.test", "preview.test") == (None, None)


async def test_lookup_uses_control_fallback_on_cache_miss(client: tuple[TestClient, MemoryStore, FakeControl]) -> None:
    test_client, _store, control = client
    resp = await test_client.get(
        "/v1/lookup",
        headers={
            "Host": "3000-sbx_test.preview.test",
            "Authorization": "Bearer runtime-secret-that-gateway-must-ignore",
        },
    )
    assert resp.status == 204
    assert resp.headers["X-Microsandbox-Upstream"] == "10.0.0.2:43000"
    assert control.route_calls == 1
    assert control.ensure_calls == []


async def test_lookup_uses_control_fallback_when_cached_route_lacks_port(
    client: tuple[TestClient, MemoryStore, FakeControl],
) -> None:
    test_client, store, control = client
    await store.set_json(
        route_key("sbx_test"),
        {
            "sandbox_id": "sbx_test",
            "status": "running",
            "upstreams": {"5173": "http://10.0.0.2:45173"},
            "updated_at": 9999999999,
        },
    )

    resp = await test_client.get("/v1/lookup", headers={"Host": "3000-sbx_test.preview.test"})

    assert resp.status == 204
    assert resp.headers["X-Microsandbox-Upstream"] == "10.0.0.2:43000"
    assert control.route_calls == 1


async def test_lookup_always_ensures_runtime_port(client: tuple[TestClient, MemoryStore, FakeControl]) -> None:
    test_client, store, control = client
    await store.set_json(route_key("sbx_test"), control.route_payload)

    resp = await test_client.get("/v1/lookup", headers={"Host": "7080-sbx_test.preview.test"})

    assert resp.status == 204
    assert resp.headers["X-Microsandbox-Upstream"] == "10.0.0.2:47080"
    assert control.ensure_calls == [("sbx_test", 7080, "runtime_ready")]


async def test_stopped_route_wake_is_singleflight(client: tuple[TestClient, MemoryStore, FakeControl]) -> None:
    test_client, store, control = client
    control.ensure_delay = 0.05
    await store.set_json(route_key("sbx_test"), dict(control.route_payload, status="stopped"))

    async def lookup() -> str:
        resp = await test_client.get("/v1/lookup", headers={"Host": "3000-sbx_test.preview.test"})
        assert resp.status == 204
        return resp.headers["X-Microsandbox-Upstream"]

    upstreams = await asyncio.gather(*(lookup() for _ in range(5)))

    assert upstreams == ["10.0.0.2:43000"] * 5
    assert control.ensure_calls == [("sbx_test", 3000, "port_open")]


async def test_admin_route_round_trip(client: tuple[TestClient, MemoryStore, FakeControl]) -> None:
    test_client, _store, _control = client
    payload = {
        "sandbox_id": "sbx_admin",
        "status": "running",
        "upstreams": {"3000": "http://10.0.0.3:43000"},
    }
    put = await test_client.put(
        "/v1/routes/sbx_admin",
        headers={"Authorization": "Bearer admin-token"},
        json=payload,
    )
    assert put.status == 200

    get = await test_client.get("/v1/routes/sbx_admin", headers={"Authorization": "Bearer admin-token"})
    assert get.status == 200
    body = await get.json()
    assert body["upstreams"]["3000"] == "http://10.0.0.3:43000"
