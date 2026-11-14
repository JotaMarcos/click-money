# ADR 0008 — Multi-tenant com Row-Level Security

- Status: Aceita
- Data: 2026-11-13

## Contexto

Vários comércios compartilham o mesmo banco. Um vazamento entre tenants expõe a lista de devedores de um comerciante a outro, o que é incidente de LGPD e fim da confiança no produto. Um `WHERE tenant_id = ...` esquecido em uma query não pode ser suficiente para causar isso.

## Decisão

- Banco e esquema compartilhados; toda tabela de negócio tem `tenant_id NOT NULL`.
- **Primeira barreira, na aplicação**: todo caso de uso recebe o `TenantID` do contexto autenticado e toda query filtra por ele. O `tenant_id` nunca vem do corpo da requisição.
- **Segunda barreira, no banco**: Row-Level Security com `FORCE ROW LEVEL SECURITY` e política `tenant_id = current_setting('app.tenant_id')::uuid` para leitura e escrita.
- O Unit of Work abre a transação e executa `SET LOCAL app.tenant_id` antes de qualquer query. Sem a variável definida, `current_setting` falha e a query é negada: o padrão é fechado.
- O usuário de banco da aplicação não é dono das tabelas nem tem `BYPASSRLS`. O usuário de migração é separado.
- Chaves únicas e estrangeiras do núcleo incluem `tenant_id`, para que uma FK nunca aponte para linha de outro tenant.
- Tabelas globais (`users`, `refresh_tokens`) não têm RLS; o acesso a elas é restrito aos casos de uso de `identity`.
- O `worker`, que processa eventos de todos os tenants, lê a outbox com um papel próprio e assume o tenant de cada evento com `SET LOCAL` antes de tocar em dados de negócio.

## Alternativas

- **Um banco ou esquema por tenant**: descartado. Milhares de pequenos comércios tornariam migrações e conexões inviáveis.
- **Só filtro na aplicação**: descartado. Um único esquecimento vaza dados.

## Consequências

- Uma query sem filtro de tenant devolve zero linhas em vez de dados alheios.
- `SET LOCAL` exige que todo acesso ocorra dentro de transação, o que o Unit of Work já impõe. Funciona com pool em modo transação.
- O SQLite não tem RLS: os testes de acesso cruzado entre tenants rodam em PostgreSQL real e são obrigatórios no CI.
- Relatórios agregados entre tenants (Fase 3) precisarão de um papel e de um caminho de leitura próprios, com dados anonimizados.
