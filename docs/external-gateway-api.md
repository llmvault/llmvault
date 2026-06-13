# External Gateway API

The External Gateway API lets a customer-owned chat wrapper send messages into a Hivy agent session and receive the final assistant reply by callback.

## 1. Create a gateway route

Create one route per external app or channel integration.

```http
POST /v1/agents/{agent_id}/gateway-routes
Authorization: Bearer <Hivy user JWT or org API key with agents scope>
Content-Type: application/json
```

```json
{
  "name": "WhatsApp support group",
  "provider": "whatsapp",
  "callback_url": "https://your-app.example.com/hivy/callback",
  "enabled": true
}
```

The response includes a `secret` once. Store it securely.

```json
{
  "id": "route_uuid",
  "provider": "whatsapp",
  "inbound_url": "https://api.usehivy.com/incoming/gateways/external/route_uuid",
  "callback_url": "https://your-app.example.com/hivy/callback",
  "secret": "hvgw_...",
  "secret_prefix": "hvgw_abcd1234",
  "enabled": true
}
```

## 2. Forward messages to Hivy

```bash
curl -X POST "$HIVY_INBOUND_URL" \
  -H "Authorization: Bearer $HIVY_GATEWAY_SECRET" \
  -H "Content-Type: application/json" \
  -d '{
    "message_id": "wa-msg-123",
    "thread_id": "wa-group-999",
    "channel_id": "wa-group-999",
    "sender": {"id": "user-123", "name": "Ada"},
    "text": "Can you check today sales?"
  }'
```

Rules:

- `message_id` must be stable across retries. Hivy uses it for dedupe.
- `thread_id` controls the Hivy session. Reuse it to continue a conversation.
- `channel_id` should identify the external group/channel where the reply should be posted.
- Do not forward messages sent by your wrapper bot itself.
- The HTTP response is only an acknowledgement. The final answer arrives by callback.

Accepted response:

```json
{
  "status": "accepted",
  "duplicate": false,
  "event_id": "event_uuid",
  "agent_session_id": "session_uuid",
  "runtime_session_id": "runtime-session-id",
  "runtime_trace_id": "trace-id",
  "runtime_turn_id": "turn-id"
}
```

Duplicate response:

```json
{
  "status": "duplicate",
  "duplicate": true,
  "event_id": "event_uuid"
}
```

## 3. Receive replies

Hivy posts the final assistant response to the route `callback_url`.

```http
POST https://your-app.example.com/hivy/callback
Content-Type: application/json
X-Hivy-Gateway-Route-ID: route_uuid
X-Hivy-Gateway-Event-ID: event_uuid
X-Hivy-Signature: sha256=<hmac>
```

```json
{
  "route_id": "route_uuid",
  "event_id": "event_uuid",
  "agent_session_id": "session_uuid",
  "runtime_session_id": "runtime-session-id",
  "runtime_trace_id": "trace-id",
  "runtime_turn_id": "turn-id",
  "thread_id": "wa-group-999",
  "thread_key": "wa-group-999",
  "channel_id": "wa-group-999",
  "provider": "whatsapp",
  "response_id": "response_id",
  "text": "Today's sales are...",
  "markdown": "Today's sales are..."
}
```

Verify `X-Hivy-Signature` by computing HMAC-SHA256 over the raw request body using the route secret.

