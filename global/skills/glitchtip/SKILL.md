---
name: glitchtip
description: Use when investigating GlitchTip errors, issues, projects, events, stacktraces, releases, environments, monitors, logs, performance, or production exceptions. Provides verified curl and jq commands through the Hivy GlitchTip proxy using HIVY_GLITCHTIP_URL and HIVY_GLITCHTIP_TOKEN, and explains how to construct dashboard links using HIVY_GLITCHTIP_DASHBOARD_BASE_URL instead of the proxy URL.
---

# GlitchTip Error Monitoring

## Overview

Use GlitchTip through the Hivy-provided proxy at `$HIVY_GLITCHTIP_URL`.

GlitchTip exposes a Sentry-compatible REST API under `/api/0`. Every API call must use:

```bash
Authorization: Bearer $HIVY_GLITCHTIP_TOKEN
```

`HIVY_GLITCHTIP_URL` is a Hivy API proxy URL. Do not use it for links shown to users.

`HIVY_GLITCHTIP_DASHBOARD_BASE_URL` is the real GlitchTip dashboard base URL. Use it only for human-facing links. If it is unset, provide IDs and slugs instead of inventing links.

## Required Environment

| Variable | Purpose |
| --- | --- |
| `HIVY_GLITCHTIP_URL` | Hivy-provided GlitchTip proxy base URL |
| `HIVY_GLITCHTIP_TOKEN` | Bearer token for the provided proxy endpoint |

Optional:

| Variable | Purpose |
| --- | --- |
| `HIVY_GLITCHTIP_DASHBOARD_BASE_URL` | Real GlitchTip dashboard base URL for user-facing links |

## Setup

```bash
test -n "$HIVY_GLITCHTIP_URL" || { echo "HIVY_GLITCHTIP_URL is not set" >&2; exit 1; }
test -n "$HIVY_GLITCHTIP_TOKEN" || { echo "HIVY_GLITCHTIP_TOKEN is not set" >&2; exit 1; }
HIVY_GLITCHTIP_URL="${HIVY_GLITCHTIP_URL%/}"
GLITCHTIP_API="$HIVY_GLITCHTIP_URL/api/0"
HIVY_GLITCHTIP_DASHBOARD_BASE_URL="${HIVY_GLITCHTIP_DASHBOARD_BASE_URL%/}"
```

Quick auth check:

```bash
curl -fsS "$GLITCHTIP_API/" \
  -H "Authorization: Bearer $HIVY_GLITCHTIP_TOKEN" \
  | jq .
```

## Safety

- Do not print `$HIVY_GLITCHTIP_TOKEN`.
- Prefer read-only `GET` calls for investigation.
- Event and log payloads can be large and may include sensitive request context. Start with small `limit` values and summarize fields instead of dumping full payloads unless the user asks for raw data.
- Do not delete organizations, teams, projects, issues, releases, files, comments, monitors, alerts, or API tokens.
- Do not mutate issue state, alert rules, teams, members, subscriptions, billing, project keys, releases, or monitors unless the user explicitly asks.
- Never call GlitchTip directly with the raw provider token when the Hivy proxy variables are available.

## Dashboard Links

Construct links with `HIVY_GLITCHTIP_DASHBOARD_BASE_URL`, not `HIVY_GLITCHTIP_URL`.

Common dashboard URL patterns:

```text
$HIVY_GLITCHTIP_DASHBOARD_BASE_URL/{organization_slug}/issues/{issue_id}/
$HIVY_GLITCHTIP_DASHBOARD_BASE_URL/{organization_slug}/issues/{issue_id}/events/{event_id}/
$HIVY_GLITCHTIP_DASHBOARD_BASE_URL/{organization_slug}/projects/{project_slug}/
$HIVY_GLITCHTIP_DASHBOARD_BASE_URL/{organization_slug}/releases/{version}/
```

If a dashboard URL returns 404, keep the API-derived IDs/slugs in the response and say the dashboard route may differ for this GlitchTip deployment.

## Organizations

List organizations:

```bash
curl -fsS "$GLITCHTIP_API/organizations/" \
  -H "Authorization: Bearer $HIVY_GLITCHTIP_TOKEN" \
  | jq '[.[] | {id, slug, name}]'
```

Get organization detail:

```bash
ORG_SLUG="example-org"
curl -fsS "$GLITCHTIP_API/organizations/$ORG_SLUG/" \
  -H "Authorization: Bearer $HIVY_GLITCHTIP_TOKEN" \
  | jq .
```

## Projects

List all accessible projects:

```bash
curl -fsS "$GLITCHTIP_API/projects/" \
  -H "Authorization: Bearer $HIVY_GLITCHTIP_TOKEN" \
  | jq '[.[] | {id, slug, name, platform, organization: .organization.slug}]'
```

List projects in an organization:

```bash
ORG_SLUG="example-org"
curl -fsS "$GLITCHTIP_API/organizations/$ORG_SLUG/projects/" \
  -H "Authorization: Bearer $HIVY_GLITCHTIP_TOKEN" \
  | jq '[.[] | {id, slug, name, platform}]'
```

Get project detail:

```bash
ORG_SLUG="example-org"
PROJECT_SLUG="web"
curl -fsS "$GLITCHTIP_API/projects/$ORG_SLUG/$PROJECT_SLUG/" \
  -H "Authorization: Bearer $HIVY_GLITCHTIP_TOKEN" \
  | jq .
```

## Issues

List recent issues for an organization:

```bash
ORG_SLUG="example-org"
curl -fsS "$GLITCHTIP_API/organizations/$ORG_SLUG/issues/?sort=-last_seen&limit=25" \
  -H "Authorization: Bearer $HIVY_GLITCHTIP_TOKEN" \
  | jq '[.[] | {id, title, shortId, culprit, count, userCount, firstSeen, lastSeen, status, project: .project.slug}]'
```

Filter issues by project ID:

```bash
ORG_SLUG="example-org"
PROJECT_ID="123"
curl -fsS "$GLITCHTIP_API/organizations/$ORG_SLUG/issues/?project=$PROJECT_ID&sort=-last_seen&limit=25" \
  -H "Authorization: Bearer $HIVY_GLITCHTIP_TOKEN" \
  | jq '[.[] | {id, title, count, lastSeen, project: .project.slug}]'
```

Search issues:

```bash
ORG_SLUG="example-org"
QUERY="is:unresolved"
curl -G -fsS "$GLITCHTIP_API/organizations/$ORG_SLUG/issues/" \
  -H "Authorization: Bearer $HIVY_GLITCHTIP_TOKEN" \
  --data-urlencode "query=$QUERY" \
  --data-urlencode "sort=-last_seen" \
  --data-urlencode "limit=25" \
  | jq '[.[] | {id, title, status, count, lastSeen}]'
```

Get issue detail:

```bash
ISSUE_ID="123456"
curl -fsS "$GLITCHTIP_API/issues/$ISSUE_ID/" \
  -H "Authorization: Bearer $HIVY_GLITCHTIP_TOKEN" \
  | jq .
```

## Events

List issue events:

```bash
ISSUE_ID="123456"
curl -fsS "$GLITCHTIP_API/issues/$ISSUE_ID/events/?limit=3" \
  -H "Authorization: Bearer $HIVY_GLITCHTIP_TOKEN" \
  | jq '[.[] | {id, eventID, dateCreated, message, type}]'
```

Get the latest event for an issue:

```bash
ISSUE_ID="123456"
curl -fsS "$GLITCHTIP_API/issues/$ISSUE_ID/events/latest/" \
  -H "Authorization: Bearer $HIVY_GLITCHTIP_TOKEN" \
  | jq .
```

Get a specific event:

```bash
ISSUE_ID="123456"
EVENT_ID="abcdef"
curl -fsS "$GLITCHTIP_API/issues/$ISSUE_ID/events/$EVENT_ID/" \
  -H "Authorization: Bearer $HIVY_GLITCHTIP_TOKEN" \
  | jq .
```

Get event JSON by organization route:

```bash
ORG_SLUG="example-org"
ISSUE_ID="123456"
EVENT_ID="abcdef"
curl -fsS "$GLITCHTIP_API/organizations/$ORG_SLUG/issues/$ISSUE_ID/events/$EVENT_ID/json/" \
  -H "Authorization: Bearer $HIVY_GLITCHTIP_TOKEN" \
  | jq .
```

## Stacktrace Extraction

Use the latest event payload and inspect exceptions/threads:

```bash
ISSUE_ID="123456"
curl -fsS "$GLITCHTIP_API/issues/$ISSUE_ID/events/latest/" \
  -H "Authorization: Bearer $HIVY_GLITCHTIP_TOKEN" \
  | jq '{
      id,
      eventID,
      message,
      entry_types: [.entries[]?.type],
      exceptions: [.entries[]? | select(.type == "exception")],
      threads: [.entries[]? | select(.type == "threads")]
    }'
```

Some events are message-only and have no exception or thread entries. In that case `exceptions` and `threads` will be empty arrays and frame extraction will produce no lines.

Print exception frames:

```bash
ISSUE_ID="123456"
curl -fsS "$GLITCHTIP_API/issues/$ISSUE_ID/events/latest/" \
  -H "Authorization: Bearer $HIVY_GLITCHTIP_TOKEN" \
  | jq -r '
    .entries[]?
    | select(.type == "exception")
    | .data.values[]?
    | "Exception: \(.type // ""): \(.value // "")",
      (.stacktrace.frames[]? | "\(.filename // "?"):\(.lineNo // "?") in \(.function // "?")")
  '
```

## Tags

List tags for an issue:

```bash
ISSUE_ID="123456"
curl -fsS "$GLITCHTIP_API/issues/$ISSUE_ID/tags/" \
  -H "Authorization: Bearer $HIVY_GLITCHTIP_TOKEN" \
  | jq .
```

## Environments

List organization environments:

```bash
ORG_SLUG="example-org"
curl -fsS "$GLITCHTIP_API/organizations/$ORG_SLUG/environments/" \
  -H "Authorization: Bearer $HIVY_GLITCHTIP_TOKEN" \
  | jq .
```

List project environments:

```bash
ORG_SLUG="example-org"
PROJECT_SLUG="web"
curl -fsS "$GLITCHTIP_API/projects/$ORG_SLUG/$PROJECT_SLUG/environments/" \
  -H "Authorization: Bearer $HIVY_GLITCHTIP_TOKEN" \
  | jq .
```

## Releases

List releases:

```bash
ORG_SLUG="example-org"
curl -fsS "$GLITCHTIP_API/organizations/$ORG_SLUG/releases/?limit=25" \
  -H "Authorization: Bearer $HIVY_GLITCHTIP_TOKEN" \
  | jq '[.[] | {version, dateCreated, newGroups, lastEvent}]'
```

Get release detail:

```bash
ORG_SLUG="example-org"
VERSION="1.2.3"
VERSION_ENC="$(printf "%s" "$VERSION" | jq -sRr @uri)"
curl -fsS "$GLITCHTIP_API/organizations/$ORG_SLUG/releases/$VERSION_ENC/" \
  -H "Authorization: Bearer $HIVY_GLITCHTIP_TOKEN" \
  | jq .
```

## Monitors

List uptime/cron monitors:

```bash
ORG_SLUG="example-org"
curl -fsS "$GLITCHTIP_API/organizations/$ORG_SLUG/monitors/" \
  -H "Authorization: Bearer $HIVY_GLITCHTIP_TOKEN" \
  | jq .
```

Get monitor checks:

```bash
ORG_SLUG="example-org"
MONITOR_ID="123"
curl -fsS "$GLITCHTIP_API/organizations/$ORG_SLUG/monitors/$MONITOR_ID/checks/" \
  -H "Authorization: Bearer $HIVY_GLITCHTIP_TOKEN" \
  | jq .
```

## Logs

List logs:

```bash
ORG_SLUG="example-org"
curl -fsS "$GLITCHTIP_API/organizations/$ORG_SLUG/logs/?limit=25" \
  -H "Authorization: Bearer $HIVY_GLITCHTIP_TOKEN" \
  | jq .
```

Get log detail:

```bash
ORG_SLUG="example-org"
LOG_ID="123"
curl -fsS "$GLITCHTIP_API/organizations/$ORG_SLUG/logs/$LOG_ID/" \
  -H "Authorization: Bearer $HIVY_GLITCHTIP_TOKEN" \
  | jq .
```

## Standard Investigation Flow

1. List organizations and choose `ORG_SLUG`.
2. List projects and identify relevant project IDs/slugs.
3. List recent unresolved issues with `sort=-last_seen`.
4. Get issue detail and latest event.
5. Extract exception frames, tags, environment, release, and culprit.
6. Return a concise summary with issue ID, project, last seen time, frequency, stacktrace top frame, suspected cause, and dashboard URL when available.

Example summary command:

```bash
ORG_SLUG="example-org"
curl -fsS "$GLITCHTIP_API/organizations/$ORG_SLUG/issues/?sort=-last_seen&limit=10" \
  -H "Authorization: Bearer $HIVY_GLITCHTIP_TOKEN" \
  | jq --arg base "$HIVY_GLITCHTIP_DASHBOARD_BASE_URL" --arg org "$ORG_SLUG" '
    [.[] | {
      id,
      title,
      project: .project.slug,
      count,
      users: .userCount,
      last_seen: .lastSeen,
      status,
      dashboard_url: (if $base == "" then null else "\($base)/\($org)/issues/\(.id)/" end)
    }]'
```
