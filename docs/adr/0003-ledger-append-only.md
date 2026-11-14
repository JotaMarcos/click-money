# ADR 0003 — Ledger append-only, saldo projetado e alocação FIFO

- Status: Aceita
- Data: 2026-11-13

## Contexto

O histórico de fiado precisa ser auditável e resolver disputas ("eu já paguei"). Listagens mostram o saldo de muitos clientes e não podem varrer o histórico. O aging da carteira depende de saber quanto de cada compra ainda está em aberto.

## Decisão

**Lançamentos imutáveis.** A tabela `ledger_entries` só aceita `INSERT`. A correção é um lançamento `REVERSAL` que referencia o original, com motivo e autor. A imutabilidade é garantida em três níveis: o domínio não expõe mutação, o usuário de banco da aplicação não tem `UPDATE`/`DELETE` na tabela e um trigger rejeita as duas operações.

**Valor sempre positivo, com direção explícita.** `amount_cents > 0` por CHECK. A coluna `direction` diz o efeito sobre a dívida:

| Tipo | Direção |
| --- | --- |
| `CREDIT_SALE`, `INTEREST` | `DEBIT` (aumenta a dívida) |
| `PAYMENT` | `CREDIT` (reduz a dívida) |
| `REVERSAL` | oposta à do lançamento estornado |
| `ADJUSTMENT` | escolhida pelo Dono |

Um CHECK amarra tipo e direção. Um índice único em `reverses_entry_id` impede estornar duas vezes o mesmo lançamento; estorno de estorno é proibido.

**Projeções na mesma transação.** Cada lançamento atualiza, na mesma transação:

- `customer_balances`: saldo (`balance_cents`, com sinal; negativo é crédito a favor do cliente), número de compras em aberto, data da compra em aberto mais antiga e `version`.
- `credit_sale_balances`: uma linha por compra, com valor original, valor em aberto e status (`OPEN`, `PARTIAL`, `PAID`, `REVERSED`).
- `payment_allocations`: quanto de cada pagamento foi aplicado a cada compra. Também é somente-inserção: desfazer uma alocação insere uma linha compensatória.

**Concorrência otimista.** A atualização de `customer_balances` usa `WHERE version = $n`. Em conflito, o caso de uso recarrega e tenta de novo, até três vezes.

**Alocação FIFO.** Um pagamento é aplicado às compras em aberto na ordem `(occurred_on, id)`. Como o `id` é UUIDv7, o desempate segue a ordem de criação no dispositivo.

**Estornos e alocação.**

- Estornar um pagamento libera as alocações dele; as compras voltam a ficar em aberto.
- Estornar uma compra que já recebeu pagamentos libera essas alocações e reaplica os valores às compras seguintes pela mesma regra FIFO. O que sobrar vira saldo a favor do cliente.

**Excedente de pagamento.** Sem compra em aberto para receber, o excedente fica como saldo negativo. Se o comércio não permite crédito a favor do cliente, o caso de uso rejeita com `PAYMENT_EXCEEDS_BALANCE` antes de gravar (online) ou envia para a fila de revisão (sync).

## Alternativas

- **Valor com sinal em vez de direção**: descartado. Contraria o CHECK `amount_cents > 0` da especificação e facilita erro de sinal.
- **Saldo calculado por `SUM` sob demanda**: descartado. Custo cresce com o histórico e a especificação proíbe em listagens.
- **Projeções assíncronas via outbox**: descartado para saldo e alocação. O comerciante precisa ver o saldo correto logo após lançar.
- **Lock pessimista (`SELECT ... FOR UPDATE`) no saldo**: funciona, mas a especificação pede controle otimista, que também se porta igual no SQLite.

## Consequências

- O saldo de qualquer cliente é uma leitura por chave primária.
- As projeções podem ser reconstruídas a partir de `ledger_entries`. Um comando de reconciliação em `cmd/migrate` compara projeção e histórico e será usado em teste e em auditoria.
- A alocação FIFO depende da ordem em que os lançamentos chegam ao servidor. Ordens diferentes de sincronização produzem o mesmo **saldo**, mas podem produzir alocações diferentes quando um pagamento chega antes de uma compra mais antiga. O teste de propriedade exige convergência de saldo; a convergência de alocação fica registrada como débito técnico a avaliar na Fase 1 (realocação determinística na chegada de lançamento retroativo).
- A escrita toca até quatro tabelas por lançamento. É aceitável para o volume de um pequeno comércio e será medido no teste de carga.
