# Diagramas de sequência

## 1. Lançar fiado offline e sincronizar

```mermaid
sequenceDiagram
    autonumber
    actor Caixa
    participant PWA
    participant IDB as IndexedDB
    participant API as cmd/api (sync)
    participant Ledger as ledger/app
    participant PG as PostgreSQL

    Note over Caixa,IDB: Sem internet
    Caixa->>PWA: Escolhe o cliente, digita o valor, confirma
    PWA->>PWA: Gera id e command_id (UUIDv7),<br/>calcula occurred_on no fuso do comércio
    PWA->>IDB: Grava o lançamento local e enfileira RecordCreditSale
    PWA-->>Caixa: Mostra o saldo provisório e "pendente de envio"

    Note over PWA,PG: A conexão volta
    PWA->>API: POST /v1/sync/push (lote de comandos, gzip)
    API->>API: Autentica, verifica dispositivo e CSRF
    loop Cada comando, em transação própria
        API->>PG: BEGIN, SET LOCAL app.tenant_id
        API->>PG: Procura command_id em sync_commands
        alt Já processado
            PG-->>API: Resultado gravado
            API->>API: Marca como duplicate
        else Novo
            API->>Ledger: RecordCreditSale(comando)
            Ledger->>PG: Carrega saldo, versão e configurações
            Ledger->>Ledger: Valida datas, limite e papel
            Ledger->>PG: INSERT ledger_entries e credit_sale_balances
            Ledger->>PG: UPDATE customer_balances WHERE version = n
            Ledger->>PG: INSERT outbox_events e tenant_changes
            API->>PG: INSERT sync_commands (resultado)
        end
        API->>PG: COMMIT
    end
    API-->>PWA: Resultado por comando:<br/>applied, duplicate, rejected ou needs_review
    PWA->>IDB: Remove da fila o que foi aceito, marca o que foi para revisão
    PWA->>API: GET /v1/sync/pull?cursor=n
    API-->>PWA: Mudanças e saldos projetados, próximo cursor
    PWA->>IDB: Aplica as mudanças e substitui o saldo provisório
    PWA-->>Caixa: "Tudo sincronizado"
```

Se a resposta do push se perder, o PWA reenvia o mesmo lote com backoff e jitter; a consulta a `sync_commands` garante que nada é lançado duas vezes.

## 2. Pagamento com alocação FIFO

```mermaid
sequenceDiagram
    autonumber
    actor Caixa
    participant API as cmd/api
    participant UC as RecordPayment
    participant Conta as CustomerAccount (domínio)
    participant PG as PostgreSQL
    participant Worker as cmd/worker

    Caixa->>API: POST /v1/ledger/payments<br/>Idempotency-Key, valor, meio de pagamento
    API->>UC: Comando validado, com tenant e autor da sessão
    UC->>PG: BEGIN, SET LOCAL app.tenant_id
    UC->>PG: Consulta idempotency_keys
    alt Chave já usada com o mesmo corpo
        PG-->>UC: Resposta gravada
        UC-->>API: Mesma resposta de antes
    else Primeira vez
        UC->>PG: Carrega customer_balances (com version)<br/>e compras em aberto por (occurred_on, id)
        UC->>Conta: Aplicar pagamento
        Conta->>Conta: Verifica excedente conforme a política do comércio
        alt Excede o saldo e o comércio não permite crédito
            Conta-->>UC: Erro PAYMENT_EXCEEDS_BALANCE
            UC->>PG: ROLLBACK
            UC-->>API: 422 Problem Details
        else Aceito
            Conta->>Conta: Aloca da compra mais antiga para a mais nova
            Conta-->>UC: Lançamento, alocações, novo saldo, eventos
            UC->>PG: INSERT ledger_entries (PAYMENT)
            UC->>PG: INSERT payment_allocations
            UC->>PG: UPDATE credit_sale_balances (em aberto e status)
            UC->>PG: UPDATE customer_balances WHERE version = n
            alt Nenhuma linha atualizada (conflito de versão)
                UC->>PG: ROLLBACK
                UC->>UC: Recarrega e tenta de novo (até 3 vezes)
            else Atualizado
                UC->>PG: INSERT outbox_events, tenant_changes, idempotency_keys
                UC->>PG: COMMIT
                UC-->>API: Lançamento e saldo atualizado
            end
        end
    end
    API-->>Caixa: 201 com o lançamento e o novo saldo
    Worker->>PG: Lê a outbox
    Worker->>Worker: Projeta auditoria e read models (Fase 2)
```

## 3. Cobrança via WhatsApp (Fase 1, link `wa.me`)

```mermaid
sequenceDiagram
    autonumber
    actor Dono
    participant PWA
    participant API as cmd/api
    participant Col as collections/app
    participant PG as PostgreSQL
    participant WA as WhatsApp

    Dono->>PWA: Abre o cliente e toca em "Cobrar"
    PWA->>API: POST /v1/customers/{id}/collection-messages
    API->>Col: PrepareCollectionMessage
    Col->>PG: Lê saldo, telefone, consentimento e configurações
    Col->>Col: Calcula a hora local no fuso do comércio
    alt Fora de 8h–20h, domingo ou feriado bloqueado
        Col-->>API: Erro OUTSIDE_COLLECTION_HOURS<br/>com o próximo horário permitido
        API-->>PWA: 422 Problem Details
        PWA-->>Dono: "Você poderá cobrar a partir de amanhã, 8h"
    else Sem consentimento de comunicação
        Col-->>API: Erro CONSENT_REQUIRED
        API-->>PWA: 422 Problem Details
    else Permitido
        Col->>Col: Monta a mensagem com modelo respeitoso,<br/>valor, e código Pix copia e cola se houver chave
        Col->>PG: INSERT outbox_events (CollectionMessagePrepared)
        Col-->>API: Texto e link wa.me
        API-->>PWA: 201 com texto e link
        PWA-->>Dono: Mostra a mensagem para revisão
        Dono->>PWA: Confirma
        PWA->>WA: Abre o link wa.me com o texto preenchido
        Dono->>WA: Envia pelo próprio WhatsApp
    end
```

Na Fase 1 quem envia é o próprio comerciante, pelo aplicativo dele; o sistema registra que a mensagem foi preparada, não que foi entregue. Na Fase 2 o envio passa pela API oficial, com webhook de status.

## 4. Consulta ao assistente (Fase 2)

```mermaid
sequenceDiagram
    autonumber
    actor Dono
    participant PWA
    participant API as cmd/api
    participant Agente as assistant/app (laço do agente)
    participant Tools as Ferramentas (casos de uso)
    participant LLM as LLMProvider
    participant Mongo as MongoDB

    Dono->>PWA: "Quem está me devendo há mais de 30 dias?"
    PWA->>API: POST /v1/assistant/messages
    API->>Agente: Pergunta, com tenant e papel da sessão
    Agente->>Agente: Verifica consentimento de IA e orçamento do tenant
    loop Até o limite de passos
        Agente->>LLM: Instruções, pergunta, esquemas das ferramentas,<br/>dados delimitados e pseudonimizados
        LLM-->>Agente: Pedido de ferramenta ou resposta final
        opt Pedido de ferramenta
            Agente->>Tools: listar_inadimplentes(dias_minimos: 30)
            Tools->>Tools: Autoriza pelo papel do usuário,<br/>no escopo do tenant
            Tools-->>Agente: Resultado tipado
            Agente->>Agente: Pseudonimiza nomes e telefones
        end
    end
    Agente->>Agente: Restaura os nomes reais na resposta
    Agente->>Mongo: Grava o rastro (passos, tokens, custo, latência)
    Agente-->>API: Resposta e, se houver, proposta de ação
    API-->>PWA: Resposta
    PWA-->>Dono: Lista de clientes e botão "Preparar cobrança"

    opt O Dono aceita uma proposta
        Dono->>PWA: Confirma
        PWA->>API: Chamada normal da API (fluxo 3)
        Note over PWA,API: A execução não passa pelo agente
    end
```
