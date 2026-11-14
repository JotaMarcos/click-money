-- +goose Up

-- Equivalente de desenvolvimento da migração do PostgreSQL. O SQLite não tem
-- papéis nem Row-Level Security: aqui o isolamento entre comércios depende só
-- da aplicação. Identificadores são texto (UUID) e instantes são texto em UTC
-- com largura fixa (AAAA-MM-DDTHH:MM:SS.ffffffZ), o que preserva a ordenação.

CREATE TABLE tenants (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL CHECK (length(trim(name)) BETWEEN 2 AND 120),
    timezone   TEXT NOT NULL,
    plan       TEXT NOT NULL CHECK (plan IN ('FREE', 'PAID', 'ASSOCIATION')),
    status     TEXT NOT NULL CHECK (status IN ('ACTIVE', 'SUSPENDED')),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
) STRICT;

CREATE TABLE outbox_events (
    id              TEXT    PRIMARY KEY,
    tenant_id       TEXT    NOT NULL REFERENCES tenants (id) ON DELETE RESTRICT,
    aggregate_type  TEXT    NOT NULL,
    aggregate_id    TEXT    NOT NULL,
    event_type      TEXT    NOT NULL,
    payload         TEXT    NOT NULL CHECK (json_valid(payload)),
    occurred_at     TEXT    NOT NULL,
    attempts        INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    next_attempt_at TEXT    NOT NULL,
    last_error      TEXT,
    published_at    TEXT,
    failed_at       TEXT
) STRICT;

CREATE INDEX outbox_events_pending_idx
    ON outbox_events (next_attempt_at, id)
    WHERE published_at IS NULL AND failed_at IS NULL;

CREATE INDEX outbox_events_aggregate_pending_idx
    ON outbox_events (aggregate_id, id)
    WHERE published_at IS NULL;

-- +goose Down

DROP TABLE outbox_events;
DROP TABLE tenants;
