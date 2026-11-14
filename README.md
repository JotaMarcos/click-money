# Click-Money

Plataforma de gestão de fiado offline-first para comércios de Manaus e do interior do Amazonas. Especificação em [.claude/prompt-mestre.md](.claude/prompt-mestre.md).

## Situação

- Arquitetura (Etapa 2): concluída.
- Fundação (Fase 0): em andamento. Já existem os tipos compartilhados, configuração, logs, servidor HTTP, banco com Unit of Work, migrações, outbox e os três binários. Faltam geração de código a partir do OpenAPI, OpenTelemetry, testes de integração com PostgreSQL, empacotamento, CI e o esqueleto do PWA. Detalhes em [docs/backlog/fase-0.md](docs/backlog/fase-0.md).

## Como executar

Requer Go 1.27 ou mais recente. O banco local é SQLite, sem nenhuma outra dependência.

```sh
go run ./cmd/migrate up     # cria click_money.db
go run ./cmd/api            # API em http://localhost:8080
```

Verifique com `curl http://localhost:8080/readyz`. A configuração é feita por variáveis `CM_*`; veja [.env.example](.env.example).

## Como testar

```sh
go test ./...
gofmt -l . && go vet ./...
```

O [Makefile](Makefile) reúne as mesmas tarefas (`make dev`, `make test`, `make cover`, `make fuzz`, `make build`) para quem tem `make` instalado.

## Documentação

| Documento | Conteúdo |
| --- | --- |
| [docs/adr/](docs/adr/README.md) | Decisões de arquitetura |
| [docs/architecture/c4.md](docs/architecture/c4.md) | Diagramas C4: contexto, contêineres e componentes do `ledger` |
| [docs/architecture/er.md](docs/architecture/er.md) | Modelo de dados, constraints e índices |
| [docs/architecture/sequencias.md](docs/architecture/sequencias.md) | Fiado offline e sync, pagamento FIFO, cobrança, assistente |
| [docs/architecture/estrutura-de-diretorios.md](docs/architecture/estrutura-de-diretorios.md) | Organização do repositório e regras de dependência |
| [docs/backlog/](docs/backlog/) | Histórias e critérios de aceite por fase |
| [docs/glossario.md](docs/glossario.md) | Termos de domínio em português e inglês |
| [api/openapi.yaml](api/openapi.yaml) | Contrato da API (OpenAPI 3.1) |
