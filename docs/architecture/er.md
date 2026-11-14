# Diagrama ER (PostgreSQL)

Modelo relacional da Fase 1. Tipos na notação do PostgreSQL; no SQLite, `uuid` e `timestamptz` viram `text` (ADR 0002). Toda tabela tem `created_at`, e as mutáveis têm `updated_at`; essas colunas foram omitidas do diagrama quando não acrescentam informação.

```mermaid
erDiagram
    users ||--o{ memberships : "participa de"
    users ||--o{ refresh_tokens : "possui"
    users ||--o{ devices : "registra"
    tenants ||--o{ memberships : "tem"
    tenants ||--|| tenant_settings : "configura"
    tenants ||--|| tenant_sync_states : "sequência de sync"
    tenants ||--o{ devices : "autoriza"
    tenants ||--o{ customers : "atende"
    devices ||--o{ refresh_tokens : "sessões"
    customers ||--o{ customer_consents : "consente"
    customers ||--|| customer_balances : "saldo projetado"
    customers ||--o{ ledger_entries : "movimenta"
    ledger_entries ||--o| ledger_entries : "estorna"
    ledger_entries ||--o| credit_sale_balances : "em aberto"
    ledger_entries ||--o{ payment_allocations : "pagamento"
    ledger_entries ||--o{ payment_allocations : "compra"
    customers ||--o{ credit_limit_overrides : "liberações"
    tenants ||--o{ cash_closings : "fecha caixa"
    devices ||--o{ sync_commands : "envia"
    sync_commands ||--o| review_items : "gera"
    tenants ||--o{ tenant_changes : "registra mudanças"
    tenants ||--o{ outbox_events : "emite"
    tenants ||--o{ idempotency_keys : "guarda"

    users {
        uuid id PK
        text name
        text phone_e164 UK
        text email
        text password_hash
        timestamptz locked_until
    }
    refresh_tokens {
        uuid id PK
        uuid user_id FK
        uuid device_id FK
        uuid tenant_id FK
        uuid family_id
        bytea token_hash UK
        bytea access_token_hash UK
        timestamptz access_expires_at
        timestamptz expires_at
        timestamptz rotated_at
        timestamptz revoked_at
    }
    tenants {
        uuid id PK
        text name
        text timezone
        text plan
        text status
    }
    tenant_settings {
        uuid tenant_id PK
        text overpayment_policy
        text credit_limit_mode
        int max_backdate_days
        int collection_start_hour
        int collection_end_hour
        boolean collection_on_sundays
        boolean collection_on_holidays
        boolean interest_enabled
        int late_fee_bps
        int monthly_interest_bps
        text pix_key
        text pix_merchant_name
        text pix_merchant_city
        boolean assistant_enabled
        bigint version
    }
    memberships {
        uuid tenant_id PK
        uuid user_id PK
        text role
        timestamptz revoked_at
    }
    devices {
        uuid id PK
        uuid tenant_id FK
        uuid user_id FK
        text name
        timestamptz last_seen_at
        timestamptz revoked_at
    }
    customers {
        uuid id PK
        uuid tenant_id FK
        text name
        text nickname
        text phone_e164
        text community
        text reference_point
        text notes
        bigint credit_limit_cents
        text preferred_payment_kind
        int preferred_payment_day
        text preferred_payment_note
        bigint version
        timestamptz archived_at
        timestamptz anonymized_at
    }
    customer_consents {
        uuid id PK
        uuid tenant_id FK
        uuid customer_id FK
        text channel
        text status
        text evidence
        timestamptz recorded_at
        uuid recorded_by FK
    }
    customer_balances {
        uuid tenant_id PK
        uuid customer_id PK
        bigint balance_cents
        int open_sales_count
        date oldest_open_sale_on
        date last_payment_on
        bigint version
    }
    ledger_entries {
        uuid id PK
        uuid tenant_id FK
        uuid customer_id FK
        text type
        text direction
        bigint amount_cents
        date occurred_on
        timestamptz recorded_at
        timestamptz client_recorded_at
        text payment_method
        text description
        text reason
        uuid device_id FK
        uuid author_id FK
        text idempotency_key
        uuid reverses_entry_id FK
    }
    credit_sale_balances {
        uuid tenant_id PK
        uuid entry_id PK
        uuid customer_id FK
        date occurred_on
        bigint original_cents
        bigint open_cents
        text status
    }
    payment_allocations {
        uuid id PK
        uuid tenant_id FK
        uuid payment_entry_id FK
        uuid sale_entry_id FK
        uuid source_entry_id FK
        bigint amount_cents
        text kind
    }
    credit_limit_overrides {
        uuid id PK
        uuid tenant_id FK
        uuid customer_id FK
        uuid entry_id FK
        uuid granted_by FK
        text reason
    }
    cash_closings {
        uuid id PK
        uuid tenant_id FK
        uuid operator_id FK
        date closed_on
        bigint cash_cents
        bigint pix_cents
        bigint card_cents
        bigint other_cents
        timestamptz closed_at
    }
    sync_commands {
        uuid command_id PK
        uuid tenant_id FK
        uuid device_id FK
        text type
        text status
        text reject_code
        jsonb result
        timestamptz client_recorded_at
        timestamptz received_at
    }
    review_items {
        uuid id PK
        uuid tenant_id FK
        uuid command_id FK
        text reason_code
        jsonb command
        text status
        text resolution
        uuid resolved_by FK
        timestamptz resolved_at
    }
    tenant_sync_states {
        uuid tenant_id PK
        bigint last_seq
    }
    tenant_changes {
        uuid tenant_id PK
        bigint seq PK
        text entity_type
        uuid entity_id
        text operation
    }
    outbox_events {
        uuid id PK
        uuid tenant_id FK
        text aggregate_type
        uuid aggregate_id
        text event_type
        jsonb payload
        int attempts
        timestamptz next_attempt_at
        timestamptz published_at
    }
    idempotency_keys {
        uuid tenant_id PK
        text key PK
        bytea request_hash
        int response_status
        jsonb response_body
        timestamptz expires_at
    }
```

## Constraints e índices que o diagrama não mostra

| Tabela | Regra |
| --- | --- |
| `ledger_entries` | `CHECK (amount_cents > 0)`; `CHECK` de `type`, `direction` e `payment_method`; `CHECK` que amarra tipo e direção; `UNIQUE (tenant_id, idempotency_key)`; `UNIQUE (reverses_entry_id)`; trigger e permissão que negam `UPDATE` e `DELETE`; FKs `ON DELETE RESTRICT` |
| `ledger_entries` | Índice `(tenant_id, customer_id, occurred_on DESC, id DESC)` para o extrato; índice `(tenant_id, occurred_on, author_id)` para o fechamento de caixa |
| `customers` | Índice único parcial `(tenant_id, phone_e164) WHERE archived_at IS NULL AND phone_e164 IS NOT NULL`; índice de busca por nome e apelido normalizados |
| `customer_balances` | Índice `(tenant_id, balance_cents DESC, customer_id)` para a lista de devedores; índice `(tenant_id, oldest_open_sale_on)` para o aging |
| `credit_sale_balances` | `CHECK (open_cents BETWEEN 0 AND original_cents)`; índice parcial `(tenant_id, customer_id, occurred_on, entry_id) WHERE open_cents > 0` para a alocação FIFO |
| `payment_allocations` | `CHECK (amount_cents > 0)`; `kind` em `ALLOCATE` ou `RELEASE`; somente-inserção |
| `memberships` | `role` em `OWNER`, `MANAGER`, `CASHIER`; ao menos um `OWNER` ativo por tenant (verificado no caso de uso) |
| `outbox_events` | Índice parcial `(next_attempt_at) WHERE published_at IS NULL` |
| Todas as de negócio | RLS por `tenant_id` (ADR 0008); FKs compostas com `tenant_id` |

## Coleções do MongoDB (Fase 2)

| Coleção | Conteúdo | Retenção (TTL) **[PREMISSA]** |
| --- | --- | --- |
| `audit_trail` | Quem, quando, de qual dispositivo, antes e depois | 5 anos |
| `sync_logs` | Lotes de push e pull por dispositivo | 90 dias |
| `messages` | Cobranças e comprovantes enviados, status de entrega | 2 anos |
| `message_templates` | Modelos de mensagem por tenant | sem expiração |
| `assistant_runs` | Passos, ferramentas, tokens, custo, avaliação | 180 dias |
| `read_models` | Aging e indicadores pré-agregados | reconstruível |

Todas com validação `$jsonSchema` e índice iniciado por `tenant_id`.
