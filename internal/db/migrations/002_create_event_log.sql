-- Idempotency guard: one row per unique event.
-- event_hash = hex(SHA256(chamado_id || tipo || timestamp_rfc3339))
CREATE TABLE IF NOT EXISTS event_log (
    event_hash  TEXT        PRIMARY KEY,
    received_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
