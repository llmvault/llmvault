-- +goose Up
-- Idempotency key for runtime session events. The runtime outbox redelivers an
-- event whenever it does not observe a clean 2xx ack (lost ack, batch with a
-- failed item redelivered whole). Without a stable dedupe key a redelivery
-- inserts a second row, duplicating the agent's reply in the persisted timeline
-- and re-driving memory-retain / gateway delivery (P0-15).
--
-- The runtime does not stamp a stable per-event id and its `sequence` is
-- turn-local, so the Go ingestion boundary derives a content-hash event_id when
-- one is absent (see deriveSessionEventID). A redelivery carries the
-- byte-identical event, so the hash is stable across retries yet distinct per
-- event. This partial unique index lets the inserts dedupe via ON CONFLICT DO
-- NOTHING; legacy rows with an empty event_id are excluded.
CREATE UNIQUE INDEX IF NOT EXISTS idx_employee_session_events_sandbox_event
    ON employee_session_events (sandbox_id, event_id)
    WHERE event_id <> '';

-- +goose Down
DROP INDEX IF EXISTS idx_employee_session_events_sandbox_event;
