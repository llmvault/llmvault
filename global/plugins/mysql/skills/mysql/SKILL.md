---
name: mysql
description: Query the connected MySQL database through Hivy's read-only database proxy.
---

# Use MySQL

You are running inside the Hivy runtime. All MySQL access must go through the Hivy proxy for security, credential isolation, policy enforcement, and tracking.

## Environment

| Variable | Purpose |
|---|---|
| `HIVY_MYSQL_URL` | Hivy-provided MySQL query proxy URL |
| `HIVY_MYSQL_TOKEN` | Bearer token for the proxy |

## Query

Send raw SQL to the proxy exactly as you would send SQL to MySQL.

```bash
test -n "$HIVY_MYSQL_URL" || { echo "HIVY_MYSQL_URL is not set" >&2; exit 1; }
test -n "$HIVY_MYSQL_TOKEN" || { echo "HIVY_MYSQL_TOKEN is not set" >&2; exit 1; }

curl -fsS "$HIVY_MYSQL_URL" \
  -H "Authorization: Bearer $HIVY_MYSQL_TOKEN" \
  -H "Content-Type: text/plain" \
  --data-binary 'SELECT id, created_at FROM users LIMIT 20;' | jq
```

## jq filtering

The proxy returns JSON with `columns`, `rows`, `row_count`, and `truncated`. Always pipe through `jq` and return only the data needed for the answer. Avoid dumping full row arrays unless the user explicitly needs raw rows.

```bash
run_mysql() {
  curl -fsS "$HIVY_MYSQL_URL" \
    -H "Authorization: Bearer $HIVY_MYSQL_TOKEN" \
    -H "Content-Type: text/plain" \
    --data-binary "$1"
}
```

Examples:

```bash
# 1. Return only a compact list of selected fields.
run_mysql 'SELECT id, email, created_at FROM users ORDER BY created_at DESC LIMIT 10;' \
  | jq '{count: .row_count, users: [.rows[] | {id, email, created_at}]}'

# 2. Aggregate in SQL, then shape the answer with jq.
run_mysql '
  SELECT DATE(created_at) AS day, COUNT(*) AS signups
  FROM users
  WHERE created_at >= NOW() - INTERVAL 14 DAY
  GROUP BY DATE(created_at)
  ORDER BY day DESC;
' | jq '{metric:"daily_signups", days:.rows}'

# 3. Complex grouped aggregate with percentages.
run_mysql '
  SELECT status, COUNT(*) AS total
  FROM orders
  GROUP BY status
  ORDER BY total DESC;
' | jq '
  (.rows | map(.total | tonumber) | add) as $sum
  | {total_orders:$sum, by_status:[.rows[] | {status, total:(.total|tonumber), pct:(((.total|tonumber) / $sum * 100) | round)}]}
'

# 4. Check whether results were capped before relying on them.
run_mysql 'SELECT id, amount, created_at FROM invoices ORDER BY created_at DESC LIMIT 100;' \
  | jq '{truncated, sample:[.rows[:10][] | {id, amount, created_at}]}'
```

## Rules

- Use only narrow, read-only SQL.
- Prefer explicit columns and small limits.
- Push filtering, grouping, sorting, and aggregation into SQL first; use `jq` to shape the final JSON and remove noise.
- Do not attempt writes, deletes, schema changes, privilege changes, credential extraction, or admin commands.
- If a query is blocked, explain that the requested data or action is outside the configured Hivy database access policy.
- Do not print `$HIVY_MYSQL_TOKEN`.
- Do not call the database directly.
