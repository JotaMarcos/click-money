# Tarefas de desenvolvimento do Click-Money.
# O padrão local é SQLite, sem dependências além de Go.

GO        ?= go
BIN_DIR   := bin
PKGS      := ./...
# Pacotes cuja cobertura conta para o piso geral (binários e utilitários de
# teste ficam de fora).
COVER_PKG := ./internal/...

.PHONY: dev worker test test-race test-integration cover fuzz lint fmt generate migrate-up migrate-down migrate-status build e2e tidy clean

dev: migrate-up ## sobe a API em localhost:8080 com SQLite
	$(GO) run ./cmd/api

worker: ## sobe o worker contra o mesmo banco local
	$(GO) run ./cmd/worker

test: ## testes unitários
	$(GO) test $(PKGS)

test-race: ## testes com detector de corrida (exige CGO e um compilador C recente)
	$(GO) test -race $(PKGS)

test-integration: ## testes contra PostgreSQL e MongoDB reais (exige Docker)
	$(GO) test -tags=integration $(PKGS)

cover: ## cobertura por pacote e total
	$(GO) test -coverpkg=$(COVER_PKG) -coverprofile=coverage.out $(PKGS)
	$(GO) tool cover -func=coverage.out | tail -n 1

fuzz: ## fuzzing curto dos parsers
	$(GO) test ./internal/platform/kernel -run '^$$' -fuzz FuzzParseBRL -fuzztime 20s
	$(GO) test ./internal/platform/kernel -run '^$$' -fuzz FuzzParsePhone -fuzztime 20s

lint: ## formatação, vet e golangci-lint
	@test -z "$$(gofmt -l .)" || (echo "arquivos fora do gofmt:"; gofmt -l .; exit 1)
	$(GO) vet $(PKGS)
	golangci-lint run

fmt: ## formata o código
	gofmt -w .

generate: ## código gerado a partir do OpenAPI e das queries (ainda não há geradores configurados)
	$(GO) generate $(PKGS)

migrate-up: ## aplica as migrações pendentes
	$(GO) run ./cmd/migrate up

migrate-down: ## reverte a última migração
	$(GO) run ./cmd/migrate down

migrate-status: ## mostra a versão do banco
	$(GO) run ./cmd/migrate status

build: ## binários estáticos em bin/
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags="-s -w" -o $(BIN_DIR)/ ./cmd/...

e2e: ## testes de ponta a ponta do PWA (ainda não há PWA)
	@echo "e2e: o PWA ainda não existe; ver docs/backlog/fase-0.md (F0-10)"; exit 1

tidy:
	$(GO) mod tidy

clean:
	rm -rf $(BIN_DIR) coverage.out
