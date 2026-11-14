-- +goose Up

-- Papéis de grupo, sem login. Os usuários reais da aplicação e do worker são
-- criados pela infraestrutura e recebem um destes papéis. Nenhum deles é dono
-- das tabelas nem tem BYPASSRLS; o usuário de migração é separado.
-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'click_money_app') THEN
        CREATE ROLE click_money_app NOLOGIN;
    END IF;
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'click_money_worker') THEN
        CREATE ROLE click_money_worker NOLOGIN;
    END IF;
END
$$;
-- +goose StatementEnd

CREATE TABLE tenants (
    id         uuid        PRIMARY KEY,
    name       text        NOT NULL CHECK (length(btrim(name)) BETWEEN 2 AND 120),
    timezone   text        NOT NULL,
    plan       text        NOT NULL CHECK (plan IN ('FREE', 'PAID', 'ASSOCIATION')),
    status     text        NOT NULL CHECK (status IN ('ACTIVE', 'SUSPENDED')),
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL
);

CREATE TABLE outbox_events (
    id              uuid        PRIMARY KEY,
    tenant_id       uuid        NOT NULL REFERENCES tenants (id) ON DELETE RESTRICT,
    aggregate_type  text        NOT NULL,
    aggregate_id    uuid        NOT NULL,
    event_type      text        NOT NULL,
    payload         jsonb       NOT NULL,
    occurred_at     timestamptz NOT NULL,
    attempts        integer     NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    next_attempt_at timestamptz NOT NULL,
    last_error      text,
    published_at    timestamptz,
    failed_at       timestamptz
);

-- Fila do worker: só eventos pendentes, na ordem de entrega.
CREATE INDEX outbox_events_pending_idx
    ON outbox_events (next_attempt_at, id)
    WHERE published_at IS NULL AND failed_at IS NULL;

-- Garante a ordem por agregado: um evento só sai depois dos anteriores.
CREATE INDEX outbox_events_aggregate_pending_idx
    ON outbox_events (aggregate_id, id)
    WHERE published_at IS NULL;

-- Isolamento entre comércios. FORCE aplica as políticas também ao dono das
-- tabelas. current_setting sem o segundo argumento falha quando a variável
-- não foi definida: uma transação sem tenant é negada, não liberada.
ALTER TABLE tenants ENABLE ROW LEVEL SECURITY;
ALTER TABLE tenants FORCE ROW LEVEL SECURITY;
ALTER TABLE outbox_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE outbox_events FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON tenants
    FOR ALL TO click_money_app
    USING (id = current_setting('app.tenant_id')::uuid)
    WITH CHECK (id = current_setting('app.tenant_id')::uuid);

CREATE POLICY tenant_isolation ON outbox_events
    FOR ALL TO click_money_app
    USING (tenant_id = current_setting('app.tenant_id')::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::uuid);

-- O worker entrega eventos de todos os comércios, por isso enxerga a outbox
-- inteira; para tocar em dados de negócio ele assume o tenant de cada evento.
CREATE POLICY worker_access ON outbox_events
    FOR ALL TO click_money_worker
    USING (true)
    WITH CHECK (true);

CREATE POLICY worker_read ON tenants
    FOR SELECT TO click_money_worker
    USING (true);

GRANT SELECT, INSERT, UPDATE ON tenants TO click_money_app;
GRANT SELECT, INSERT ON outbox_events TO click_money_app;
GRANT SELECT ON tenants TO click_money_worker;
GRANT SELECT, UPDATE, DELETE ON outbox_events TO click_money_worker;

-- +goose Down

DROP TABLE outbox_events;
DROP TABLE tenants;
-- Os papéis click_money_app e click_money_worker são do cluster, não deste
-- banco, e podem estar em uso por outro ambiente; por isso não são removidos.
