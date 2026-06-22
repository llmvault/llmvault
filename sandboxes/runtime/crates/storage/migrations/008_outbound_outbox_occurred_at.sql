ALTER TABLE outbound_outbox ADD COLUMN occurred_at TEXT NOT NULL DEFAULT '';

UPDATE outbound_outbox
SET occurred_at = CASE
    WHEN json_valid(payload_json) THEN COALESCE(
        NULLIF(TRIM(json_extract(payload_json, '$.occurred_at')), ''),
        created_at
    )
    ELSE created_at
END
WHERE occurred_at = '';
