# ADR 0009 — OpenAPI design-first sobre `net/http`

- Status: Aceita
- Data: 2026-11-13

## Contexto

Backend e PWA evoluem em paralelo e o PWA precisa de tipos confiáveis. A especificação exige `net/http` da biblioteca padrão, sem framework, e OpenAPI 3.1 como fonte da verdade.

## Decisão

- `api/openapi.yaml` (OpenAPI 3.1) é escrito antes do código. Mudança de contrato começa por ele.
- `oapi-codegen` gera os tipos e a interface de servidor no modo `std-http`, que registra rotas no `http.ServeMux` com padrões de método e caminho (Go 1.22+). O código gerado não é versionado; `make generate` o recria e o CI falha se a geração não estiver em dia com o contrato.
- O cliente TypeScript é gerado com `openapi-typescript` (tipos) e usado com `openapi-fetch`.
- Middlewares próprios, na ordem: recuperação de pânico, `request_id`, telemetria, log, cabeçalhos de segurança, CORS, limite de corpo, compressão, rate limit, autenticação, CSRF, tenant.
- Convenções do contrato:
  - prefixo `/v1`; envelope `{ "data": ..., "meta": ... }`;
  - erros em `application/problem+json` (RFC 9457) com `code` estável e mensagens em pt-BR;
  - paginação por cursor opaco (keyset);
  - `Idempotency-Key` obrigatório em POST que cria lançamento;
  - `ETag` e `If-None-Match` nas leituras frequentes;
  - campos opcionais são omitidos, nunca enviados como `null`.
- Um teste de contrato valida requisições e respostas reais contra o documento.

## Alternativas

- **Code-first (anotações gerando o OpenAPI)**: descartado. O contrato vira subproduto do código e o front espera o back.
- **Roteador de terceiros (chi, gin, echo)**: descartado pela especificação; o `ServeMux` atual cobre método e parâmetros de caminho.
- **gRPC ou GraphQL**: descartados. Pior encaixe com cache HTTP, service worker e depuração em campo.

## Consequências

- Front e back trabalham contra o mesmo documento desde o primeiro dia.
- **Risco conhecido:** o suporte do `oapi-codegen` a OpenAPI 3.1 é parcial. A mitigação é manter o contrato em um subconjunto conservador, sem arrays de tipo (`type: [string, "null"]`) e sem recursos exclusivos do JSON Schema 2020-12. A regra "opcional é omitido, nunca nulo" existe também por isso. A geração será validada na Fase 0; se falhar, a alternativa a decidir em novo ADR é gerar a partir de uma versão 3.0 derivada automaticamente do mesmo arquivo.
- Handlers gerados impõem uma assinatura fixa; a lógica fica nos casos de uso, e os handlers só traduzem DTO e erro.
