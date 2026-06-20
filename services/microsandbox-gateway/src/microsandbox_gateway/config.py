from __future__ import annotations

from dataclasses import dataclass
import os


def _duration_seconds(value: str | None, default: int) -> int:
    raw = (value or "").strip().lower()
    if not raw:
        return default
    try:
        if raw.endswith("ms"):
            return max(1, int(float(raw[:-2]) / 1000))
        if raw.endswith("s"):
            return max(1, int(float(raw[:-1])))
        if raw.endswith("m"):
            return max(1, int(float(raw[:-1]) * 60))
        return max(1, int(float(raw)))
    except ValueError:
        return default


@dataclass(frozen=True)
class Config:
    addr: str
    port: int
    redis_url: str
    admin_token: str
    base_domain: str
    control_url: str
    control_token: str
    wake_timeout_seconds: int
    activity_debounce_seconds: int
    route_stale_seconds: int
    runtime_port: int

    @classmethod
    def from_env(cls) -> "Config":
        control_token = (
            os.environ.get("HIVY_MICROSANDBOX_CONTROL_API_TOKEN", "")
            or os.environ.get("HIVY_MICROSANDBOX_PREVIEW_WAKE_TOKEN", "")
            or os.environ.get("HIVY_MICROSANDBOX_RUNNER_API_TOKEN", "")
        )
        return cls(
            addr=os.environ.get("HIVY_MICROSANDBOX_PREVIEW_CACHE_ADDR", "127.0.0.1"),
            port=int(os.environ.get("HIVY_MICROSANDBOX_PREVIEW_CACHE_PORT", "8091")),
            redis_url=os.environ.get(
                "HIVY_MICROSANDBOX_PREVIEW_CACHE_REDIS_URL",
                "redis://127.0.0.1:6379/0",
            ),
            admin_token=os.environ.get("HIVY_MICROSANDBOX_PREVIEW_CACHE_TOKEN", ""),
            base_domain=os.environ.get(
                "HIVY_MICROSANDBOX_PREVIEW_BASE_DOMAIN",
                "preview.usehivy.com",
            ).strip("."),
            control_url=os.environ.get("HIVY_MICROSANDBOX_CONTROL_URL", "").rstrip("/"),
            control_token=control_token,
            wake_timeout_seconds=_duration_seconds(
                os.environ.get("HIVY_MICROSANDBOX_PREVIEW_WAKE_TIMEOUT"),
                90,
            ),
            activity_debounce_seconds=_duration_seconds(
                os.environ.get("HIVY_MICROSANDBOX_ACTIVITY_DEBOUNCE"),
                5,
            ),
            route_stale_seconds=_duration_seconds(
                os.environ.get("HIVY_MICROSANDBOX_ROUTE_STALE_AFTER"),
                30,
            ),
            runtime_port=int(os.environ.get("HIVY_MICROSANDBOX_RUNTIME_PORT", "7080")),
        )
