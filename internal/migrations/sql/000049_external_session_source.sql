-- +goose Up
UPDATE session_events
SET source = 'external'
WHERE source = 'slack'
  AND event_id LIKE 'slack:%';

UPDATE sessions
SET source = 'external'
WHERE source = 'slack'
  AND source_resource_key LIKE 'slack:%';

-- +goose Down
UPDATE session_events
SET source = 'slack'
WHERE source = 'external'
  AND event_id LIKE 'slack:%';

UPDATE sessions
SET source = 'slack'
WHERE source = 'external'
  AND source_resource_key LIKE 'slack:%';
