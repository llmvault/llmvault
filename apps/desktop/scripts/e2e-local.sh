#!/usr/bin/env bash
set -euo pipefail
trap 'printf "desktop e2e command failed at line %s\n" "$LINENO" >&2' ERR

api_url="${HIVY_DESKTOP_E2E_API_URL:-http://localhost:8080}"
runtime_url="${HIVY_DESKTOP_E2E_RUNTIME_URL:-http://127.0.0.1:37080}"
email="${HIVY_DESKTOP_E2E_EMAIL:-dev@hivy.local}"
password="${HIVY_DESKTOP_E2E_PASSWORD:-local-development}"
runtime_secret="${HIVY_DESKTOP_RUNTIME_SECRET:?set HIVY_DESKTOP_RUNTIME_SECRET to the desktop dev runtime bearer}"
expected="desktop runtime works"

login=$(curl -fsS -X POST "$api_url/auth/login" \
  -H 'content-type: application/json' \
  --data "{\"email\":\"$email\",\"password\":\"$password\"}")
access_token=$(printf '%s' "$login" | jq -er .access_token)
org_id=$(printf '%s' "$login" | jq -er '.orgs[0].id')
auth_header="authorization: Bearer $access_token"
org_header="X-Org-ID: $org_id"

agents=$(curl -fsS "$api_url/v1/agents" -H "$auth_header" -H "$org_header")
agent_id=$(printf '%s' "$agents" | jq -er 'first(.data[] | select(.name == "Hivy") | .id)')
team_id=$(printf '%s' "$agents" | jq -er 'first(.data[] | select(.name == "Hivy") | .team_id)')
secondary_agent_id=$(printf '%s' "$agents" | jq -r \
  'first(.data[] | select(.name == "Desktop E2E Secondary") | .id) // empty')
if [[ -z "$secondary_agent_id" ]]; then
  secondary_agent_id=$(curl -fsS -X POST "$api_url/v1/agents" \
    -H "$auth_header" -H "$org_header" -H 'content-type: application/json' \
    --data "{\"name\":\"Desktop E2E Secondary\",\"team_id\":\"$team_id\"}" | jq -er .agent.id)
fi

bootstrap=$(curl -fsS -X POST \
  "$api_url/v1/desktop/agents/$agent_id/runtime-config" \
  -H "$auth_header" -H "$org_header")
printf '%s' "$bootstrap" | jq -c .config | curl -fsS -X PUT \
  "$runtime_url/desktop/agents/$agent_id/config" \
  -H "authorization: Bearer $runtime_secret" \
  -H 'content-type: application/json' --data-binary @- >/dev/null

created=$(curl -fsS -X POST "$api_url/v1/desktop/sessions" \
  -H "$auth_header" -H "$org_header" -H 'content-type: application/json' \
  --data "{\"agent_id\":\"$agent_id\",\"text\":\"Reply with exactly: $expected\"}")
session_id=$(printf '%s' "$created" | jq -er .session.id)

delivery=$(printf '%s' "$created" | jq -c .runtime_request | curl -fsS -X POST \
  "$runtime_url/desktop/agents/$agent_id/sessions/$session_id/messages" \
  -H "authorization: Bearer $runtime_secret" \
  -H 'content-type: application/json' --data-binary @-)
stream_id=$(printf '%s' "$delivery" | jq -er .stream_id)
turn_id=$(printf '%s' "$delivery" | jq -er .turn_id)

curl -fsS -X POST "$api_url/v1/desktop/sessions/$session_id/delivery" \
  -H "$auth_header" -H "$org_header" -H 'content-type: application/json' \
  --data "{\"stream_id\":\"$stream_id\",\"turn_id\":\"$turn_id\"}" >/dev/null

primary_passed=false
for _ in $(seq 1 90); do
  detail=$(curl -fsS "$runtime_url/sessions/$session_id" \
    -H "authorization: Bearer $runtime_secret")
  answer=$(printf '%s' "$detail" | jq -r \
    '[.events[] | select(.kind == "assistant_message") | .payload.message.parts[]? | select(.type == "text") | .text] | first // ""')
  if [[ "$answer" == "$expected" ]]; then
    cloud_events=$(curl -fsS "$api_url/v1/sessions/$session_id/events" \
      -H "$auth_header" -H "$org_header")
    if printf '%s' "$cloud_events" | jq -e \
      --arg turn_id "$turn_id" \
      'any(.data[]; .event_type == "turn_completed" and .turn_id == $turn_id)' >/dev/null; then
      primary_passed=true
      break
    fi
  fi
  sleep 1
done

if [[ "$primary_passed" != true ]]; then
  printf 'desktop e2e failed: no matching assistant response for session %s\n' "$session_id" >&2
  exit 1
fi

# Register two real cloud-agent configurations in one runtime, activate each on
# a separate turn, and verify the active definition between completed turns.
secondary_bootstrap=$(curl -fsS -X POST \
  "$api_url/v1/desktop/agents/$secondary_agent_id/runtime-config" \
  -H "$auth_header" -H "$org_header")
printf '%s' "$secondary_bootstrap" | jq -c \
  '.config | .definition.agent.name = "Desktop E2E Secondary"' | curl -fsS -X PUT \
  "$runtime_url/desktop/agents/$secondary_agent_id/config" \
  -H "authorization: Bearer $runtime_secret" \
  -H 'content-type: application/json' --data-binary @- >/dev/null
printf '%s' "$bootstrap" | jq -c .config | curl -fsS -X PUT \
  "$runtime_url/desktop/agents/$agent_id/config" \
  -H "authorization: Bearer $runtime_secret" \
  -H 'content-type: application/json' --data-binary @- >/dev/null

switched=$(curl -fsS -X POST "$api_url/v1/desktop/sessions" \
  -H "$auth_header" -H "$org_header" -H 'content-type: application/json' \
  --data "{\"agent_id\":\"$secondary_agent_id\",\"text\":\"Reply with exactly: secondary ready\"}")
switch_session_id=$(printf '%s' "$switched" | jq -er .session.id)
secondary_delivery=$(printf '%s' "$switched" | jq -c .runtime_request | curl -fsS -X POST \
  "$runtime_url/desktop/agents/$secondary_agent_id/sessions/$switch_session_id/messages" \
  -H "authorization: Bearer $runtime_secret" \
  -H 'content-type: application/json' --data-binary @-)
secondary_stream_id=$(printf '%s' "$secondary_delivery" | jq -er .stream_id)
secondary_turn_id=$(printf '%s' "$secondary_delivery" | jq -er .turn_id)
curl -fsS -X POST "$api_url/v1/desktop/sessions/$switch_session_id/delivery" \
  -H "$auth_header" -H "$org_header" -H 'content-type: application/json' \
  --data "{\"stream_id\":\"$secondary_stream_id\",\"turn_id\":\"$secondary_turn_id\"}" >/dev/null

secondary_passed=false
for _ in $(seq 1 90); do
  detail=$(curl -fsS "$runtime_url/sessions/$switch_session_id" \
    -H "authorization: Bearer $runtime_secret")
  answer=$(printf '%s' "$detail" | jq -r \
    '[.events[] | select(.kind == "assistant_message") | .payload.message.parts[]? | select(.type == "text") | .text] | first // ""')
  active_name=$(curl -fsS "$runtime_url/config" \
    -H "authorization: Bearer $runtime_secret" | jq -r .agent.name)
  if [[ "$answer" == "secondary ready" && "$active_name" == "Desktop E2E Secondary" ]]; then
    secondary_passed=true
    break
  fi
  sleep 1
done
if [[ "$secondary_passed" != true ]]; then
  printf 'desktop e2e failed: secondary agent configuration did not activate\n' >&2
  exit 1
fi

prepared=$(curl -fsS -X POST "$api_url/v1/desktop/sessions" \
  -H "$auth_header" -H "$org_header" -H 'content-type: application/json' \
  --data "{\"agent_id\":\"$agent_id\",\"text\":\"Reply with exactly: primary ready\"}")
primary_switch_session_id=$(printf '%s' "$prepared" | jq -er .session.id)
primary_delivery=$(printf '%s' "$prepared" | jq -c .runtime_request | curl -fsS -X POST \
  "$runtime_url/desktop/agents/$agent_id/sessions/$primary_switch_session_id/messages" \
  -H "authorization: Bearer $runtime_secret" \
  -H 'content-type: application/json' --data-binary @-)
primary_stream_id=$(printf '%s' "$primary_delivery" | jq -er .stream_id)
primary_turn_id=$(printf '%s' "$primary_delivery" | jq -er .turn_id)
curl -fsS -X POST "$api_url/v1/desktop/sessions/$primary_switch_session_id/delivery" \
  -H "$auth_header" -H "$org_header" -H 'content-type: application/json' \
  --data "{\"stream_id\":\"$primary_stream_id\",\"turn_id\":\"$primary_turn_id\"}" >/dev/null

switch_passed=false
for _ in $(seq 1 90); do
  detail=$(curl -fsS "$runtime_url/sessions/$primary_switch_session_id" \
    -H "authorization: Bearer $runtime_secret")
  primary_answer=$(printf '%s' "$detail" | jq -r \
    '[.events[] | select(.kind == "assistant_message") | .payload.message.parts[]? | select(.type == "text") | .text] | first // ""')
  active_name=$(curl -fsS "$runtime_url/config" \
    -H "authorization: Bearer $runtime_secret" | jq -r .agent.name)
  cloud_events=$(curl -fsS "$api_url/v1/sessions/$primary_switch_session_id/events" \
    -H "$auth_header" -H "$org_header")
  if [[ "$primary_answer" == "primary ready" && "$active_name" == "Hivy" ]] && \
    printf '%s' "$cloud_events" | jq -e --arg turn_id "$primary_turn_id" \
      'any(.data[]; .event_type == "turn_completed" and .turn_id == $turn_id)' >/dev/null; then
    switch_passed=true
    break
  fi
  sleep 1
done
if [[ "$switch_passed" != true ]]; then
  printf 'desktop e2e failed: primary agent did not resume with cloud backup\n' >&2
  exit 1
fi

mcp_created=$(curl -fsS -X POST "$api_url/v1/desktop/sessions" \
  -H "$auth_header" -H "$org_header" -H 'content-type: application/json' \
  --data "{\"agent_id\":\"$agent_id\",\"text\":\"Call the list_agents MCP tool once, then reply with exactly: MCP desktop works\"}")
mcp_session_id=$(printf '%s' "$mcp_created" | jq -er .session.id)
mcp_delivery=$(printf '%s' "$mcp_created" | jq -c .runtime_request | curl -fsS -X POST \
  "$runtime_url/desktop/agents/$agent_id/sessions/$mcp_session_id/messages" \
  -H "authorization: Bearer $runtime_secret" \
  -H 'content-type: application/json' --data-binary @-)
mcp_stream_id=$(printf '%s' "$mcp_delivery" | jq -er .stream_id)
mcp_turn_id=$(printf '%s' "$mcp_delivery" | jq -er .turn_id)
curl -fsS -X POST "$api_url/v1/desktop/sessions/$mcp_session_id/delivery" \
  -H "$auth_header" -H "$org_header" -H 'content-type: application/json' \
  --data "{\"stream_id\":\"$mcp_stream_id\",\"turn_id\":\"$mcp_turn_id\"}" >/dev/null

mcp_passed=false
for _ in $(seq 1 90); do
  detail=$(curl -fsS "$runtime_url/sessions/$mcp_session_id" \
    -H "authorization: Bearer $runtime_secret")
  mcp_answer=$(printf '%s' "$detail" | jq -r \
    '[.events[] | select(.kind == "assistant_message") | .payload.message.parts[]? | select(.type == "text") | .text] | first // ""')
  mcp_tool_mentions=$(printf '%s' "$detail" | jq \
    '[.events[] | .. | strings | select(test("list_agents"))] | length')
  cloud_events=$(curl -fsS "$api_url/v1/sessions/$mcp_session_id/events" \
    -H "$auth_header" -H "$org_header")
  if [[ "$mcp_answer" == "MCP desktop works" && "$mcp_tool_mentions" -gt 0 ]] && \
    printf '%s' "$cloud_events" | jq -e --arg turn_id "$mcp_turn_id" \
      'any(.data[]; .event_type == "turn_completed" and .turn_id == $turn_id)' >/dev/null; then
    mcp_passed=true
    break
  fi
  sleep 1
done
if [[ "$mcp_passed" != true ]]; then
  printf 'desktop e2e failed: MCP tool call did not complete with cloud backup\n' >&2
  exit 1
fi

printf 'desktop e2e passed: session=%s turn=%s answer=%q; multi-agent session=%s; mcp session=%s\n' \
  "$session_id" "$turn_id" "$expected" "$switch_session_id" "$mcp_session_id"
