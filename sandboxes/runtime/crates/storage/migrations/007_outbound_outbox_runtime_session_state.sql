ALTER TABLE outbound_outbox ADD COLUMN session_id TEXT;
ALTER TABLE outbound_outbox ADD COLUMN runtime_seq INTEGER;
ALTER TABLE outbound_outbox ADD COLUMN lease_until TEXT;

UPDATE outbound_outbox
SET
    session_id = NULLIF(TRIM(json_extract(payload_json, '$.session_id')), ''),
    runtime_seq = CAST(json_extract(payload_json, '$.runtime_seq') AS INTEGER)
WHERE json_valid(payload_json)
  AND NULLIF(TRIM(COALESCE(json_extract(payload_json, '$.session_id'), '')), '') IS NOT NULL
  AND json_type(payload_json, '$.runtime_seq') = 'integer';

CREATE INDEX idx_outbox_due_lease
    ON outbound_outbox(status, next_retry_at, lease_until);

CREATE INDEX idx_outbox_session_head
    ON outbound_outbox(channel_name, session_id, status, runtime_seq, id)
    WHERE session_id IS NOT NULL;

CREATE UNIQUE INDEX idx_outbox_runtime_identity
    ON outbound_outbox(channel_name, session_id, runtime_seq)
    WHERE session_id IS NOT NULL AND runtime_seq IS NOT NULL;
