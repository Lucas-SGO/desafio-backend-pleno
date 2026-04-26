CREATE TABLE IF NOT EXISTS notifications (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    cpf_hash        TEXT        NOT NULL,
    chamado_id      TEXT        NOT NULL,
    titulo          TEXT        NOT NULL,
    descricao       TEXT        NOT NULL,
    status_anterior TEXT        NOT NULL,
    status_novo     TEXT        NOT NULL,
    is_read         BOOLEAN     NOT NULL DEFAULT FALSE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_notifications_cpf_hash
    ON notifications (cpf_hash, created_at DESC);
