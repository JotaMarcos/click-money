# ADR 0007 — Outbox transacional

- Status: Aceita
- Data: 2026-11-13

## Contexto

Depois de um lançamento, o sistema precisa atualizar read models, registrar auditoria detalhada e, mais adiante, disparar notificações. Fazer isso dentro da requisição acopla o núcleo a sistemas que podem estar fora do ar; publicar depois do `COMMIT` sem garantia perde eventos.

## Decisão

- Todo caso de uso que altera estado grava os eventos de domínio em `outbox_events` na mesma transação da mudança.
- O `cmd/worker` consome a tabela em lotes com `SELECT ... FOR UPDATE SKIP LOCKED`, entrega a cada consumidor e marca `published_at`. No SQLite, que tem um único escritor, o `SKIP LOCKED` é dispensado.
- A entrega é "ao menos uma vez". Todo consumidor é idempotente, usando o `id` do evento como chave.
- Falha de entrega incrementa `attempts` e agenda nova tentativa com backoff exponencial e jitter. Após 10 tentativas **[PREMISSA]** o evento vai para estado de falha permanente e dispara alerta.
- Eventos do mesmo agregado são entregues em ordem (ordenação por `id` UUIDv7 dentro de `aggregate_id`).
- Eventos publicados são expurgados após 7 dias **[PREMISSA]**.
- O payload do evento não carrega PII além do necessário; consumidores buscam detalhes pelo `id` quando precisam.

## Alternativas

- **Broker (Kafka, NATS, RabbitMQ)**: descartado por ora. Mais um componente para operar, sem resolver a escrita dupla entre banco e broker. A outbox pode alimentar um broker no futuro.
- **`LISTEN/NOTIFY` do PostgreSQL**: útil só como gatilho para reduzir a latência do polling; não substitui a tabela porque a notificação se perde se ninguém estiver ouvindo.
- **CDC (Debezium)**: descartado pela complexidade operacional.

## Consequências

- MongoDB fora do ar não afeta a escrita: os eventos acumulam e são projetados depois.
- O tamanho e a idade da fila são métricas de primeira classe, com painel e alerta.
- Os read models são eventualmente consistentes; telas que exigem valor exato (saldo, extrato) leem do PostgreSQL.
