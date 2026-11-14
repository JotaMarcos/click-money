# Glossário

Termos de domínio em português (interface, documentação, mensagens) e o identificador em inglês usado no código.

| Português | Inglês (código) | Significado |
| --- | --- | --- |
| Comércio | `tenant` | O estabelecimento que usa o Click-Money. Unidade de isolamento dos dados. |
| Dono | `owner` | Papel com acesso total ao comércio, inclusive ajustes e configurações. |
| Gerente | `manager` | Papel que libera limite, lança com data retroativa além do permitido e resolve revisões. |
| Caixa | `cashier` | Papel que cadastra clientes e lança compras e pagamentos. |
| Membro | `membership` | Vínculo de um usuário com um comércio, com um papel. |
| Cliente | `customer` | Pessoa que compra fiado no comércio. Não tem login. |
| Apelido | `nickname` | Como o cliente é conhecido no balcão; usado na busca. |
| Comunidade ou bairro | `community` | Onde o cliente mora. |
| Ponto de referência | `reference_point` | Indicação para localizar o cliente. |
| Fiado | `credit sale` | Venda a prazo, sem contrato formal. |
| Livro-razão | `ledger` | Registro imutável de todos os lançamentos. |
| Lançamento | `ledger entry` | Uma linha do livro-razão. |
| Compra no fiado | `CREDIT_SALE` | Lançamento que aumenta a dívida. |
| Pagamento | `PAYMENT` | Lançamento que reduz a dívida. |
| Estorno | `REVERSAL` | Lançamento que anula outro, com motivo. |
| Ajuste | `ADJUSTMENT` | Correção de saldo feita só pelo Dono, com motivo. |
| Juros | `INTEREST` | Multa ou juros de mora, quando o comércio habilita. |
| Direção | `direction` | `DEBIT` aumenta a dívida; `CREDIT` reduz. |
| Data do lançamento | `occurred_on` | Data civil, no fuso do comércio, em que a compra ou o pagamento aconteceu. |
| Registrado em | `recorded_at` | Instante em que o servidor gravou o lançamento. |
| Saldo devedor | `balance` | Quanto o cliente deve. Negativo significa crédito a favor do cliente. |
| Saldo projetado | `customer balance` | Saldo mantido em tabela própria, atualizado junto com cada lançamento. |
| Crédito a favor do cliente | `customer credit` | Valor pago além da dívida. |
| Alocação | `payment allocation` | Parte de um pagamento aplicada a uma compra. |
| Compra em aberto | `open sale` | Compra ainda não quitada. Status: em aberto, parcial ou paga. |
| Limite de crédito | `credit limit` | Teto de dívida de um cliente. Modo aviso ou bloqueio. |
| Liberação de limite | `credit limit override` | Autorização pontual de Gerente ou Dono para passar do limite. |
| Dia preferencial de pagamento | `preferred payment day` | Quando o cliente costuma pagar (benefício, quinzena, safra, fim do defeso). |
| Fechamento de caixa | `cash closing` | Total recebido no dia por operador e por meio de pagamento. |
| Meio de pagamento | `payment method` | Dinheiro (`CASH`), Pix (`PIX`), cartão (`CARD`) ou outros (`OTHER`). |
| Carteira | `portfolio` | Conjunto das dívidas a receber do comércio. |
| Aging | `aging` | Carteira por idade da dívida: 0–30, 31–60, 61–90 e mais de 90 dias. |
| Inadimplente | `overdue customer` | Cliente com compra em aberto além do prazo considerado pelo comércio. |
| Cobrança | `collection message` | Mensagem pedindo o pagamento, sempre respeitosa e em horário permitido. |
| Régua de cobrança | `collection schedule` | Sequência de lembretes agendados (Fase 2). |
| Comprovante | `receipt` | Resumo de um lançamento enviado ao cliente. |
| Pix copia e cola | `Pix BR Code` | Código de pagamento Pix no padrão EMV. |
| Consentimento | `consent` | Autorização do cliente para receber comunicações, com data, canal e evidência. |
| Arquivar | `archive` | Tirar o cliente da lista ativa. Só com saldo zero. |
| Anonimizar | `anonymize` | Apagar os dados pessoais a pedido do titular, mantendo os lançamentos. |
| Dispositivo | `device` | Aparelho registrado para usar o comércio. Pode ser revogado. |
| Comando | `command` | Intenção registrada no aparelho e enviada ao servidor na sincronização. |
| Sincronização | `sync` | Envio dos comandos pendentes (`push`) e recebimento das mudanças (`pull`). |
| Fila de revisão | `review queue` | Comandos que precisam de decisão humana. |
| Chave de idempotência | `idempotency key` | Identificador que impede lançar a mesma operação duas vezes. |
| Caixa de saída | `outbox` | Fila de eventos gravada junto com a mudança, para entrega posterior. |
| Assistente | `assistant` | Agente de IA que responde perguntas e prepara ações para confirmação. |
