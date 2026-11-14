# ADR 0002 — Persistência poliglota com SQLite em desenvolvimento

- Status: Aceita
- Data: 2026-11-13

## Contexto

O núcleo financeiro precisa de ACID, constraints e Row-Level Security. Auditoria detalhada, histórico de mensagens, rastros do assistente e read models têm volume alto e esquema flexível. O desenvolvedor deve conseguir rodar o sistema sem instalar nada além do Go.

## Decisão

| Ambiente | Relacional | Documentos |
| --- | --- | --- |
| Produção e homologação | PostgreSQL 16+ | MongoDB 7+ em replica set |
| Desenvolvimento local | SQLite (`modernc.org/sqlite`, WAL, `foreign_keys=ON`) | SQLite, tabela `documents` com JSON1 |
| Integração e CI | PostgreSQL e MongoDB reais via testcontainers-go | idem |

- PostgreSQL é a única fonte da verdade financeira. Nenhuma regra de negócio lê do MongoDB.
- MongoDB recebe apenas dados derivados, projetados pelo `worker` a partir da outbox. Se ele cair, os eventos ficam na outbox e `/readyz` reporta "degradado" sem derrubar a prontidão.
- **[PREMISSA]** Na Fase 1 o MongoDB não é implantado em produção. As portas de documentos existem desde a Fase 0, com o adaptador SQLite em desenvolvimento e um adaptador nulo em produção; os eventos permanecem na outbox até a Fase 2.
- Acesso por SQL explícito: `pgx/v5` com `sqlc`, um conjunto de queries por dialeto (`postgres` e `sqlite`). Sem ORM.
- Cada porta de repositório tem uma suíte de testes de contrato única, executada contra os dois adaptadores, para garantir comportamento equivalente.
- Migrações com `goose` em `migrations/postgres` e `migrations/sqlite`, executadas por `cmd/migrate`. Recursos exclusivos do PostgreSQL (RLS, triggers de imutabilidade, permissões, particionamento) ficam em migrações só de PostgreSQL.

## Alternativas

- **Só PostgreSQL, com JSONB para os documentos**: mais simples de operar e tecnicamente suficiente na Fase 1. Descartado como desenho final porque a especificação exige MongoDB para os dados de alto volume; a premissa acima adia o custo operacional sem contrariar o requisito.
- **PostgreSQL também em desenvolvimento (via Docker)**: descartado como padrão. Exige Docker na máquina de quem desenvolve; continua disponível pelo `compose` opcional.
- **ORM (GORM, ent)**: descartado pela especificação e porque esconde o SQL do núcleo financeiro.

## Consequências

- Dois conjuntos de migrações e de queries para manter. A suíte de contrato dos repositórios é o que impede divergência.
- No SQLite não há RLS nem papéis de banco: o isolamento por tenant em desenvolvimento depende só da aplicação. Os testes de acesso cruzado rodam obrigatoriamente em PostgreSQL.
- O SQLite não tem `timestamptz` nem `uuid`: instantes são gravados como texto RFC 3339 em UTC e UUIDs como texto.
