# ADR 0005 — UUIDv7 gerado no cliente

- Status: Aceita
- Data: 2026-11-13

## Contexto

Clientes e lançamentos são criados em aparelhos sem internet. O registro precisa de um identificador definitivo no momento da criação, para que o comprovante, o extrato local e o reenvio usem o mesmo `id`.

## Decisão

- Entidades criadas offline (`customers`, `ledger_entries`) e todo comando de sync recebem um UUIDv7 gerado no dispositivo.
- O servidor valida que o valor é um UUID versão 7 e que o timestamp embutido não está no futuro além da tolerância de relógio (5 minutos **[PREMISSA]**).
- O servidor nunca troca o `id` enviado. Se o `id` já existe com conteúdo diferente no mesmo tenant, o comando é rejeitado; `id` de outro tenant é indistinguível de inexistente por causa da RLS, e a inserção falha por chave primária sem revelar nada.
- Entidades criadas só no servidor (sessões, eventos de outbox, itens de revisão) usam UUIDv7 gerado pela porta `IDGenerator`, injetável para testes determinísticos.

## Alternativas

- **ID sequencial atribuído pelo servidor**: descartado. Exige troca de identificador após a sincronização e complica a idempotência.
- **UUIDv4**: descartado. Sem ordenação temporal; fragmenta os índices B-tree e não serve de desempate na alocação FIFO.
- **ULID**: equivalente, mas UUIDv7 é padronizado (RFC 9562) e cabe no tipo `uuid` do PostgreSQL.

## Consequências

- A chave primária serve de chave de idempotência natural para criação.
- A ordenação por `id` aproxima a ordem de criação, mas depende do relógio do aparelho. Por isso ela é usada só como desempate; a ordem oficial de recebimento é `recorded_at`, atribuído pelo servidor.
- O relógio do aparelho pode estar errado ou ser manipulado. A validação de tolerância e a auditoria por dispositivo limitam o efeito.
