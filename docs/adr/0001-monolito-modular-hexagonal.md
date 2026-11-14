# ADR 0001 — Monólito modular hexagonal

- Status: Aceita
- Data: 2026-11-13

## Contexto

O produto tem oito contextos de negócio (`identity`, `tenancy`, `customers`, `ledger`, `collections`, `sync`, `reporting`, `assistant`) e um time pequeno. O núcleo financeiro exige transações ACID que atravessam lançamento, saldo, alocação e outbox. A operação precisa ser barata e simples no início, mas as fronteiras devem permitir extrair serviços depois.

## Decisão

Um único repositório e um único módulo Go (`github.com/JotaMarcos/click-money`), com três binários: `cmd/api`, `cmd/worker` e `cmd/migrate`.

Cada contexto vive em `internal/<modulo>` com três camadas:

- `domain`: entidades, value objects, regras e erros. Só importa a biblioteca padrão e `internal/platform/kernel` (tipos compartilhados como `Money`, `CivilDate`, `TenantID`).
- `app`: casos de uso e portas de saída (interfaces definidas por quem consome).
- `adapters`: `http`, `postgres`, `sqlite`, `mongo`.

Regras de dependência:

1. `domain` não importa `app`, `adapters` nem bibliotecas de infraestrutura.
2. Um módulo só enxerga outro pela API pública do `app` dele ou por eventos de domínio na outbox.
3. A injeção de dependências é manual, em `cmd/*/main.go`.
4. As regras 1 e 2 são verificadas no CI por `depguard` (golangci-lint) e por um teste de arquitetura que analisa os imports.

## Alternativas

- **Microserviços desde o início**: descartado. Transações distribuídas no ledger e custo operacional alto sem ganho para o volume esperado.
- **Monólito em camadas sem fronteira de módulo**: descartado. Barato agora, mas torna a extração futura uma reescrita.
- **Múltiplos módulos Go com `go.work`**: adiado. Só compensa quando um módulo precisar de ciclo de versão próprio.

## Consequências

- Uma transação de banco cobre todo o fluxo de escrita do ledger.
- Deploy único; `api` e `worker` escalam separadamente por serem binários distintos.
- A disciplina de fronteira depende do lint e do teste de arquitetura; sem eles ela se perde.
- Chamadas entre módulos são chamadas de função: extrair um serviço exigirá trocar o adaptador da porta, não o caso de uso.
