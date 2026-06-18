---
name: postgres
description: Use when inspecting, querying, joining, aggregating, counting, or troubleshooting data in a connected PostgreSQL database.
---

# Use PostgreSQL

Do not connect to PostgreSQL directly. Use the provided proxy endpoint and environment variables.

## Environment

| Variable | Purpose |
|---|---|
| `HIVY_POSTGRES_URL` | Provided PostgreSQL query proxy URL |
| `HIVY_POSTGRES_TOKEN` | Bearer token for the proxy |

## Query

Send raw SQL to the proxy exactly as you would send SQL to PostgreSQL.

```bash
test -n "$HIVY_POSTGRES_URL" || { echo "HIVY_POSTGRES_URL is not set" >&2; exit 1; }
test -n "$HIVY_POSTGRES_TOKEN" || { echo "HIVY_POSTGRES_TOKEN is not set" >&2; exit 1; }

curl -fsS "$HIVY_POSTGRES_URL" \
  -H "Authorization: Bearer $HIVY_POSTGRES_TOKEN" \
  -H "Content-Type: text/plain" \
  --data-binary 'SELECT id, created_at FROM users LIMIT 20;' | jq
```

## jq filtering

The proxy returns JSON with `columns`, `rows`, `row_count`, and `truncated`. Always pipe through `jq` and return only the data needed for the answer. Avoid dumping full row arrays unless the user explicitly needs raw rows.

```bash
run_pg() {
  curl -fsS "$HIVY_POSTGRES_URL" \
    -H "Authorization: Bearer $HIVY_POSTGRES_TOKEN" \
    -H "Content-Type: text/plain" \
    --data-binary "$1"
}
```

Examples:

```bash
# 1. Return only a compact list of selected fields.
run_pg 'SELECT id, email, created_at FROM users ORDER BY created_at DESC LIMIT 10;' \
  | jq '{count: .row_count, users: [.rows[] | {id, email, created_at}]}'

# 2. Aggregate in SQL, then shape the answer with jq.
run_pg '
  SELECT date_trunc('"'day'"', created_at)::date AS day, count(*) AS signups
  FROM users
  WHERE created_at >= now() - interval '"'14 days'"'
  GROUP BY day
  ORDER BY day DESC;
' | jq '{metric:"daily_signups", days:.rows}'

# 3. Complex grouped aggregate with percentages.
run_pg '
  SELECT status, count(*) AS total
  FROM orders
  GROUP BY status
  ORDER BY total DESC;
' | jq '
  (.rows | map(.total | tonumber) | add) as $sum
  | {total_orders:$sum, by_status:[.rows[] | {status, total:(.total|tonumber), pct:(((.total|tonumber) / $sum * 100) | round)}]}
'

# 4. Check whether results were capped before relying on them.
run_pg 'SELECT id, amount, created_at FROM invoices ORDER BY created_at DESC LIMIT 100;' \
  | jq '{truncated, sample:[.rows[:10][] | {id, amount, created_at}]}'
```

## Rules

- Use only narrow, read-only SQL.
- Prefer explicit columns and small limits.
- Push filtering, grouping, sorting, and aggregation into SQL first; use `jq` to shape the final JSON and remove noise.
- Do not attempt writes, deletes, schema changes, privilege changes, credential extraction, or admin commands.
- If a query is blocked, explain that the requested data or action is outside the configured database access policy.
- Do not print `$HIVY_POSTGRES_TOKEN`.
- Do not call the database directly.
