---
name: mongodb
description: Query the connected MongoDB database through Hivy's read-only database proxy.
---

# Use MongoDB

You are running inside the Hivy runtime. All MongoDB access must go through the Hivy proxy for security, credential isolation, policy enforcement, and tracking.

## Environment

| Variable | Purpose |
|---|---|
| `HIVY_MONGODB_URL` | Hivy-provided MongoDB command proxy URL |
| `HIVY_MONGODB_TOKEN` | Bearer token for the proxy |

## Query

Send native MongoDB command JSON to the proxy.

```bash
test -n "$HIVY_MONGODB_URL" || { echo "HIVY_MONGODB_URL is not set" >&2; exit 1; }
test -n "$HIVY_MONGODB_TOKEN" || { echo "HIVY_MONGODB_TOKEN is not set" >&2; exit 1; }

curl -fsS "$HIVY_MONGODB_URL" \
  -H "Authorization: Bearer $HIVY_MONGODB_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"find":"users","filter":{"status":"active"},"limit":20}' | jq
```

## jq filtering

Always pipe through `jq` and return only the data needed for the answer. Avoid dumping complete documents unless the user explicitly needs raw documents.

```bash
run_mongo() {
  curl -fsS "$HIVY_MONGODB_URL" \
    -H "Authorization: Bearer $HIVY_MONGODB_TOKEN" \
    -H "Content-Type: application/json" \
    -d "$1"
}
```

Examples:

```bash
# 1. Find recent users and keep only stable fields.
run_mongo '{"find":"users","filter":{"status":"active"},"projection":{"email":1,"created_at":1},"sort":{"created_at":-1},"limit":10}' \
  | jq '{users:[.cursor.firstBatch[] | {email, created_at}]}'

# 2. Count/group with an aggregation pipeline.
run_mongo '{
  "aggregate":"orders",
  "pipeline":[
    {"$group":{"_id":"$status","total":{"$sum":1}}},
    {"$sort":{"total":-1}}
  ],
  "cursor":{}
}' | jq '{by_status:[.cursor.firstBatch[] | {status:._id, total}]}'

# 3. Complex aggregate with date buckets and compact output.
run_mongo '{
  "aggregate":"users",
  "pipeline":[
    {"$group":{"_id":{"$dateToString":{"format":"%Y-%m-%d","date":"$created_at"}},"signups":{"$sum":1}}},
    {"$sort":{"_id":-1}},
    {"$limit":14}
  ],
  "cursor":{}
}' | jq '{metric:"daily_signups", days:[.cursor.firstBatch[] | {day:._id, signups}]}'

# 4. Summarize large documents instead of printing them.
run_mongo '{"find":"invoices","sort":{"created_at":-1},"limit":50}' \
  | jq '{sample:[.cursor.firstBatch[:10][] | {id:._id, amount, status, created_at}], returned:(.cursor.firstBatch|length)}'
```

## Rules

- Use only read-only commands such as `find`, `aggregate`, `count`, and `distinct`.
- Prefer narrow filters, small limits, and specific fields.
- Push filtering, grouping, sorting, projection, and aggregation into MongoDB first; use `jq` to shape the final JSON and remove noise.
- Do not attempt writes, deletes, schema changes, privilege changes, credential extraction, or admin commands.
- If a command is blocked, explain that the requested data or action is outside the configured Hivy database access policy.
- Do not print `$HIVY_MONGODB_TOKEN`.
- Do not call MongoDB directly.
