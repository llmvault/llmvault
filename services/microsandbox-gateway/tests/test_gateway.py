from __future__ import annotations

import asyncio
import json
import logging
import time
from typing import Any

import pytest
from aiohttp.test_utils import TestClient, TestServer

from microsandbox_gateway.app import (
    JsonLogFormatter,
    create_app,
    parse_alias_host,
    parse_preview_host,
)
from microsandbox_gateway.config import Config
from microsandbox_gateway.store import alias_key, route_key


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
        self.ensure_calls: list[tuple[str, int]] = []
        self.ensure_delay = 0.0
        self.activity_calls: list[str] = []
        self.activity_bulk_calls: list[list[str]] = []
        self.route_payload = {
            "sandbox_id": "sbx_test",
            "status": "running",
            "upstreams": {"3000": "http://10.0.0.2:43000", "7080": "http://10.0.0.2:47080"},
            "route_generation": 1,
            "lease_expires_at": int(time.time()) + 300,
            "next_activity_after": int(time.time()) + 30,
        }

    async def route(self, sandbox_id: str) -> dict[str, Any]:
        self.route_calls += 1
        return dict(self.route_payload, sandbox_id=sandbox_id)

    async def ensure_ready(
        self,
        sandbox_id: str,
        guest_port: int,
        timeout_seconds: int,
        request_id: str,
    ) -> dict[str, Any]:
        if self.ensure_delay > 0:
            await asyncio.sleep(self.ensure_delay)
        self.ensure_calls.append((sandbox_id, guest_port))
        return dict(self.route_payload, sandbox_id=sandbox_id)

    async def activity(self, sandbox_id: str, source: str = "gateway") -> None:
        self.activity_calls.append(sandbox_id)

    async def activity_bulk(self, sandbox_ids: list[str], source: str = "gateway") -> list[dict[str, Any]]:
        self.activity_bulk_calls.append(list(sandbox_ids))
        refreshed = []
        for sandbox_id in sandbox_ids:
            refreshed.append(
                dict(
                    self.route_payload,
                    sandbox_id=sandbox_id,
                    lease_expires_at=int(time.time()) + 300,
                    next_activity_after=int(time.time()) + 30,
                )
            )
        return refreshed


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
        activity_debounce_seconds=30,
        route_cache_size=100,
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


def test_parse_alias_host() -> None:
    assert parse_alias_host("my-app.preview.test", "preview.test") == "my-app"
    # bare domain is not an alias
    assert parse_alias_host("preview.test", "preview.test") is None
    # preview {port}-{sandbox} hosts must not be treated as aliases
    assert parse_alias_host("3000-sbx_123.preview.test", "preview.test") is None
    # extra labels are rejected
    assert parse_alias_host("a.b.preview.test", "preview.test") is None
    # off-domain host
    assert parse_alias_host("my-app.other.test", "preview.test") is None


async def test_lookup_resolves_alias_to_cached_route(
    client: tuple[TestClient, MemoryStore, FakeControl],
) -> None:
    test_client, store, control = client
    await store.set_json(route_key("sbx_test"), control.route_payload)
    await store.set_json(alias_key("my-app"), {"alias": "my-app", "sandbox_id": "sbx_test", "port": 3000})

    resp = await test_client.get("/v1/lookup", headers={"Host": "my-app.preview.test"})

    assert resp.status == 204
    assert resp.headers["X-Microsandbox-Upstream"] == "10.0.0.2:43000"
    assert control.ensure_calls == []


async def test_lookup_alias_triggers_ensure_ready_on_cold_sandbox(
    client: tuple[TestClient, MemoryStore, FakeControl],
) -> None:
    test_client, store, control = client
    await store.set_json(alias_key("my-app"), {"alias": "my-app", "sandbox_id": "sbx_test", "port": 7080})

    resp = await test_client.get("/v1/lookup", headers={"Host": "my-app.preview.test"})

    assert resp.status == 204
    assert resp.headers["X-Microsandbox-Upstream"] == "10.0.0.2:47080"
    assert control.ensure_calls == [("sbx_test", 7080)]


async def test_lookup_unknown_alias_is_404(
    client: tuple[TestClient, MemoryStore, FakeControl],
) -> None:
    test_client, _store, control = client
    resp = await test_client.get("/v1/lookup", headers={"Host": "ghost-app.preview.test"})
    assert resp.status == 404
    assert control.ensure_calls == []


async def test_alias_admin_round_trip(client: tuple[TestClient, MemoryStore, FakeControl]) -> None:
    test_client, _store, _control = client
    put = await test_client.put(
        "/v1/aliases/my-app",
        headers={"Authorization": "Bearer admin-token"},
        json={"sandbox_id": "sbx_admin", "port": 8080},
    )
    assert put.status == 200
    body = await put.json()
    assert body == {
        "alias": "my-app",
        "sandbox_id": "sbx_admin",
        "port": 8080,
        "updated_at": body["updated_at"],
    }

    get = await test_client.get("/v1/aliases/my-app", headers={"Authorization": "Bearer admin-token"})
    assert get.status == 200
    assert (await get.json())["sandbox_id"] == "sbx_admin"

    bulk = await test_client.post(
        "/v1/aliases/bulk",
        headers={"Authorization": "Bearer admin-token"},
        json={"aliases": [{"alias": "app-b", "sandbox_id": "sbx_admin", "port": 3000}]},
    )
    assert bulk.status == 200
    assert (await bulk.json()) == {"stored": 1}

    deleted = await test_client.delete("/v1/aliases/my-app", headers={"Authorization": "Bearer admin-token"})
    assert deleted.status == 200
    assert (await deleted.json()) == {"deleted": True}

    missing = await test_client.get("/v1/aliases/my-app", headers={"Authorization": "Bearer admin-token"})
    assert missing.status == 404


def test_json_log_formatter_includes_lookup_fields() -> None:
    record = logging.LogRecord(
        name="microsandbox_gateway",
        level=logging.INFO,
        pathname=__file__,
        lineno=1,
        msg="lookup ok",
        args=(),
        exc_info=None,
    )
    record.lookup_result = "memory_hit"
    record.sandbox_id = "sbx_test"
    record.guest_port = 7080
    record.request_id = "req_123"
    record.duration_ms = 3
    record.lease_seconds = 120

    payload = json.loads(JsonLogFormatter().format(record))

    assert payload["message"] == "lookup ok"
    assert payload["lookup_result"] == "memory_hit"
    assert payload["sandbox_id"] == "sbx_test"
    assert payload["guest_port"] == 7080
    assert payload["request_id"] == "req_123"
    assert payload["duration_ms"] == 3
    assert payload["lease_seconds"] == 120


async def test_lookup_ensure_ready_on_cache_miss(client: tuple[TestClient, MemoryStore, FakeControl]) -> None:
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
    assert control.route_calls == 0
    assert control.ensure_calls == [("sbx_test", 3000)]


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
            "lease_expires_at": int(time.time()) + 300,
            "next_activity_after": int(time.time()) + 30,
        },
    )

    resp = await test_client.get("/v1/lookup", headers={"Host": "3000-sbx_test.preview.test"})

    assert resp.status == 204
    assert resp.headers["X-Microsandbox-Upstream"] == "10.0.0.2:43000"
    assert control.route_calls == 0
    assert control.ensure_calls == [("sbx_test", 3000)]


async def test_runtime_port_is_not_special_when_lease_is_valid(client: tuple[TestClient, MemoryStore, FakeControl]) -> None:
    test_client, store, control = client
    await store.set_json(route_key("sbx_test"), control.route_payload)

    resp = await test_client.get("/v1/lookup", headers={"Host": "7080-sbx_test.preview.test"})

    assert resp.status == 204
    assert resp.headers["X-Microsandbox-Upstream"] == "10.0.0.2:47080"
    assert control.ensure_calls == []


async def test_runtime_paths_have_identical_lifecycle_behavior(client: tuple[TestClient, MemoryStore, FakeControl]) -> None:
    test_client, store, control = client
    await store.set_json(route_key("sbx_test"), control.route_payload)

    responses = await asyncio.gather(
        *(
            test_client.get(
                "/v1/lookup",
                headers={"Host": "7080-sbx_test.preview.test", "X-Forwarded-Uri": path},
            )
            for path in ["/healthz", "/config", "/readyz", "/sessions/sess_1/messages"]
        )
    )

    assert [resp.status for resp in responses] == [204, 204, 204, 204]
    assert control.ensure_calls == []


async def test_expired_lease_triggers_single_ensure_ready(client: tuple[TestClient, MemoryStore, FakeControl]) -> None:
    test_client, store, control = client
    control.ensure_delay = 0.05
    await store.set_json(
        route_key("sbx_test"),
        dict(control.route_payload, lease_expires_at=int(time.time()) - 1),
    )

    async def lookup() -> str:
        resp = await test_client.get("/v1/lookup", headers={"Host": "7080-sbx_test.preview.test"})
        assert resp.status == 204
        return resp.headers["X-Microsandbox-Upstream"]

    upstreams = await asyncio.gather(*(lookup() for _ in range(5)))

    assert upstreams == ["10.0.0.2:47080"] * 5
    assert control.ensure_calls == [("sbx_test", 7080)]


async def test_hot_leased_burst_uses_no_control_calls(client: tuple[TestClient, MemoryStore, FakeControl]) -> None:
    test_client, store, control = client
    await store.set_json(route_key("sbx_test"), control.route_payload)

    async def lookup() -> int:
        resp = await test_client.get("/v1/lookup", headers={"Host": "3000-sbx_test.preview.test"})
        return resp.status

    statuses = await asyncio.gather(*(lookup() for _ in range(200)))

    assert statuses == [204] * 200
    assert control.route_calls == 0
    assert control.ensure_calls == []
    assert control.activity_bulk_calls == []


async def test_near_expiry_activity_refreshes_without_wake(client: tuple[TestClient, MemoryStore, FakeControl]) -> None:
    test_client, store, control = client
    await store.set_json(
        route_key("sbx_test"),
        dict(control.route_payload, next_activity_after=int(time.time()) - 1),
    )

    resp = await test_client.get("/v1/lookup", headers={"Host": "3000-sbx_test.preview.test"})
    await asyncio.sleep(0.01)

    assert resp.status == 204
    assert control.ensure_calls == []
    assert control.activity_bulk_calls == [["sbx_test"]]


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
    assert control.ensure_calls == [("sbx_test", 3000)]


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
