# Estrutura de diretórios

Módulo Go: `github.com/JotaMarcos/click-money`.

```
click-money/
├── api/
│   └── openapi.yaml              # contrato, fonte da verdade
├── cmd/
│   ├── api/                      # servidor HTTP (composition root)
│   ├── worker/                   # outbox, lembretes, projeções
│   └── migrate/                  # migrações e reconciliação
├── internal/
│   ├── identity/                 # usuários, credenciais, sessões
│   ├── tenancy/                  # comércio, membros, papéis, configurações
│   ├── customers/                # clientes, consentimentos, limite
│   ├── ledger/                   # lançamentos, FIFO, saldo, extrato
│   ├── collections/              # cobranças, comprovantes, Pix
│   ├── sync/                     # push, pull, dispositivos, revisão
│   ├── reporting/                # indicadores, aging, exportações
│   ├── assistant/                # agente de IA (Fase 2)
│   └── platform/
│       ├── kernel/               # Money, CivilDate, Phone, TenantID, erros base
│       ├── clock/                # porta Clock e implementações
│       ├── idgen/                # porta IDGenerator (UUIDv7)
│       ├── config/               # variáveis de ambiente validadas
│       ├── logging/              # slog, redação de PII
│       ├── telemetry/            # OpenTelemetry, Prometheus, pprof
│       ├── httpx/                # servidor, middlewares, Problem Details
│       ├── auth/                 # sessão, CSRF, contexto autenticado
│       ├── db/                   # pool, Unit of Work, RLS (postgres e sqlite)
│       ├── docstore/             # porta de documentos (mongo, sqlite, nulo)
│       ├── outbox/               # escrita e consumo da outbox
│       ├── resilience/           # retry, backoff, circuit breaker
│       └── i18n/                 # mensagens pt-BR
├── migrations/
│   ├── postgres/
│   └── sqlite/
├── web/                          # PWA (React, Vite, TypeScript)
│   └── src/
│       ├── api/                  # cliente gerado do OpenAPI
│       ├── db/                   # Dexie, cifragem, outbox local
│       ├── sync/                 # sincronizador
│       ├── design-system/        # tokens e componentes
│       ├── features/             # telas por funcionalidade
│       ├── i18n/                 # mensagens
│       └── lib/                  # dinheiro, datas, telefone
├── deploy/
│   ├── Dockerfile
│   ├── compose.yaml              # PostgreSQL, MongoDB e observabilidade (opcional)
│   ├── tofu/                     # infraestrutura como código
│   └── dashboards/
├── docs/
│   ├── adr/
│   ├── architecture/
│   ├── backlog/
│   ├── db/
│   ├── runbooks/
│   ├── security/
│   └── glossario.md
├── tests/
│   ├── contract/                 # respostas reais contra o OpenAPI
│   ├── e2e/                      # Playwright
│   └── load/                     # k6
├── .github/workflows/
├── Makefile
├── go.mod
└── README.md
```

## Dentro de cada módulo de negócio

```
internal/<modulo>/
├── domain/        # entidades, value objects, regras, erros (sem infraestrutura)
├── app/           # casos de uso e portas de saída
└── adapters/
    ├── http/      # handlers, DTOs, mapeamento de erros
    ├── postgres/  # repositórios (pgx + sqlc)
    ├── sqlite/    # repositórios de desenvolvimento
    └── mongo/     # repositórios de documentos (quando houver)
```

## Regras de dependência

| De | Pode importar |
| --- | --- |
| `<modulo>/domain` | biblioteca padrão e `platform/kernel` |
| `<modulo>/app` | o próprio `domain`, `platform/kernel`, `platform/clock`, `platform/idgen` |
| `<modulo>/adapters/*` | o próprio `app` e `domain`, e `platform` |
| `<modulo>` para outro módulo | só o pacote `app` do outro, por interface declarada em quem consome |
| `cmd/*` | tudo; é onde as dependências são montadas |

As regras são verificadas no CI por `depguard` e por um teste de arquitetura (ADR 0001).

## O que não é versionado

Código gerado (`oapi-codegen`, `sqlc`, cliente TypeScript), `dist/`, `bin/`, `node_modules/`, bancos SQLite locais, `.env` e relatórios de cobertura. `make generate` recria o código gerado.
