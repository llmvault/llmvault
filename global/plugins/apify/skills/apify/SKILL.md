---
name: apify
description: Use for a web-data extraction task that needs an Apify Actor: discover an Actor, inspect its current schema and pricing, run it after approval, and retrieve a bounded result set through Hivy's Apify proxy.
---

# Apify through Hivy

Use Apify only through the sandbox proxy. It supplies the customer's credential without exposing it.

```bash
test -n "$HIVY_APIFY_URL" && test -n "$HIVY_APIFY_TOKEN"
APIFY_API="${HIVY_APIFY_URL%/}/v2"
AUTH=(-H "Authorization: Bearer $HIVY_APIFY_TOKEN")
```

Never call `api.apify.com` directly, print the token, or attempt a delete. The proxy rejects destructive requests. Read-only discovery and result retrieval are safe; an Actor or Task run can consume money.

## Required workflow

1. Search the Store for the user's task. Prefer an Apify-maintained Actor when a current one fits. Workflow-specific guidance lives in `references/workflows/`; load the matching guide when it has needed actor or input detail.
2. Inspect the selected Actor before use: confirm it is not deprecated, inspect current pricing, and read its input schema. Never guess a limit or input-field name.
3. Before a paid run, state the Actor, input, expected result volume, and rough cost. Wait for explicit approval. If unsure whether a run is paid, ask.
4. Run a small quick job synchronously; otherwise start a run and poll its terminal status. On success, fetch a small, selected result set. On failure, inspect `statusMessage` and the Actor schema before changing input.
5. Keep large data out of context. Select fields, apply a limit, save CSV/JSON locally when needed, and report the run and dataset links.

## Recommended Actor starting points

Use this short list to start discovery, then still inspect the live Store result, pricing, deprecation state, and input schema. Actor availability and fields change. Prefer a maintained `apify` Actor when it meets the task; otherwise choose the currently healthy Store result with the best schema and price. Convert `owner/name` to `owner~name` in API paths.

| Task | Recommended starting Actor |
|---|---|
| Instagram profiles, posts, reels, comments | `apify/instagram-scraper` |
| TikTok profiles, videos, comments | `clockworks/tiktok-scraper` |
| YouTube videos, channels, comments | `streamers/youtube-scraper` |
| LinkedIn people or company research | `harvestapi/linkedin-profile-search` |
| Google Maps businesses, reviews, contacts | `compass/crawler-google-places` |
| Google Search / SERP research | `apify/google-search-scraper` |
| Google Trends | `apify/google-trends-scraper` |
| Public web-page content for research or RAG | `apify/website-content-crawler` |
| JavaScript-heavy crawling | `apify/playwright-scraper` |
| General structured website extraction | `apify/web-scraper` |
| Product and e-commerce extraction | `apify/e-commerce-scraping-tool` |
| Airbnb listings or reviews | `tri_angle/airbnb-scraper` |

## Core requests

Search:

```bash
curl -fsS -G "$APIFY_API/store" "${AUTH[@]}" \
  --data-urlencode "search=$QUERY" --data-urlencode "limit=10" \
  | jq '[.data.items[] | {actor:"\(.username)/\(.name)", title, pricing:.currentPricingInfo}]'
```

Inspect current metadata and input schema. Actor path IDs use `owner~name`, not `owner/name`.

```bash
curl -fsS "$APIFY_API/acts/$ACTOR" "${AUTH[@]}" \
  | jq '.data | {id, title, deprecated:.isDeprecated, pricing:(.pricingInfos[-1] // null)}'
curl -fsS "$APIFY_API/acts/$ACTOR/builds/default" "${AUTH[@]}" \
  | jq '.data.inputSchema | fromjson | {required, properties}'
```

For an approved quick run, pass one JSON object as input:

```bash
curl -fsS -X POST "$APIFY_API/acts/$ACTOR/run-sync-get-dataset-items" \
  "${AUTH[@]}" -H 'Content-Type: application/json' --data @input.json \
  | jq '.[0:25]'
```

For a longer job, start `POST /acts/$ACTOR/runs`, retain `.data.id` and `.data.defaultDatasetId`, then request `GET /actor-runs/$RUN_ID?waitForFinish=60` until it is terminal. Fetch only needed dataset fields:

```bash
curl -fsS -G "$APIFY_API/datasets/$DATASET_ID/items" "${AUTH[@]}" \
  --data-urlencode 'clean=true' --data-urlencode 'limit=100' \
  --data-urlencode 'fields=name,url,email' | jq '.'
```

`SUCCEEDED` permits retrieval. For `FAILED`, report the status message and use the run log at `https://console.apify.com/actors/runs/$RUN_ID/log`; do not claim a result exists. A Task follows the same approval and result rules through `/actor-tasks/$TASK_ID/...`.

## Data and handoff

Use `jq` to project needed fields and write larger data to a sandbox file. Hand that file to Sheets or another requested destination rather than pasting raw datasets into chat. Report the actor, confirmed scope/cost, result count, run link, and dataset link:

```text
https://console.apify.com/actors/runs/RUN_ID
https://console.apify.com/storage/datasets/DATASET_ID
```
