# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

A especificação oficial do produto é o prompt-mestre abaixo; ela prevalece sobre qualquer preferência. As decisões já tomadas estão em `docs/adr/` e o que falta fazer em `docs/backlog/`.

@.claude/prompt-mestre.md

## Situação

- Etapa 2 (arquitetura) concluída: ADRs, diagramas, contrato `api/openapi.yaml`, backlog das Fases 0 e 1.
- Fase 0 em andamento: existe `internal/platform` (tipos compartilhados, config, logs, HTTP, banco, migrações, outbox) e os três binários. Ainda **não** existem módulos de negócio (`ledger`, `customers` etc.), PWA (`web/`), geração de código (`oapi-codegen`, `sqlc`), OpenTelemetry, Dockerfile, CI nem testes de integração com PostgreSQL. Ver `docs/backlog/fase-0.md`.

## Comandos

O `Makefile` é a referência, mas `make` não está instalado na máquina Windows de desenvolvimento; os equivalentes diretos são:

```sh
go run ./cmd/migrate up        # aplica migrações (SQLite em click_money.db por padrão)
go run ./cmd/migrate status    # versão do banco; "down" reverte a última
go run ./cmd/api               # API em :8080 (exige migrações aplicadas)
go run ./cmd/worker

go test ./...                                              # tudo
go test ./internal/platform/outbox -run TestOrderIsPreservedPerAggregate   # um teste
go test ./internal/platform/kernel -run '^$' -fuzz FuzzParseBRL -fuzztime 20s
go test -coverpkg=./internal/... -coverprofile=coverage.out ./... && go tool cover -func=coverage.out

gofmt -l . && go vet ./...     # deve sair vazio antes de commitar
CGO_ENABLED=0 go build -o bin/ ./cmd/...
```

- `go test -race` falha nesta máquina com `exit status 0xc0000139` (o MinGW instalado é antigo demais para o runtime do detector). Rode sem `-race` aqui; o `-race` é obrigação do CI em Linux.
- `golangci-lint`, `govulncheck` e `gosec` ainda não estão instalados nem configurados.
- Configuração por variáveis `CM_*` (ver `.env.example`). O `.env` só é lido quando `CM_ENV` é `development`. Fora de desenvolvimento, `config.Load` exige PostgreSQL e origens `https`.

## Arquitetura

Monólito modular hexagonal (ADR 0001). Módulo Go único: `github.com/JotaMarcos/click-money`.

**Camadas de um módulo de negócio** (`internal/<modulo>/{domain,app,adapters}`): `domain` só importa a biblioteca padrão e `platform/kernel`; `app` define casos de uso e as portas de saída; um módulo só fala com outro pelo `app` dele ou por eventos na outbox. A montagem é manual em `cmd/*/main.go`.

**Como uma escrita deve fluir** (o padrão que todo caso de uso segue):

1. O caso de uso abre a transação com `db.WithinTenant(ctx, tenantID, fn)`. No PostgreSQL isso define `app.tenant_id`, de que dependem as políticas de RLS; sem ele o banco nega a query.
2. Repositórios não recebem a transação por parâmetro: pegam com `db.Tx(ctx)`. Chamar fora de um Unit of Work devolve `db.ErrNoTransaction`; aninhar devolve `db.ErrNestedTransaction`.
3. Eventos de domínio são gravados com `outbox.Writer.Append(ctx, ...)` **na mesma transação**.
4. O `cmd/worker` entrega depois, via `outbox.Relay`, com entrega "ao menos uma vez" e ordem preservada por agregado. Hoje o worker só reporta a fila: não há consumidores, e publicar sem consumidor perderia os eventos.

**Dois dialetos SQL, um código.** Produção usa PostgreSQL (driver pgx via `database/sql`); desenvolvimento usa SQLite (`modernc.org/sqlite`, sem CGO). Consequências práticas:

- Escreva queries com `?` e passe por `dialect.Rebind` (vira `$1...` no PostgreSQL).
- Instantes: passe como argumento com `dialect.TimeArg(t)` e leia com `db.Time`. No SQLite eles são texto UTC de largura fixa, para que a ordem do texto seja a ordem do tempo.
- UUIDs trafegam como texto; converta com `kernel.ParseID`.
- Toda migração existe em `migrations/postgres` **e** `migrations/sqlite`, é reversível e é embutida no binário. RLS, papéis e permissões só existem na de PostgreSQL. A API nunca migra ao subir.
- Em teste, `dbtest.NewSQLite(t)` devolve um banco em memória já migrado.

**Regras que o compilador não impõe:**

- Dinheiro é `kernel.Money` (centavos em `int64`). Nada de `float` em cálculo financeiro. Entrada só por `kernel.ParseBRL`, que recusa formato ambíguo em vez de corrigir.
- Não chame `time.Now()` em código de negócio: use `clock.Clock` injetado. "Hoje" é `clock.Today(c, fusoDoComercio)`, nunca a data em UTC.
- IDs novos vêm de `idgen.Generator` (UUIDv7); entidades criadas offline chegam com o ID gerado no cliente.
- Erros de negócio são `*kernel.Error` com `Code` estável igual a um `ProblemCode` do OpenAPI. `httpx.WriteError` traduz o código em status (`statusByCode`; o que não está lá vira 422) e nunca expõe erro interno ao cliente. Código novo com status diferente de 422 precisa entrar nesse mapa.
- Logs passam por redação baseada no **nome** do atributo (`logging.redact`): registre telefone como `phone`/`telefone`, segredos como `token`/`password` etc. Um dado pessoal sob nome neutro vaza.
- Mudança de contrato começa por `api/openapi.yaml`. Campos opcionais são omitidos, nunca `null` (ADR 0009).

## Convenções do repositório

- Domínio, mensagens, documentação e comentários em pt-BR; identificadores em inglês (`docs/glossario.md`).
- Conventional Commits, sem `Co-Authored-By` nem qualquer menção a ferramentas ou autoria em commits, código ou docs. Todos os commits saem com a identidade Git local do repositório.
- Suposições novas são marcadas com **[PREMISSA]**; dependência externa nova exige atualizar o ADR 0012.
- Os papéis do time em `.claude/agents/` descrevem as regras de cada especialidade (DBA, QA, segurança etc.).
