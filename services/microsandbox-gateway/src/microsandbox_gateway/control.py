from __future__ import annotations

from typing import Any

from aiohttp import ClientSession, ClientTimeout


class ControlNotFoundError(RuntimeError):
    pass


class ControlClient:
    def __init__(self, base_url: str, token: str, session: ClientSession | None = None) -> None:
        self.base_url = base_url.rstrip("/")
        self.token = token
        self.session = session
        self._owned_session: ClientSession | None = None

    async def __aenter__(self) -> "ControlClient":
        if self.session is None:
            self._owned_session = ClientSession(timeout=ClientTimeout(total=120))
            self.session = self._owned_session
        return self

    async def __aexit__(self, *args: object) -> None:
        await self.close()

    async def close(self) -> None:
        if self._owned_session is not None:
            await self._owned_session.close()
            self._owned_session = None
            self.session = None

    def configured(self) -> bool:
        return bool(self.base_url and self.token)

    async def route(self, sandbox_id: str) -> dict[str, Any]:
        payload = await self._request("GET", f"/v1/sandboxes/{sandbox_id}/route")
        return payload["route"]

    async def alias(self, alias: str) -> dict[str, Any] | None:
        """Resolve an alias to its (sandbox_id, port) mapping via the control
        plane. Returns None when control has no such alias (404). This is the
        gateway's self-heal on a Redis miss: control is the source of truth for
        alias→sandbox bindings, so a dropped push (or evicted key) no longer
        strands the alias behind "invalid preview host"."""
        if not self.configured():
            return None
        if self.session is None:
            self._owned_session = ClientSession(timeout=ClientTimeout(total=120))
            self.session = self._owned_session
        headers = {"Authorization": f"Bearer {self.token}"}
        async with self.session.request(
            "GET",
            f"{self.base_url}/v1/aliases/{alias}",
            headers=headers,
            timeout=ClientTimeout(total=5),
        ) as response:
            if response.status == 404:
                return None
            if response.status >= 300:
                body = await response.text()
                raise RuntimeError(f"control returned {response.status}: {body.strip()}")
            return await response.json()

    async def ensure_ready(
        self,
        sandbox_id: str,
        guest_port: int,
        timeout_seconds: int,
        request_id: str,
    ) -> dict[str, Any]:
        payload = await self._request(
            "POST",
            f"/v1/sandboxes/{sandbox_id}/ensure-ready",
            {
                "guest_port": guest_port,
                "timeout_seconds": timeout_seconds,
                "request_id": request_id,
            },
            timeout_seconds + 5,
        )
        return payload["route"]

    async def activity(self, sandbox_id: str, source: str = "gateway") -> None:
        await self.activity_bulk([sandbox_id], source)

    async def activity_bulk(self, sandbox_ids: list[str], source: str = "gateway") -> list[dict[str, Any]]:
        payload = await self._request(
            "POST",
            "/v1/sandboxes/activity/bulk",
            {"source": source, "sandbox_ids": sandbox_ids, "runtime_busy": False},
            timeout=5,
        )
        return list(payload.get("routes") or [])

    async def _request(
        self,
        method: str,
        path: str,
        json_body: dict[str, Any] | None = None,
        timeout: int | None = None,
    ) -> dict[str, Any]:
        if not self.configured():
            raise RuntimeError("microsandbox control is not configured")
        if self.session is None:
            self._owned_session = ClientSession(timeout=ClientTimeout(total=120))
            self.session = self._owned_session
        headers = {"Authorization": f"Bearer {self.token}"}
        request_timeout = ClientTimeout(total=timeout) if timeout else None
        async with self.session.request(
            method,
            self.base_url + path,
            json=json_body,
            headers=headers,
            timeout=request_timeout,
        ) as response:
            if response.status == 404:
                raise ControlNotFoundError("control resource not found")
            if response.status >= 300:
                body = await response.text()
                raise RuntimeError(f"control returned {response.status}: {body.strip()}")
            if response.content_length == 0:
                return {}
            return await response.json()
