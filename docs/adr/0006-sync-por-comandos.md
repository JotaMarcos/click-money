# ADR 0006 — Sincronização offline por comandos

- Status: Aceita
- Data: 2026-11-13

## Contexto

O comerciante opera dias sem internet, às vezes com mais de um aparelho. A conexão, quando existe, é lenta e cai no meio do envio. O servidor não pode confiar em saldo nem em permissão calculados no aparelho.

## Decisão

**O cliente envia comandos, não estados.** A outbox local guarda `CreateCustomer`, `RecordCreditSale`, `RecordPayment` e `ReverseEntry`, cada um com `command_id` (UUIDv7), `device_id`, `client_recorded_at` e, quando se aplica, `base_version`.

**Push.** `POST /v1/sync/push` recebe até 100 comandos **[PREMISSA]**, processados na ordem do lote.

- Cada comando roda em transação própria e passa pelo mesmo caso de uso da API online, com as mesmas regras e a mesma autorização.
- O resultado é gravado em `sync_commands` na mesma transação. `command_id` repetido devolve o resultado gravado, sem reexecutar.
- Resultado por comando: `applied`, `duplicate`, `rejected` (com `code` de negócio) ou `needs_review`.
- Um comando rejeitado não interrompe o lote.

**Pull.** `GET /v1/sync/pull?cursor=` devolve as mudanças do tenant depois do cursor, paginadas e ordenadas, incluindo os saldos projetados.

- O cursor é um número de sequência **por tenant**. A sequência fica em `tenant_sync_states.last_seq` e é incrementada dentro da transação de cada escrita, que também insere uma linha em `tenant_changes`.
- Incrementar um contador por tenant serializa as escritas daquele tenant. Isso garante que nenhum leitor veja a sequência 12 antes de a 11 estar confirmada, o que uma `SEQUENCE` do PostgreSQL não garante.

**Regras específicas do caminho offline.**

- Datas: o aparelho envia `occurred_on` já no fuso do comércio. "Data futura" e o limite de retroatividade são avaliados contra `client_recorded_at`, não contra a hora de chegada ao servidor, para que dias de espera sem sinal não invalidem o lançamento.
- `client_recorded_at` no futuro além de 5 minutos, ou mais antigo que 30 dias **[PREMISSA]**, manda o comando para revisão.
- Pagamento maior que o saldo: se o comércio permite crédito a favor do cliente, aplica; caso contrário, `needs_review`. O dinheiro já foi recebido no balcão, então o servidor não descarta o fato.
- Limite de crédito em modo bloqueio estourado offline: `needs_review`.
- Telefone duplicado em `CreateCustomer`: `needs_review`, com sugestão de unificar.

**Fila de revisão.** `review_items` guarda o comando original, o motivo e a decisão (quem, quando, o quê). Só Gerente e Dono resolvem.

**Dispositivos.** Todo aparelho é registrado e pode ser revogado. Push e pull de aparelho revogado respondem `DEVICE_REVOKED`; o PWA então apaga os dados locais. Os dados sensíveis no IndexedDB são cifrados com AES-GCM (Web Crypto) e chave derivada de segredo da sessão.

## Alternativas

- **Sincronizar estado (última escrita vence)**: descartado. Perde lançamentos em conflito, o que é inaceitável em um livro-razão.
- **CRDTs**: descartado. O ledger append-only já é comutativo para saldo; os conflitos restantes pedem decisão humana, não fusão automática.
- **Transação única para o lote inteiro**: descartado. Um comando inválido derrubaria os demais e um lote grande seguraria locks por muito tempo em conexão lenta.
- **Cursor por `recorded_at`**: descartado. Timestamps iguais e transações que confirmam fora de ordem fazem o cliente perder mudanças.

## Consequências

- Reenviar o mesmo lote é sempre seguro, o que simplifica o retry com backoff e jitter no cliente.
- O contador por tenant limita a vazão de escrita de um tenant a uma transação por vez. É irrelevante para um comércio com poucos caixas e será medido no teste de carga com 500 tenants.
- `sync_commands` cresce continuamente; resultados com mais de 90 dias **[PREMISSA]** são expurgados, e um `command_id` mais antigo que isso é tratado pela validação de `client_recorded_at`.
- A fila de revisão precisa de tela própria e de aviso visível no PWA, senão itens ficam esquecidos.
