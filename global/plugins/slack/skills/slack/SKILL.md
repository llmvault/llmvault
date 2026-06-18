---
name: slack
description: Use when reading Slack conversations, searching workspace messages, summarizing channels or threads, inspecting users or channels, drafting replies, or sending Slack messages.
---

# Slack

Slack access is provided through the organization's connected Slack app.

Do not call the real Slack API directly. Use the provided proxy endpoint and environment variables.

Normal mention replies and thread responses are handled automatically. Use this skill only when the user explicitly asks you to inspect or act on Slack workspace data.

## Environment

Required:

| Variable | Purpose |
|---|---|
| `HIVY_SLACK_API_URL` | Provided Slack Web API proxy base URL |
| `HIVY_SLACK_TOKEN` | Bearer token for the provided Slack endpoint |

Initialize once:

```bash
test -n "$HIVY_SLACK_API_URL" || { echo "HIVY_SLACK_API_URL is not set" >&2; exit 1; }
test -n "$HIVY_SLACK_TOKEN" || { echo "HIVY_SLACK_TOKEN is not set" >&2; exit 1; }
HIVY_SLACK_API_URL="${HIVY_SLACK_API_URL%/}"
```

Use this helper for Slack Web API calls:

```bash
slack_api() {
  local method="$1"
  local api_path="$2"
  local body="${3:-}"
  if test -n "$body"; then
    curl -fsS -X "$method" "$HIVY_SLACK_API_URL$api_path" \
      -H "Authorization: Bearer $HIVY_SLACK_TOKEN" \
      -H "Content-Type: application/json" \
      --data-binary "$body"
  else
    curl -fsS -X "$method" "$HIVY_SLACK_API_URL$api_path" \
      -H "Authorization: Bearer $HIVY_SLACK_TOKEN"
  fi
}
```

## Common Operations

Check the connected bot/user:

```bash
slack_api GET "/auth.test" | jq '{ok, team, user, bot_id, url}'
```

List public and private channels visible to the app:

```bash
slack_api GET "/conversations.list?types=public_channel,private_channel&limit=100" \
  | jq '[.channels[] | {id, name, is_member, is_private, updated: .updated}]'
```

Fetch recent messages from a channel:

```bash
CHANNEL_ID="C..."
slack_api GET "/conversations.history?channel=$CHANNEL_ID&limit=20" \
  | jq '[.messages[] | {ts, user, bot_id, text}]'
```

Fetch a thread:

```bash
CHANNEL_ID="C..."
THREAD_TS="1780000000.000000"
slack_api GET "/conversations.replies?channel=$CHANNEL_ID&ts=$THREAD_TS&limit=50" \
  | jq '[.messages[] | {ts, user, bot_id, text}]'
```

Post only when the user explicitly asks you to send a Slack message:

```bash
slack_api POST "/chat.postMessage" "$(jq -n \
  --arg channel "$CHANNEL_ID" \
  --arg text "Message text" \
  '{channel: $channel, text: $text}')" \
  | jq '{ok, channel, ts, message: {text: .message.text}}'
```

## Rules

- Do not use this skill to send the assistant's normal reply; that is handled automatically.
- Always filter Slack responses with `jq`.
- Never print `$HIVY_SLACK_TOKEN`.
- Do not call `https://slack.com/api` directly from the runtime. This will fail.
- Prefer read-only operations unless the user explicitly asks for a Slack write action.
- Delete, remove, archive, trash, and destroy operations are blocked by the provided proxy. If the user asks for one of these actions, explain that they must perform it themselves in Slack.
