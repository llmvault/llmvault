---
name: redis
description: Use when inspecting, sampling, querying, or troubleshooting data in a connected Redis database.
---

# Use Redis

Do not connect to Redis directly. Use the provided proxy endpoint and environment variables.

## Environment

| Variable | Purpose |
|---|---|
| `HIVY_REDIS_PROXY_URL` | Provided Redis command proxy URL |
| `HIVY_REDIS_PROXY_TOKEN` | Bearer token for the proxy |

## Query

Send read-only Redis commands to the proxy as JSON.

```bash
test -n "$HIVY_REDIS_PROXY_URL" || { echo "HIVY_REDIS_PROXY_URL is not set" >&2; exit 1; }
test -n "$HIVY_REDIS_PROXY_TOKEN" || { echo "HIVY_REDIS_PROXY_TOKEN is not set" >&2; exit 1; }

curl -fsS "$HIVY_REDIS_PROXY_URL" \
  -H "Authorization: Bearer $HIVY_REDIS_PROXY_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"command":"GET","args":["user:123"]}' | jq
```

The proxy accepts either:

```json
{"command":"GET","args":["user:123"]}
```

or:

```json
["GET","user:123"]
```

## jq filtering

The proxy returns JSON with `command`, `result`, `row_count`, and `truncated`. Always pipe through `jq` and return only the data needed for the answer. Avoid dumping large arrays unless the user explicitly needs raw values.

```bash
run_redis() {
  curl -fsS "$HIVY_REDIS_PROXY_URL" \
    -H "Authorization: Bearer $HIVY_REDIS_PROXY_TOKEN" \
    -H "Content-Type: application/json" \
    -d "$1"
}
```

Examples:

```bash
# 1. Read a single key.
run_redis '{"command":"GET","args":["user:123"]}' \
  | jq '{key:"user:123", value:.result}'

# 2. Check type and TTL before reading a key.
run_redis '{"command":"TYPE","args":["session:abc"]}' | jq '{type:.result}'
run_redis '{"command":"TTL","args":["session:abc"]}' | jq '{ttl_seconds:.result}'

# 3. Sample keys with a bounded scan.
run_redis '{"command":"SCAN","args":["0","MATCH","user:*","COUNT","20"]}' \
  | jq '{scan:.result, truncated}'

# 4. Read a bounded list range.
run_redis '{"command":"LRANGE","args":["events","0","19"]}' \
  | jq '{events:.result}'
```

## Rules

- Use only read-only commands such as `GET`, `MGET`, `TYPE`, `TTL`, `SCAN`, `HGET`, `HMGET`, `HSCAN`, `LRANGE`, `SSCAN`, `ZRANGE`, and `ZSCAN`.
- Prefer exact key reads and bounded ranges.
- For list or sorted-set ranges, use small non-negative start and stop offsets.
- For scans, include `MATCH` and `COUNT`.
- Do not attempt writes, deletes, publishes, subscriptions, scripts, config changes, admin commands, credential extraction, or blocking operations.
- If a command is blocked, explain that the requested data or action is outside the configured Redis access policy.
- Do not print `$HIVY_REDIS_PROXY_TOKEN`.
- Do not call Redis directly.
