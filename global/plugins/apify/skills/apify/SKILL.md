---
name: apify
description: Universal web scraper for any platform. Scrape data from Instagram, Facebook, TikTok, YouTube, LinkedIn, X/Twitter, Google Maps, Google Search, Google Trends, Reddit, Airbnb, Yelp, and 15+ more platforms. Use for lead generation, brand monitoring, competitor analysis, influencer discovery, trend research, content analytics, audience analysis, review analysis, SEO intelligence, recruitment, or any data extraction task.
---

# Running Apify Actors and Fetching Results

## Overview

Apify runs cloud programs called **Actors** that scrape websites and platforms (Instagram, TikTok, YouTube, LinkedIn, Google Maps, Google Search, Reddit, Amazon, Airbnb, and many more) and write their output to **datasets** and **key-value stores**.

Access Apify through the provided proxy endpoint at `$HIVY_APIFY_URL`. Apify exposes a REST API where every path lives under `/v2`. Every API call must use:

```bash
Authorization: Bearer $HIVY_APIFY_TOKEN
```

Do not call `https://api.apify.com` directly, and never try to read or print a raw Apify token. Use only the provided proxy endpoint and environment variables. The proxy authenticates this sandbox and swaps in the real Apify credential before forwarding the request.

## When to Use

- Finding an Actor in the Apify Store for a scraping or extraction task
- Inspecting an Actor's input schema and pricing before running it
- Running an Actor (or a saved Task) with custom input
- Polling a run to completion and reading its status
- Fetching scraped items from a dataset (JSON or CSV)
- Reading a record from a key-value store (for example an Actor's `OUTPUT`)

Do not use this skill for writing or deploying your own Actor source code.

## Workflow Guides

Load a bundled workflow guide on demand when running a multi-step pipeline — they carry the domain detail (Actor IDs, input field names, pricing, and multi-step pipelines) that this file summarizes. Pick the guide that matches the task:

| Task involves... | Read |
|-----------------|------|
| leads, contacts, emails, B2B | `references/workflows/lead-generation.md` |
| competitor, ads, pricing, SERP position | `references/workflows/competitive-intel.md` |
| influencer, creator, vetting | `references/workflows/influencer-vetting.md` |
| brand, mentions, sentiment, social listening | `references/workflows/brand-monitoring.md` |
| reviews, ratings, reputation | `references/workflows/review-analysis.md` |
| SEO, SERP, crawl, content brief | `references/workflows/content-and-seo.md` |
| analytics, engagement, performance | `references/workflows/social-media-analytics.md` |
| trends, keywords, hashtags | `references/workflows/trend-research.md` |
| jobs, recruiting, candidates, hiring signals | `references/workflows/job-market-and-recruitment.md` |
| real estate, property listings, hotels | `references/workflows/real-estate-and-hospitality.md` |
| price monitoring, e-commerce, products | `references/workflows/ecommerce-price-monitoring.md` |
| contact enrichment, email extraction | `references/workflows/contact-enrichment.md` |
| knowledge base, RAG, LLM data feed | `references/workflows/knowledge-base-and-rag.md` |
| company research, due diligence, ABM | `references/workflows/company-research.md` |

Some workflow guides describe downstream processing (filtering, AI extraction, storage) as pipeline steps — treat those as the logic to apply after fetching data; the Apify portion is always performed through the proxy calls in this file.

## Required Environment

| Variable | Purpose |
| --- | --- |
| `HIVY_APIFY_URL` | Provided Apify proxy base URL |
| `HIVY_APIFY_TOKEN` | Bearer token for the provided proxy endpoint |

## Setup

```bash
test -n "$HIVY_APIFY_URL" || { echo "HIVY_APIFY_URL is not set" >&2; exit 1; }
test -n "$HIVY_APIFY_TOKEN" || { echo "HIVY_APIFY_TOKEN is not set" >&2; exit 1; }
HIVY_APIFY_URL="${HIVY_APIFY_URL%/}"
APIFY_API="$HIVY_APIFY_URL/v2"
```

Quick auth check (`whoami`):

```bash
curl -fsS "$APIFY_API/users/me" \
  -H "Authorization: Bearer $HIVY_APIFY_TOKEN" \
  | jq '.data | {id, username, email: (.email // null)}'
```

## Safety

- Never print `$HIVY_APIFY_TOKEN`, and never call `api.apify.com` directly. Always go through `$HIVY_APIFY_URL` with the bearer token.
- **Running an Actor or Task consumes the user's Apify compute credits and can cost real money.** CONFIRM WITH THE USER before starting any Actor run or Task run. State which Actor you will run, the input, and the expected result count.
- Before running a paid Actor, check its pricing (see "Inspect an Actor") and give a rough cost estimate. If the estimate is more than a few dollars, warn explicitly and get a clear go-ahead.
- Prefer read-only `GET` calls for discovery and result retrieval. Only issue `POST` run calls after the user confirms.
- Do not abort, delete, or mutate Actors, tasks, schedules, datasets, key-value stores, or runs unless the user explicitly asks.
- Datasets can be large. Start with a small `limit` (and `fields=` to select columns) and summarize instead of dumping full payloads unless the user asks for raw data.
- Always pipe responses through `jq` to extract only the fields you need — never let raw multi-megabyte JSON into your context.

## Response Shape

Most endpoints wrap their result in `{ "data": ... }`. List endpoints return `{ "data": { "items": [...], "total", "count", "offset", "limit" } }`. Two exceptions:

- `GET /datasets/{id}/items` returns a **bare JSON array** of items (no `data` wrapper).
- `GET /key-value-stores/{id}/records/{key}` returns the **raw record body** (no wrapper).

Actor IDs in a URL path use a tilde, not a slash: the store Actor `apify/instagram-scraper` becomes `apify~instagram-scraper`.

## Discover Actors

Search the public Apify Store (start here when the user names a platform or task):

```bash
QUERY="google maps"
curl -fsS -G "$APIFY_API/store" \
  -H "Authorization: Bearer $HIVY_APIFY_TOKEN" \
  --data-urlencode "search=$QUERY" \
  --data-urlencode "limit=10" \
  | jq '[.data.items[] | {
      actor: "\(.username)/\(.name)",
      title,
      users30d: .stats.totalUsers30Days,
      pricingModel: .currentPricingInfo.pricingModel
    }]'
```

Prefer Actors under the `apify` account (Apify-maintained) when several options match. See the **Actor Index** section below for a curated map of popular Actors by platform.

List the Actors in the connected account:

```bash
curl -fsS "$APIFY_API/acts?limit=50" \
  -H "Authorization: Bearer $HIVY_APIFY_TOKEN" \
  | jq '[.data.items[] | {id, name, username, "actor": "\(.username)/\(.name)"}]'
```

## Actor Index

Curated map of popular Actors by platform. Actor IDs are shown as `username/name`; convert to `username~name` for API URL paths (e.g. `apify~instagram-scraper`). Tiers: **apify** = Apify-maintained (always prefer), **community** = community-maintained (use to fill gaps). To read an Actor's input schema, see "Inspect an Actor" below.

### Instagram

| Actor | Tier | Best for |
|-------|------|----------|
| apify/instagram-scraper | apify | all Instagram data |
| apify/instagram-profile-scraper | apify | profiles, followers, bio |
| apify/instagram-post-scraper | apify | posts, engagement metrics |
| apify/instagram-comment-scraper | apify | post and reel comments |
| apify/instagram-hashtag-scraper | apify | posts by hashtag |
| apify/instagram-hashtag-analytics-scraper | apify | hashtag metrics, trends |
| apify/instagram-reel-scraper | apify | reels, transcripts, engagement |
| apify/instagram-api-scraper | apify | API-based, no login |
| apify/instagram-search-scraper | apify | search users, places |
| apify/instagram-tagged-scraper | apify | tagged/mentioned posts |
| apify/instagram-topic-scraper | apify | posts by topic |
| apify/instagram-followers-count-scraper | apify | follower count tracking |
| apify/export-instagram-comments-posts | apify | bulk posts + comments |

### Facebook

| Actor | Tier | Best for |
|-------|------|----------|
| apify/facebook-posts-scraper | apify | posts, videos, engagement |
| apify/facebook-comments-scraper | apify | comment extraction |
| apify/facebook-likes-scraper | apify | reactions, liker info |
| apify/facebook-groups-scraper | apify | public group content |
| apify/facebook-events-scraper | apify | events, attendees |
| apify/facebook-reels-scraper | apify | reels, engagement |
| apify/facebook-photos-scraper | apify | photos with OCR |
| apify/facebook-search-scraper | apify | page search |
| apify/facebook-marketplace-scraper | apify | marketplace listings |
| apify/facebook-followers-following-scraper | apify | follower lists |
| apify/facebook-video-search-scraper | apify | video search |
| apify/facebook-ads-scraper | apify | ad library, creatives |
| apify/facebook-page-contact-information | apify | page contact info |
| apify/facebook-reviews-scraper | apify | page reviews |
| apify/facebook-hashtag-scraper | apify | hashtag posts |
| apify/threads-profile-api-scraper | apify | Threads profiles |

### TikTok

| Actor | Tier | Best for |
|-------|------|----------|
| clockworks/tiktok-scraper | apify | all TikTok data |
| clockworks/tiktok-profile-scraper | apify | profiles, videos |
| clockworks/tiktok-video-scraper | apify | video details, metrics |
| clockworks/tiktok-comments-scraper | apify | video comments |
| clockworks/tiktok-hashtag-scraper | apify | videos by hashtag |
| clockworks/tiktok-followers-scraper | apify | follower profiles |
| clockworks/tiktok-user-search-scraper | apify | user search |
| clockworks/tiktok-sound-scraper | apify | videos by sound |
| clockworks/free-tiktok-scraper | apify | free tier extraction |
| clockworks/tiktok-ads-scraper | apify | hashtag analytics |
| clockworks/tiktok-trends-scraper | apify | trending content |
| clockworks/tiktok-explore-scraper | apify | explore categories |
| clockworks/tiktok-discover-scraper | apify | discover by hashtag |

### YouTube

| Actor | Tier | Best for |
|-------|------|----------|
| streamers/youtube-scraper | apify | videos, metrics |
| streamers/youtube-channel-scraper | apify | channel info |
| streamers/youtube-comments-scraper | apify | video comments |
| streamers/youtube-shorts-scraper | apify | shorts data |
| streamers/youtube-video-scraper-by-hashtag | apify | videos by hashtag |
| streamers/youtube-video-downloader | apify | video download |
| curious_coder/youtube-transcript-scraper | community | transcripts, captions |

### X/Twitter

| Actor | Tier | Best for |
|-------|------|----------|
| apidojo/tweet-scraper | community | tweet search |
| apidojo/twitter-scraper-lite | community | comprehensive, no limits |
| apidojo/twitter-user-scraper | community | user profiles |
| apidojo/twitter-profile-scraper | community | profiles + recent tweets |
| apidojo/twitter-list-scraper | community | tweets from lists |

### LinkedIn

| Actor | Tier | Best for |
|-------|------|----------|
| harvestapi/linkedin-profile-search | community | find profiles |
| harvestapi/linkedin-profile-scraper | community | profile with email |
| harvestapi/linkedin-company | community | company details |
| harvestapi/linkedin-company-employees | community | employee lists |
| harvestapi/linkedin-company-posts | community | company page posts |
| harvestapi/linkedin-profile-posts | community | profile posts |
| harvestapi/linkedin-job-search | community | job listings |
| harvestapi/linkedin-post-search | community | post search |
| harvestapi/linkedin-post-comments | community | post comments |
| harvestapi/linkedin-profile-search-by-name | community | find by name |
| harvestapi/linkedin-profile-search-by-services | community | find by service |
| apimaestro/linkedin-companies-search-scraper | community | company search |
| apimaestro/linkedin-company-detail | community | company deep data |
| apimaestro/linkedin-jobs-scraper-api | community | job search |
| apimaestro/linkedin-job-detail | community | job details |
| apimaestro/linkedin-batch-profile-posts-scraper | community | batch profile posts |
| apimaestro/linkedin-post-reshares | community | post reshares |
| apimaestro/linkedin-post-detail | community | post details |
| apimaestro/linkedin-profile-full-sections-scraper | community | full profile data |
| dev_fusion/linkedin-profile-scraper | community | mass scraping + email |

### Google Maps

| Actor | Tier | Best for |
|-------|------|----------|
| compass/crawler-google-places | apify | business listings |
| compass/google-maps-extractor | apify | detailed business data |
| compass/Google-Maps-Reviews-Scraper | apify | reviews, ratings |
| compass/enrich-google-maps-dataset-with-contacts | apify | email enrichment |
| compass/contact-details-scraper-standby | apify | quick contact extract |
| lukaskrivka/google-maps-with-contact-details | community | listings + contacts |
| curious_coder/google-maps-reviews-scraper | community | cheap review scraping |

### Google Search and Trends

| Actor | Tier | Best for |
|-------|------|----------|
| apify/google-search-scraper | apify | SERP, ads, AI overviews |
| apify/google-trends-scraper | apify | trend data |
| tri_angle/bing-search-scraper | apify | Bing SERP data |

### Reviews (cross-platform)

| Actor | Tier | Best for |
|-------|------|----------|
| tri_angle/hotel-review-aggregator | apify | 7-platform hotel reviews |
| tri_angle/restaurant-review-aggregator | apify | 6-platform restaurant reviews |
| tri_angle/yelp-scraper | apify | Yelp business data |
| tri_angle/yelp-review-scraper | apify | Yelp reviews |
| tri_angle/get-tripadvisor-urls | apify | find TripAdvisor URLs |
| tri_angle/get-yelp-urls | apify | find Yelp URLs |
| tri_angle/airbnb-reviews-scraper | apify | Airbnb reviews |
| tri_angle/social-media-sentiment-analysis-tool | apify | sentiment analysis |

### Real estate and hospitality

| Actor | Tier | Best for |
|-------|------|----------|
| tri_angle/airbnb-scraper | apify | Airbnb listings |
| tri_angle/new-fast-airbnb-scraper | apify | fast Airbnb search |
| tri_angle/airbnb-rooms-urls-scraper | apify | detailed room data |
| tri_angle/redfin-search | apify | Redfin property search |
| tri_angle/redfin-detail | apify | Redfin property details |
| tri_angle/real-estate-aggregator | apify | multi-source listings |
| tri_angle/fast-zoopla-properties-scraper | apify | UK properties |
| tri_angle/doordash-store-details-scraper | apify | DoorDash stores |
| tri_angle/cargurus-zipcode-search-scraper | apify | CarGurus listings |
| tri_angle/carmax-zipcode-search-scraper | apify | Carmax listings |

### SEO tools

| Actor | Tier | Best for |
|-------|------|----------|
| radeance/similarweb-scraper | community | traffic, rankings |
| radeance/ahrefs-scraper | community | backlinks, keywords |
| radeance/semrush-scraper | community | domain authority |
| radeance/moz-scraper | community | DA, spam score |
| radeance/ubersuggest-scraper | community | keyword suggestions |
| radeance/se-ranking-scraper | community | keyword CPC |

### Content and web crawling

| Actor | Tier | Best for |
|-------|------|----------|
| apify/website-content-crawler | apify | clean text for AI |
| apify/rag-web-browser | apify | RAG pipelines |
| apify/web-scraper | apify | general web scraping |
| apify/cheerio-scraper | apify | fast HTML parsing |
| apify/playwright-scraper | apify | JS-heavy sites |
| apify/camoufox-scraper | apify | anti-bot sites |
| apify/sitemap-extractor | apify | sitemap URLs |
| lukaskrivka/article-extractor-smart | community | article extraction |

### Other platforms

| Actor | Tier | Best for |
|-------|------|----------|
| tri_angle/telegram-scraper | apify | Telegram messages |
| tri_angle/snapchat-scraper | apify | Snapchat profiles |
| tri_angle/snapchat-spotlight-scraper | apify | Snapchat Spotlight |
| tri_angle/truth-scraper | apify | Truth Social |
| tri_angle/social-media-finder | apify | cross-platform search |
| tri_angle/website-changes-detector | apify | website monitoring |
| tri_angle/e-commerce-product-matching-tool | apify | product matching |
| trudax/reddit-scraper-lite | community | Reddit posts |
| janbuchar/github-contributors-scraper | community | GitHub contributors |

### Enrichment and contacts

| Actor | Tier | Best for |
|-------|------|----------|
| apify/social-media-leads-analyzer | apify | emails from websites |
| apify/social-media-hashtag-research | apify | cross-platform hashtags |
| apify/e-commerce-scraping-tool | apify | product data enrichment |
| vdrmota/contact-info-scraper | community | contact extraction |
| code_crafter/leads-finder | community | B2B leads |

## Inspect an Actor

Get an Actor's metadata and pricing before running it:

```bash
ACTOR="apify~instagram-scraper"
curl -fsS "$APIFY_API/acts/$ACTOR" \
  -H "Authorization: Bearer $HIVY_APIFY_TOKEN" \
  | jq '.data | {
      id, name, username,
      title: .title,
      deprecated: .isDeprecated,
      defaultBuild: .taggedBuilds.latest.buildId,
      pricing: (.pricingInfos[-1] // null) | {pricingModel, pricePerUnitUsd, trialMinutes}
    }'
```

Pricing models: `FREE` (only platform compute), `FLAT_PRICE_PER_MONTH` (subscription), `PRICE_PER_DATASET_ITEM` and `PAY_PER_EVENT` (charged per result — estimate cost before running). Present any estimate as rough guidance and tell the user to confirm actual charges in their Apify billing dashboard.

Fetch the Actor's **input schema** to learn the correct input field names:

```bash
ACTOR="apify~instagram-scraper"
curl -fsS "$APIFY_API/acts/$ACTOR/builds/default" \
  -H "Authorization: Bearer $HIVY_APIFY_TOKEN" \
  | jq '.data.inputSchema | fromjson | {title, required, properties: (.properties | to_entries | map({key, type: .value.type, description: .value.description}))}'
```

Different Actors use different limit fields (`maxItems`, `resultsLimit`, `maxResults`, `maxCrawledPages`). Always confirm the field name from the input schema rather than guessing.

## Run an Actor

CONFIRM WITH THE USER FIRST — this spends compute credits.

The request body is the Actor input JSON object (not an array). Write complex input to a file and pass it with `--data @file`.

### Option A — run and wait for the items (best for small, fast scrapes)

`run-sync-get-dataset-items` runs the Actor synchronously and returns the dataset items directly (a bare array). It blocks up to the run duration; use it only for quick jobs.

```bash
ACTOR="apify~instagram-scraper"
cat > /tmp/apify-input.json <<'JSON'
{ "usernames": ["nasa"], "resultsLimit": 5 }
JSON

curl -fsS -X POST "$APIFY_API/acts/$ACTOR/run-sync-get-dataset-items" \
  -H "Authorization: Bearer $HIVY_APIFY_TOKEN" \
  -H "Content-Type: application/json" \
  --data @/tmp/apify-input.json \
  | jq 'if type=="array" then [.[] | {username, followersCount, postsCount}] else . end'
```

### Option B — start the run, then poll (best for large or long-running scrapes)

Start the run (returns immediately with a run object):

```bash
ACTOR="apify~instagram-scraper"
RUN=$(curl -fsS -X POST "$APIFY_API/acts/$ACTOR/runs" \
  -H "Authorization: Bearer $HIVY_APIFY_TOKEN" \
  -H "Content-Type: application/json" \
  --data @/tmp/apify-input.json)
RUN_ID=$(echo "$RUN" | jq -r '.data.id')
DATASET_ID=$(echo "$RUN" | jq -r '.data.defaultDatasetId')
echo "Run: $RUN_ID  Dataset: $DATASET_ID"
```

Optional query params on `POST .../runs`: `?build=`, `?memory=` (MB), `?timeout=` (seconds), `?maxItems=`, and `?waitForFinish=60` (server holds the response up to 60s while the run finishes).

## Poll a Run's Status

Run statuses: `READY`, `RUNNING`, `SUCCEEDED`, `FAILED`, `ABORTED`, `TIMING-OUT`, `TIMED-OUT`. Poll every 5–10 seconds — do not spin in a tight loop. `waitForFinish` (max 60) lets the server block until the run ends or the timeout elapses.

```bash
RUN_ID="RUN_ID_HERE"
curl -fsS "$APIFY_API/actor-runs/$RUN_ID?waitForFinish=60" \
  -H "Authorization: Bearer $HIVY_APIFY_TOKEN" \
  | jq '.data | {
      status,
      statusMessage,
      datasetId: .defaultDatasetId,
      keyValueStoreId: .defaultKeyValueStoreId,
      itemCount: .stats.datasetItemCount,
      durationMillis: .stats.durationMillis
    }'
```

If `status` is `FAILED`, read `statusMessage` and point the user to the run log at `https://console.apify.com/actors/runs/RUN_ID/log`.

## Fetch Dataset Items

Once a run has `SUCCEEDED`, read its dataset. `GET /datasets/{id}/items` returns a bare JSON array. Use `limit`, `offset`, `fields`, and `clean=true` to keep responses small.

```bash
DATASET_ID="DATASET_ID_HERE"
curl -fsS -G "$APIFY_API/datasets/$DATASET_ID/items" \
  -H "Authorization: Bearer $HIVY_APIFY_TOKEN" \
  --data-urlencode "clean=true" \
  --data-urlencode "limit=25" \
  --data-urlencode "fields=username,followersCount,postsCount" \
  | jq '.'
```

CSV export (for saving to a file rather than reading into context):

```bash
DATASET_ID="DATASET_ID_HERE"
curl -fsS -G "$APIFY_API/datasets/$DATASET_ID/items" \
  -H "Authorization: Bearer $HIVY_APIFY_TOKEN" \
  --data-urlencode "format=csv" \
  --data-urlencode "limit=1000" \
  > "$(date +%F)_apify-export.csv"
```

Check how many items a dataset holds first:

```bash
DATASET_ID="DATASET_ID_HERE"
curl -fsS "$APIFY_API/datasets/$DATASET_ID" \
  -H "Authorization: Bearer $HIVY_APIFY_TOKEN" \
  | jq '.data | {id, name, itemCount}'
```

## Read a Key-Value Store Record

Actors often write structured results (or a summary) to the `OUTPUT` key of their default key-value store. List keys, then fetch a record. The record GET returns the raw body.

```bash
STORE_ID="STORE_ID_HERE"
# List keys
curl -fsS "$APIFY_API/key-value-stores/$STORE_ID/keys" \
  -H "Authorization: Bearer $HIVY_APIFY_TOKEN" \
  | jq '.data.items[] | {key, size}'

# Fetch a record (raw body — pipe to jq only if it is JSON)
curl -fsS "$APIFY_API/key-value-stores/$STORE_ID/records/OUTPUT" \
  -H "Authorization: Bearer $HIVY_APIFY_TOKEN"
```

## Tasks and Schedules

A **Task** is a saved Actor + input configuration. Run one the same way as an Actor.

```bash
# List saved tasks
curl -fsS "$APIFY_API/actor-tasks?limit=50" \
  -H "Authorization: Bearer $HIVY_APIFY_TOKEN" \
  | jq '[.data.items[] | {id, name, actId}]'

# Run a task (CONFIRM FIRST — spends credits). Optional JSON body overrides its saved input.
TASK_ID="TASK_ID_HERE"
curl -fsS -X POST "$APIFY_API/actor-tasks/$TASK_ID/run-sync-get-dataset-items" \
  -H "Authorization: Bearer $HIVY_APIFY_TOKEN" \
  -H "Content-Type: application/json" \
  | jq 'if type=="array" then length as $n | {items: .[0:10], total: $n} else . end'
```

List schedules (read-only):

```bash
curl -fsS "$APIFY_API/schedules?limit=50" \
  -H "Authorization: Bearer $HIVY_APIFY_TOKEN" \
  | jq '[.data.items[] | {id, name, cronExpression, isEnabled}]'
```

## Recent and Last Runs

```bash
# Most recent runs across the account
curl -fsS "$APIFY_API/actor-runs?limit=10&desc=true" \
  -H "Authorization: Bearer $HIVY_APIFY_TOKEN" \
  | jq '[.data.items[] | {id, actId, status, startedAt, datasetId: .defaultDatasetId}]'

# Last run of a specific Actor, and its dataset items in one call
ACTOR="apify~instagram-scraper"
curl -fsS "$APIFY_API/acts/$ACTOR/runs/last/dataset/items?limit=25" \
  -H "Authorization: Bearer $HIVY_APIFY_TOKEN" \
  | jq '.'
```

## Standard Workflow

1. Identify the target platform / task and search the Store (or the **Actor Index** section above) for a matching Actor. Prefer `apify`-maintained Actors.
2. Inspect the Actor: check `isDeprecated`, pricing, and the input schema for the correct field names.
3. Build the input JSON. For paid Actors, estimate cost and **confirm with the user before running**.
4. Run — Option A (`run-sync-get-dataset-items`) for quick jobs, Option B (`start` + poll) for large ones.
5. On `SUCCEEDED`, fetch dataset items (small `limit`, selected `fields`) and summarize, or save CSV to a file for large exports.
6. Report result count and links: dataset `https://console.apify.com/storage/datasets/DATASET_ID`, run `https://console.apify.com/actors/runs/RUN_ID`.

## Console Links

Use these human-facing links when reporting results (they point at the real Apify console, not the proxy):

```text
https://console.apify.com/actors/runs/RUN_ID
https://console.apify.com/actors/runs/RUN_ID/log
https://console.apify.com/storage/datasets/DATASET_ID
https://console.apify.com/storage/key-value-stores/STORE_ID
```

## Gotchas and Cost Guardrails

Read this before running any paid (PPE / per-item) Actor, or when debugging cost, deprecated Actors, empty datasets, input-shape errors, rate limits, or long runs.

### Pricing models

| Model | How it works | Action before running |
|-------|-------------|----------------------|
| FREE | No per-result cost, only platform compute | None needed |
| PAY_PER_EVENT (PPE) | Charged per result item | MUST estimate cost first |
| FLAT_PRICE_PER_MONTH | Monthly subscription | Verify user has active subscription |

To check an Actor's pricing:

```bash
curl -fsS "$APIFY_API/acts/ACTOR_ID" \
  -H "Authorization: Bearer $HIVY_APIFY_TOKEN" \
  | jq '.data.pricingInfos[-1]'
```

Read `.data.pricingInfos[-1].pricingModel` and `.data.pricingInfos[-1].pricePerUnitUsd`. When you found the Actor via the Store, each item also carries `.currentPricingInfo.pricingModel`.

### Cost estimation protocol

Before running any PPE Actor:

1. Get the per-event price from Actor info (`.currentPricingInfo.pricePerEvent`).
2. Multiply by the requested result count.
3. Present the estimate to the user with this disclaimer:

> **Estimated cost: ~$X for Y results.** This is a rough estimate only — actual costs can vary significantly depending on the Actor, data complexity, retries, and platform changes. Always check your Apify billing dashboard for actual charges.

4. If estimate > $5: warn explicitly.
5. If estimate > $20: require explicit user confirmation before proceeding.

**Important:** Cost estimates in the workflow guides are approximate and may be inaccurate. Always present them as rough guidance with the disclaimer above, never as exact amounts.

### Common pitfalls

**Cookie-dependent Actors** — Some social media scrapers require cookies or login sessions. If an Actor returns auth errors or empty results unexpectedly, check its README (bundled into the default build):

```bash
curl -fsS "$APIFY_API/acts/ACTOR_ID/builds/default" \
  -H "Authorization: Bearer $HIVY_APIFY_TOKEN" \
  | jq -r '.data.readme'
```

Look for mentions of "cookies", "login", "session", or "proxy".

**Input mechanics** — Actor input is one JSON object, not an array — it is the body of the run POST. Send inline JSON with `--data '{...}'`, or write it to a file and send `--data @input.json` (preferred for large or complex inputs). If a run reports parse, path, or object-shape input errors, inspect the schema again with `curl -fsS "$APIFY_API/acts/ACTOR_ID/builds/default" -H "Authorization: Bearer $HIVY_APIFY_TOKEN" | jq '.data.inputSchema | fromjson'`.

```bash
curl -fsS -X POST "$APIFY_API/acts/ACTOR_ID/runs" \
  -H "Authorization: Bearer $HIVY_APIFY_TOKEN" \
  -H "Content-Type: application/json" \
  --data '{"maxItems":10}'
```

Prefer a file for larger inputs:

```bash
curl -fsS -X POST "$APIFY_API/acts/ACTOR_ID/runs" \
  -H "Authorization: Bearer $HIVY_APIFY_TOKEN" \
  -H "Content-Type: application/json" \
  --data @input.json
```

**Rate limiting on large scrapes** — Platforms throttle or block large-volume scraping. Mitigations:
- Use proxy configuration when available: `"proxyConfiguration": {"useApifyProxy": true}`
- Set reasonable concurrency limits (check the Actor's `maxConcurrency` input).
- For 1,000+ results, suggest splitting into smaller batches.

**Empty results** — Common causes:
- Too-narrow search query or geo-restriction (try broader terms).
- Platform blocking without proxy (enable Apify Proxy).
- Actor requires cookies/login but none provided.
- Wrong input field name (always verify against the input schema).

**maxResults vs maxCrawledPages** — Different Actors use different limit field names. Common variants:
- `maxResults`, `resultsLimit`, `maxItems` — limit output items.
- `maxCrawledPages`, `maxRequestsPerCrawl` — limit pages visited.
Always fetch the input schema to find the correct field for the specific Actor.

**Deprecated Actors** — Check `.data.isDeprecated` in `curl -fsS "$APIFY_API/acts/ACTOR_ID" -H "Authorization: Bearer $HIVY_APIFY_TOKEN"`. If `true`:
1. Search for alternatives: `curl -fsS -G "$APIFY_API/store" -H "Authorization: Bearer $HIVY_APIFY_TOKEN" --data-urlencode "search=SIMILAR_KEYWORDS" --data-urlencode "limit=10"`
2. Prefer `apify` tier replacements over `community` alternatives.

**LinkedIn pricing** — LinkedIn Actors are all PPE and vary significantly:
- `harvestapi/` Actors: generally cheaper ($0.001-0.01/result).
- `apimaestro/` Actors: generally more expensive ($0.005-0.02/result).
- `dev_fusion/` Actors: mid-range, useful for mass scraping with email enrichment.
Always compare pricing before selecting a LinkedIn Actor.

**SEO tool pricing** — `radeance/` SEO scrapers (SimilarWeb, Ahrefs, SEMrush, Moz) have the highest per-result costs ($0.005-0.0275/result). For large-scale SEO analysis, estimate costs carefully and suggest batching.

### Error recovery

| Symptom | Likely cause | Fix |
|---------|-------------|-----|
| `status: FAILED` in run output | Actor crashed or input invalid | Read `.data.statusMessage` from `GET $APIFY_API/actor-runs/RUN_ID`; check run log at `https://console.apify.com/actors/runs/RUN_ID/log` |
| `isDeprecated: true` in Actor info | Actor is end-of-life | Search for replacement: `GET $APIFY_API/store?search=KEYWORDS&limit=10` |
| Empty dataset (0 items) | Query too narrow, geo-restriction, or anti-bot block | Broaden search terms; enable Apify Proxy; check Actor README with `GET $APIFY_API/acts/ACTOR_ID/builds/default` (`.data.readme`) |
| Run takes >10 minutes | Large scrape or slow target site | Switch to fire-and-forget: `POST $APIFY_API/acts/ACTOR_ID/runs`, poll with `GET $APIFY_API/actor-runs/RUN_ID` (check `.data.status` for `SUCCEEDED`) |

### Why Apify Actors vs raw HTTP scraping

Many automation workflows use raw HTTP Request nodes or self-hosted Puppeteer for web scraping. These hit common walls that Apify Actors handle transparently:

- **Cloudflare and WAF bypass** — Raw HTTP requests fail on sites with Cloudflare Turnstile, DataDome, or other WAFs. Apify Actors use residential proxies and browser fingerprint rotation automatically. For the toughest sites, use `apify/camoufox-scraper`.
- **JavaScript-rendered pages (SPAs)** — React, Vue, and Angular sites return empty HTML to plain HTTP requests. Apify's `apify/playwright-scraper` and `apify/camoufox-scraper` fully render JavaScript before extracting data.
- **Anti-bot fingerprinting** — Even headless browsers get detected via TLS fingerprints (JA3 hashes). Apify's browser pool rotates fingerprints across requests automatically.
- **Session and cookie management** — Social media platforms (LinkedIn, Instagram) require persistent sessions. Social media Actors handle cookie management and session rotation internally.
- **Scaling without infrastructure** — Self-hosted Puppeteer at scale requires 4-8 GB RAM per browser instance. Apify Actors run on serverless infrastructure — no browser pool management, no RAM provisioning, no Docker orchestration.

### Platform-specific rate limits

- **Instagram:** Aggressive rate limiting. Keep `maxResults` under 200 per run for profile/post scrapers. Use delays between runs. Instagram API scrapers (`apify/instagram-api-scraper`) have higher limits than browser-based ones.
- **LinkedIn:** All LinkedIn Actors are community-maintained and PPE. LinkedIn actively blocks scraping at scale. Keep batch sizes under 100 profiles. Space runs at least 5 minutes apart. Expect occasional empty results.
- **TikTok:** Anti-bot measures increasing. `clockworks/tiktok-scraper` handles most cases. For blocked regions, enable Apify Proxy with residential IPs.
- **Google Maps:** Generally stable. Set `language: "en"` explicitly for consistent results. Large-area searches may return different results depending on zoom level — use specific location queries over broad city names.
- **Amazon/E-commerce:** Heavy anti-bot. The `apify/e-commerce-scraping-tool` handles this via built-in proxy rotation. Raw HTTP requests will fail.
