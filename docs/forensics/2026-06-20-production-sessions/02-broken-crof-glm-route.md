# Broken Crof GLM Route

## Summary

Session `1964aad8-5cfa-4bb7-819c-6db15da7cdfc` failed after the runtime started successfully because the selected model route was invalid upstream.

The session model was `crof-glm-5.2`. The registry maps that canonical model to provider `crof` with upstream model ID `glm-5.2`. During the runtime turn, the model request failed three times with:

```text
HTTP 404: Model Not Known
```

The same general model family worked in the high-cost session when selected as plain `glm-5.2`, because that route maps to OpenRouter upstream `z-ai/glm-5.2`.

## Impact

- Sandbox provisioning succeeded.
- Runtime config and message delivery succeeded.
- Runtime emitted `turn_started`.
- The model call failed three consecutive times.
- Runtime emitted `error`, `final`, and `turn_failed`.
- Session ended with `agent_turn_last_outcome=failed`.

## Primary Evidence

- [Main DB snapshot](/tmp/hivy-prod-session-forensics-20260620/main_db_snapshot.json)
- [Latest web sessions snapshot](/tmp/hivy-prod-session-forensics-20260620/web_and_linked_sessions_snapshot.json)
- [API logs](/tmp/hivy-prod-session-forensics-20260620/api_logs_4h.jsonl)
- [Microsandbox DB snapshot](/tmp/hivy-prod-session-forensics-20260620/msb_db_snapshot.json)

Relevant code paths:

- Model route definitions: `internal/registry/hivy_models_latest.go`
- Per-session model override token/config: `internal/agentruntime/session_model.go`
- Runtime model request: `sandboxes/runtime/crates/agent/src/model_client.rs`
- Proxy model rewrite/auth: `internal/proxy/model.go`, `internal/proxy/director.go`, `internal/proxy/auth.go`

## Timeline

All times are UTC.

| Time | Event |
| --- | --- |
| 2026-06-20 11:29:04.297 | Hivy sandbox row for Hakaree per-session sandbox was created. |
| 2026-06-20 11:29:04.345 | Microsandbox sandbox `0fa6dq5f` was created. |
| 2026-06-20 11:29:08.301 | Microsandbox ports were registered for `0fa6dq5f`. |
| 2026-06-20 11:29:08 | Runner logged Docker daemon bootstrap for `0fa6dq5f`, `duration_ms=2947`, `status=started`. |
| 2026-06-20 11:29:16.185 | Session `1964...` was created. |
| 2026-06-20 11:29:16.188 | User message persisted: "hey hakaree. can you give me a summary of this pr ? https://github.com/usehivy/hivy/pull/191". |
| 2026-06-20 11:29:19.154 | Queue row was marked delivered with runtime trace/turn IDs. |
| 2026-06-20 11:29:19.779 | Runtime emitted `turn_started`. |
| 2026-06-20 11:29:22.220 | Runtime emitted `error`: user-facing generic generation failure text. |
| 2026-06-20 11:29:22.401 | Runtime emitted `final`: same generic failure text. |
| 2026-06-20 11:29:22.584 | Runtime emitted `turn_failed`: `model request failed 3 times consecutively: model error: HTTP 404: Model Not Known (invalid_request)`. |

## Production State

Session:

- `id`: `1964aad8-5cfa-4bb7-819c-6db15da7cdfc`
- `name`: `summary-of-pr-191-hivy`
- `source`: `web`
- `agent`: Hakaree, `11601c67-0860-44f8-9730-fd9820b3970b`
- `sandbox_strategy`: `per_session`
- `model`: `crof-glm-5.2`
- `sandbox_id`: `81ce2946-9da4-41d0-8a91-e357eb485c30`
- `external_id`: `0fa6dq5f`
- `agent_turn_last_outcome`: `failed`

Queue:

- `status`: `delivered`
- `delivered_at`: `2026-06-20T11:29:19.154435Z`
- `runtime_trace_id`: present
- `runtime_turn_id`: present

Sandbox:

- Microsandbox ID: `0fa6dq5f`
- Runner: `runner-1.sandboxes.usehivy.com`
- Image: `ghcr.io/usehivy/hivy-sandboxes-runtime-developers:v3.7.0-amd64`
- Created: `2026-06-20T11:29:04.345809Z`
- Ports registered: `2026-06-20T11:29:08.301439Z`
- Stopped later: `2026-06-20T11:34:29.141934Z`

## Route Comparison

Broken route:

```go
{
    ID: "crof-glm-5.2",
    Routes: []ModelRoute{
        {ProviderID: "crof", ModelID: "glm-5.2"},
    },
}
```

Working route used by the high-cost session:

```go
{
    ID: "glm-5.2",
    Routes: []ModelRoute{
        {ProviderID: "openrouter", ModelID: "z-ai/glm-5.2"},
    },
}
```

The failure is therefore not "GLM 5.2 cannot be used at all." It is specifically the Crof provider route for `glm-5.2`.

## Root Cause

The platform allowed a session to select `crof-glm-5.2`, but the provider returned `404 Model Not Known` for Crof upstream model `glm-5.2`.

Possible explanations:

- Crof removed or renamed the model.
- The registry route was added ahead of provider availability.
- Crof's upstream model ID differs from `glm-5.2`.
- The route requires a different provider endpoint/account/entitlement.

The production evidence does not support a sandbox or runtime provisioning issue for this session. The runtime reached the model call and failed at the provider/proxy layer.

## Contributing Factors

- Model route availability is not validated before accepting a session model.
- There is no automatic route fallback from broken Crof `glm-5.2` to working OpenRouter `z-ai/glm-5.2`.
- User-facing final output is generic and hides the actionable model-route failure.
- Repeated model failures are only visible in runtime event payloads/logs, not clearly surfaced as "selected model unavailable."

## Recommended Fix Areas

1. Disable the route:
   - Remove `crof-glm-5.2` from available production models until a live Crof call succeeds.

2. Add model route health checks:
   - Periodically verify each production route with a minimal request.
   - Mark failed routes unhealthy and prevent selection.

3. Add request-time fallback:
   - If `crof-glm-5.2` fails with provider 404, fallback to a known compatible route when allowed, such as OpenRouter `z-ai/glm-5.2`.
   - Record the fallback explicitly for auditability.

4. Improve user-facing error:
   - Map provider 404 model errors to "Selected model is unavailable" with the canonical model ID.

5. Add deployment test:
   - Any newly added model route must pass a live provider check before being exposed in production catalogs.

## Acceptance Criteria For A Fix

- A user cannot start a production session with `crof-glm-5.2` while Crof returns `Model Not Known`.
- Broken provider model routes are detected before user traffic.
- Runtime `turn_failed` events carry a clear machine-readable model-route error code.
- The UI can show "model unavailable" instead of a generic generation failure.

## Suggested Owner Brief

Own model route correctness and route health for production model selection. Start with `internal/registry/hivy_models_latest.go`, the model catalog exposure path, and proxy route resolution. The immediate fix is to disable or repair `crof-glm-5.2`; the durable fix is live route validation plus fallback/error semantics.

