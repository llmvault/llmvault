---
name: mysql
description: Use when inspecting, querying, joining, aggregating, counting, or troubleshooting data in a connected MySQL database.
---

# Use MySQL

Do not connect to MySQL directly. Use the provided proxy endpoint and environment variables.

## Environment

| Variable | Purpose |
|---|---|
| `HIVY_MYSQL_URL` | Provided MySQL query proxy URL |
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

## Schema introspection

Inspect schema and table structure with standard `information_schema` queries. This works even on connections that have an access policy configured.

```bash
run_mysql 'SELECT table_name, column_name, data_type
           FROM information_schema.columns
           WHERE table_schema = DATABASE()
           ORDER BY table_name, ordinal_position;' | jq
```

Results reflect the access policy: on a restricted connection, introspection only returns the tables, columns, and constraints the policy allows, and denied objects do not appear. Use `information_schema` for structure discovery; `SHOW`/`DESCRIBE` are not available on policy-restricted connections.

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
- If a query is blocked, explain that the requested data or action is outside the configured database access policy.
- Do not print `$HIVY_MYSQL_TOKEN`.
- Do not call the database directly.
