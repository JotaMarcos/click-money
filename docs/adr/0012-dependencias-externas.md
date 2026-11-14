# ADR 0012 — Dependências externas aprovadas

- Status: Aceita
- Data: 2026-11-13

## Contexto

Cada dependência é superfície de ataque, risco de abandono e peso no binário ou no bundle. A especificação exige justificar toda dependência externa.

## Decisão

Só entram as dependências desta lista. Uma dependência nova exige atualizar este ADR (ou criar outro) com a avaliação de manutenção, licença e histórico de vulnerabilidades.

### Backend (Go)

| Dependência | Uso | Por que não a biblioteca padrão |
| --- | --- | --- |
| `github.com/jackc/pgx/v5` | Driver e pool PostgreSQL | `database/sql` não expõe tipos nativos, `COPY` nem pool adequado |
| `modernc.org/sqlite` | SQLite sem CGO | Mantém `CGO_ENABLED=0` |
| `github.com/pressly/goose/v3` | Migrações | Dialetos PostgreSQL e SQLite, uso como biblioteca em `cmd/migrate` |
| `go.mongodb.org/mongo-driver/v2` | MongoDB | Driver oficial |
| `github.com/oapi-codegen/runtime` | Suporte ao código gerado | Exigido pelo gerador |
| `github.com/google/uuid` | UUIDv7 | Implementação mantida da RFC 9562 |
| `golang.org/x/crypto` | Argon2id | Não está na biblioteca padrão |
| `golang.org/x/sync` | `errgroup` | Não está na biblioteca padrão |
| `go.opentelemetry.io/otel` e SDK | Traces e métricas | Padrão de mercado, exigido |
| `github.com/prometheus/client_golang` | Endpoint `/metrics` | Exigido |

### Só em testes e ferramentas

| Dependência | Uso |
| --- | --- |
| `github.com/testcontainers/testcontainers-go` | PostgreSQL e MongoDB reais nos testes de integração |
| `pgregory.net/rapid` | Testes de propriedade (FIFO, convergência do sync) |
| `sqlc`, `oapi-codegen`, `golangci-lint`, `gofumpt`, `govulncheck`, `gosec` | Geração e verificação; não entram no binário |

### Frontend

| Dependência | Uso |
| --- | --- |
| `react`, `react-dom` | Interface |
| `react-router` | Roteamento com URLs reais |
| `@tanstack/react-query` | Cache de dados do servidor |
| `dexie` | IndexedDB |
| `workbox-*` (via `vite-plugin-pwa`) | Service worker |
| `openapi-fetch` | Cliente tipado da API |
| `vite`, `typescript`, `vitest`, `@playwright/test`, `@axe-core/playwright`, `openapi-typescript`, `eslint` | Build e testes; não entram no bundle |

### Feito em casa, de propósito

- Asserções de teste: biblioteca padrão, sem `testify`.
- Roteamento e middlewares HTTP: `net/http`.
- Configuração por variáveis de ambiente: leitor próprio com validação.
- BR Code do Pix (payload EMV e CRC16): poucas dezenas de linhas, testadas com vetores conhecidos.
- Circuit breaker e retry com backoff: implementação pequena em `platform`.
- Formulários, estado global e componentes de UI no front: sem biblioteca.

## Alternativas

- **`shopspring/decimal`**: desnecessário (ADR 0004).
- **`testify`**: conveniência que não justifica a dependência.
- **Biblioteca de componentes (MUI, Chakra)**: estoura o orçamento de 200 KB do bundle.
- **Preact no lugar de React**: fica como plano B se o orçamento do bundle não fechar.

## Consequências

- Um pouco mais de código próprio em troca de menos risco de cadeia de suprimentos.
- `govulncheck`, auditoria do `npm` e SBOM rodam no CI para toda a lista.
- As versões exatas são fixadas em `go.mod` e no lockfile do `npm` na Fase 0.
