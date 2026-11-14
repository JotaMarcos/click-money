# Backlog — Fase 1 (MVP vendável)

Objetivo: o comerciante larga o caderno. Ele cadastra clientes, lança fiado e pagamentos sem internet, vê quanto tem a receber, cobra pelo WhatsApp e recebe por Pix.

Estimativa em pontos relativos. A ordem de implementação é: `ledger` (domínio e testes primeiro), `customers`, `identity` e `tenancy`, `sync`, PWA, `collections`, `reporting`, LGPD.

## Personas

- **Seu Raimundo**, dono de taberna em comunidade ribeirinha. Fica dias sem sinal, vende para pescadores que pagam depois do defeso. Usa o celular do filho.
- **Dona Socorro**, dona de mercadinho em bairro de Manaus. Tem 4G instável, dois caixas e cerca de 200 clientes no fiado.
- **Seu Chico**, regatão. Vende de barco, de comunidade em comunidade, e volta à cidade a cada duas ou três semanas.
- **Jéssica**, caixa da Dona Socorro. Pouca familiaridade com aplicativos; precisa lançar rápido com fila no balcão.

## Ledger

### F1-01 — Lançar compra no fiado (5 pontos)

Como caixa, quero lançar uma compra no fiado em poucos toques, para não atrasar a fila.

```gherkin
Funcionalidade: Compra no fiado

  Contexto:
    Dado o comércio "Mercadinho da Socorro" no fuso "America/Manaus"
    E o cliente "Zé da Balsa" com saldo devedor de R$ 0,00

  Cenário: Compra simples
    Quando a caixa lança uma compra de R$ 25,50 para "Zé da Balsa"
    Então o saldo devedor de "Zé da Balsa" é R$ 25,50
    E o cliente tem 1 compra em aberto
    E o extrato mostra 1 lançamento do tipo "compra no fiado"

  Cenário: Compra às 21h fica no dia local
    Dado que agora são 21:00 de 13/11/2026 em Manaus
    Quando a caixa lança uma compra de R$ 10,00 para "Zé da Balsa"
    Então a data do lançamento é 13/11/2026

  Cenário: Valor zero
    Quando a caixa lança uma compra de R$ 0,00 para "Zé da Balsa"
    Então o lançamento é recusado com o código "VALIDATION_FAILED"
    E o saldo devedor continua R$ 0,00

  Cenário: Data futura
    Dado que hoje é 13/11/2026 em Manaus
    Quando a caixa lança uma compra com data 14/11/2026
    Então o lançamento é recusado com o código "FUTURE_DATE_NOT_ALLOWED"

  Cenário: Data retroativa dentro do limite
    Dado que o comércio permite 7 dias de retroatividade
    E que hoje é 13/11/2026 em Manaus
    Quando a caixa lança uma compra com data 08/11/2026
    Então o lançamento é aceito

  Cenário: Data retroativa além do limite exige gerente
    Dado que o comércio permite 7 dias de retroatividade
    E que hoje é 13/11/2026 em Manaus
    Quando a caixa lança uma compra com data 01/11/2026
    Então o lançamento é recusado com o código "BACKDATE_REQUIRES_MANAGER"

  Cenário: Gerente lança com data além do limite
    Dado que o comércio permite 7 dias de retroatividade
    E que hoje é 13/11/2026 em Manaus
    Quando a gerente lança uma compra com data 01/11/2026
    Então o lançamento é aceito

  Cenário: Cliente arquivado
    Dado que "Zé da Balsa" está arquivado
    Quando a caixa lança uma compra de R$ 10,00 para "Zé da Balsa"
    Então o lançamento é recusado com o código "CUSTOMER_ARCHIVED"

  Cenário: Mesma operação enviada duas vezes
    Quando a caixa lança uma compra de R$ 25,50 com a chave de idempotência "abc"
    E a mesma requisição é enviada de novo com a chave "abc"
    Então existe 1 lançamento
    E o saldo devedor de "Zé da Balsa" é R$ 25,50

  Cenário: Chave reutilizada com outro valor
    Quando a caixa lança uma compra de R$ 25,50 com a chave de idempotência "abc"
    E lança uma compra de R$ 30,00 com a chave "abc"
    Então a segunda requisição é recusada com o código "IDEMPOTENCY_KEY_REUSED"
```

### F1-02 — Limite de crédito (3 pontos)

Como dona, quero definir um limite por cliente, para controlar meu risco.

```gherkin
Funcionalidade: Limite de crédito

  Contexto:
    Dado o cliente "Maria do Açaí" com limite de R$ 100,00
    E saldo devedor de R$ 90,00

  Cenário: Modo aviso
    Dado que o comércio usa o limite em modo "aviso"
    Quando a caixa lança uma compra de R$ 20,00
    Então o lançamento é aceito
    E a resposta traz o aviso "CREDIT_LIMIT_EXCEEDED"
    E o saldo devedor é R$ 110,00

  Cenário: Modo bloqueio
    Dado que o comércio usa o limite em modo "bloqueio"
    Quando a caixa lança uma compra de R$ 20,00
    Então o lançamento é recusado com o código "CREDIT_LIMIT_EXCEEDED"
    E o saldo devedor continua R$ 90,00

  Cenário: Compra até o limite exato
    Dado que o comércio usa o limite em modo "bloqueio"
    Quando a caixa lança uma compra de R$ 10,00
    Então o lançamento é aceito

  Cenário: Liberação pontual pela gerente
    Dado que o comércio usa o limite em modo "bloqueio"
    Quando a gerente lança uma compra de R$ 20,00 com o motivo "cliente antiga, paga na sexta"
    Então o lançamento é aceito
    E a liberação fica registrada com autora e motivo

  Cenário: Caixa tenta liberar
    Dado que o comércio usa o limite em modo "bloqueio"
    Quando a caixa lança uma compra de R$ 20,00 informando um motivo de liberação
    Então o lançamento é recusado com o código "FORBIDDEN"

  Cenário: Cliente sem limite definido
    Dado o cliente "João Pescador" sem limite
    Quando a caixa lança uma compra de R$ 500,00
    Então o lançamento é aceito sem aviso
```

### F1-03 — Receber pagamento com alocação FIFO (8 pontos)

Como caixa, quero registrar um pagamento e ver a dívida diminuir a partir da compra mais antiga, para que o cliente saiba o que já quitou.

```gherkin
Funcionalidade: Pagamento com alocação às compras mais antigas

  Contexto:
    Dado o cliente "Zé da Balsa" com as compras em aberto:
      | data       | valor     |
      | 01/11/2026 | R$ 30,00  |
      | 05/11/2026 | R$ 50,00  |
      | 10/11/2026 | R$ 20,00  |

  Cenário: Pagamento quita a compra mais antiga
    Quando a caixa registra um pagamento de R$ 30,00 em dinheiro
    Então o saldo devedor é R$ 70,00
    E a compra de 01/11/2026 está "paga"
    E a compra de 05/11/2026 está "em aberto"
    E a data da compra em aberto mais antiga é 05/11/2026

  Cenário: Pagamento parcial atravessa duas compras
    Quando a caixa registra um pagamento de R$ 45,00 por Pix
    Então o saldo devedor é R$ 55,00
    E a compra de 01/11/2026 está "paga"
    E a compra de 05/11/2026 está "parcial" com R$ 35,00 em aberto
    E a compra de 10/11/2026 está "em aberto"

  Cenário: Pagamento quita tudo
    Quando a caixa registra um pagamento de R$ 100,00 no cartão
    Então o saldo devedor é R$ 0,00
    E o cliente não tem compras em aberto

  Cenário: Duas compras no mesmo dia
    Dado que "Zé da Balsa" tem só duas compras em aberto, ambas de 12/11/2026, de R$ 10,00 e depois de R$ 15,00
    Quando a caixa registra um pagamento de R$ 10,00
    Então a compra lançada primeiro está "paga"
    E a compra lançada depois está "em aberto"

  Cenário: Pagamento maior que a dívida, comércio rejeita
    Dado que o comércio rejeita pagamento acima da dívida
    Quando a caixa registra um pagamento de R$ 120,00
    Então o pagamento é recusado com o código "PAYMENT_EXCEEDS_BALANCE"
    E o saldo devedor continua R$ 100,00

  Cenário: Pagamento maior que a dívida, comércio aceita crédito
    Dado que o comércio aceita crédito a favor do cliente
    Quando a caixa registra um pagamento de R$ 120,00
    Então o saldo é de R$ 20,00 a favor do cliente
    E o cliente não tem compras em aberto

  Cenário: Compra consome o crédito a favor do cliente
    Dado que "Zé da Balsa" tem R$ 20,00 a seu favor e nenhuma compra em aberto
    Quando a caixa lança uma compra de R$ 50,00
    Então o saldo devedor é R$ 30,00
    E a nova compra está "parcial" com R$ 30,00 em aberto

  Cenário: Meio de pagamento "outros" exige descrição
    Quando a caixa registra um pagamento de R$ 30,00 em "outros" sem descrição
    Então o pagamento é recusado com o código "VALIDATION_FAILED"

  Cenário: Dois caixas lançam ao mesmo tempo
    Quando dois pagamentos de R$ 30,00 chegam ao mesmo tempo para "Zé da Balsa"
    Então os dois são registrados
    E o saldo devedor é R$ 40,00
```

Propriedades verificadas por teste gerativo:

- A soma das alocações de um pagamento nunca passa do valor dele.
- O valor em aberto de uma compra nunca fica negativo nem passa do original.
- O saldo é sempre igual à soma dos débitos menos a soma dos créditos do histórico.
- Uma compra só recebe alocação se nenhuma compra mais antiga estiver em aberto.

### F1-04 — Estornar lançamento (5 pontos)

Como gerente, quero corrigir um lançamento errado sem apagá-lo, para manter o histórico íntegro.

```gherkin
Funcionalidade: Estorno

  Cenário: Estorno de compra sem pagamentos
    Dado uma compra de R$ 40,00 de "Zé da Balsa", lançada hoje, e saldo devedor de R$ 40,00
    Quando a gerente estorna a compra com o motivo "valor digitado errado"
    Então o saldo devedor é R$ 0,00
    E o extrato mostra a compra original e o estorno
    E a compra original continua com valor R$ 40,00

  Cenário: Estorno de pagamento reabre as compras
    Dado uma compra de R$ 40,00 quitada por um pagamento de R$ 40,00
    Quando a gerente estorna o pagamento com o motivo "Pix não caiu"
    Então o saldo devedor é R$ 40,00
    E a compra está "em aberto"

  Cenário: Estorno de compra que já recebeu pagamento
    Dado as compras de R$ 30,00 em 01/11/2026 e de R$ 50,00 em 05/11/2026
    E um pagamento de R$ 30,00 alocado à compra de 01/11/2026
    Quando a gerente estorna a compra de 01/11/2026
    Então o saldo devedor é R$ 20,00
    E a compra de 05/11/2026 está "parcial" com R$ 20,00 em aberto

  Cenário: Estorno sem motivo
    Quando a gerente estorna um lançamento sem informar o motivo
    Então o estorno é recusado com o código "VALIDATION_FAILED"

  Cenário: Estorno em duplicidade
    Dado um lançamento já estornado
    Quando a gerente tenta estorná-lo de novo
    Então o estorno é recusado com o código "ENTRY_ALREADY_REVERSED"

  Cenário: Estorno de um estorno
    Quando a gerente tenta estornar um lançamento do tipo "estorno"
    Então o estorno é recusado com o código "ENTRY_NOT_REVERSIBLE"

  Cenário: Caixa estorna lançamento próprio do dia
    Dado uma compra lançada hoje pela própria caixa
    Quando a caixa estorna a compra com o motivo "cliente desistiu"
    Então o estorno é aceito

  Cenário: Caixa tenta estornar lançamento de outro dia
    Dado uma compra lançada ontem
    Quando a caixa tenta estornar a compra
    Então o estorno é recusado com o código "FORBIDDEN"

  Cenário: O banco recusa alteração direta
    Quando alguém tenta alterar ou apagar um lançamento diretamente no banco com o usuário da aplicação
    Então o banco rejeita a operação
```

**[PREMISSA]** O caixa pode estornar apenas lançamentos feitos por ele no mesmo dia; os demais exigem Gerente ou Dono.

### F1-05 — Ajuste de saldo (2 pontos)

Como dono, quero ajustar o saldo de um cliente ao migrar do caderno, para começar com os valores certos.

```gherkin
Funcionalidade: Ajuste

  Cenário: Dono lança saldo inicial
    Quando o dono lança um ajuste de R$ 150,00 a débito com o motivo "saldo do caderno"
    Então o saldo devedor é R$ 150,00
    E o ajuste conta como compra em aberto na data informada

  Cenário: Gerente não pode ajustar
    Quando a gerente tenta lançar um ajuste
    Então o lançamento é recusado com o código "FORBIDDEN"

  Cenário: Ajuste sem motivo
    Quando o dono lança um ajuste sem motivo
    Então o lançamento é recusado com o código "VALIDATION_FAILED"
```

### F1-06 — Extrato e fechamento de caixa (3 pontos)

Como dona, quero ver o extrato de um cliente e quanto cada caixa recebeu no dia, para conferir o dinheiro.

```gherkin
Funcionalidade: Fechamento de caixa

  Cenário: Total por meio de pagamento
    Dado que hoje a caixa "Jéssica" recebeu R$ 50,00 em dinheiro, R$ 30,00 por Pix e R$ 20,00 no cartão
    Quando a dona consulta o fechamento de hoje
    Então a linha de "Jéssica" mostra R$ 50,00 em dinheiro, R$ 30,00 em Pix, R$ 20,00 em cartão e total de R$ 100,00

  Cenário: Pagamento estornado não conta
    Dado que hoje "Jéssica" recebeu R$ 50,00 em dinheiro e esse pagamento foi estornado
    Quando a dona consulta o fechamento de hoje
    Então o total de "Jéssica" é R$ 0,00

  Cenário: Caixa só vê o próprio fechamento
    Quando "Jéssica" consulta o fechamento de outro operador
    Então a consulta é recusada com o código "FORBIDDEN"
```

- O extrato é paginado por cursor, do mais recente para o mais antigo.

## Clientes

### F1-07 — Cadastrar e buscar cliente (5 pontos)

Como caixa, quero cadastrar um cliente só com o nome e achá-lo pelo apelido, para atender rápido.

```gherkin
Funcionalidade: Cadastro de clientes

  Cenário: Cadastro mínimo
    Quando a caixa cadastra um cliente com o nome "Raimundo Nonato"
    Então o cliente é criado com saldo devedor de R$ 0,00

  Cenário: Busca por apelido sem acento
    Dado o cliente "Raimundo Nonato" com apelido "Seu Dico da Balsa"
    Quando a caixa busca por "dico"
    Então "Raimundo Nonato" aparece no resultado

  Cenário: Telefone repetido entre clientes ativos
    Dado um cliente ativo com o telefone "(92) 99123-4567"
    Quando a caixa cadastra outro cliente com o telefone "92991234567"
    Então o cadastro é recusado com o código "CUSTOMER_PHONE_TAKEN"

  Cenário: Recadastro após arquivamento
    Dado um cliente arquivado com o telefone "(92) 99123-4567"
    Quando a caixa cadastra outro cliente com o mesmo telefone
    Então o cadastro é aceito

  Cenário: Telefone de outro estado
    Quando a caixa cadastra um cliente com o telefone "(11) 98888-7777"
    Então o cadastro é aceito

  Cenário: Arquivar com dívida
    Dado um cliente com saldo devedor de R$ 10,00
    Quando a dona tenta arquivá-lo
    Então a operação é recusada com o código "CUSTOMER_HAS_DEBT"

  Cenário: Limite do plano gratuito
    Dado um comércio no plano gratuito com o número máximo de clientes ativos
    Quando a caixa cadastra mais um cliente
    Então o cadastro é recusado com o código "PLAN_LIMIT_REACHED"
```

### F1-08 — Dia preferencial de pagamento (2 pontos)

Como dono, quero registrar quando cada cliente costuma pagar, para cobrar na hora certa.

- Aceita dia do mês, quinzena, dia do benefício, safra ou texto livre ("depois do defeso").
- Aparece na ficha do cliente e na lista de devedores.

## Acesso

### F1-09 — Criar conta e comércio (5 pontos)

Como dono, quero criar minha conta com telefone e senha, para começar a usar em minutos.

```gherkin
Funcionalidade: Cadastro e login

  Cenário: Cadastro
    Quando eu me cadastro com nome, telefone, senha e o nome do comércio
    Então o comércio é criado no fuso "America/Manaus"
    E eu sou o Dono
    E a sessão está iniciada neste aparelho

  Cenário: Senha errada e telefone inexistente dão a mesma resposta
    Quando eu tento entrar com um telefone inexistente
    E depois com um telefone existente e senha errada
    Então as duas respostas são idênticas

  Cenário: Bloqueio progressivo
    Quando eu erro a senha 5 vezes seguidas
    Então novas tentativas são recusadas por um tempo crescente com o código "RATE_LIMITED"

  Cenário: Reuso de refresh token
    Dado uma sessão cujo refresh token já foi trocado
    Quando o token antigo é apresentado de novo
    Então todas as sessões daquele aparelho são encerradas
```

### F1-10 — Membros e papéis (3 pontos)

Como dono, quero adicionar caixa e gerente, para que cada pessoa tenha só o acesso de que precisa.

- O Dono adiciona e remove membros; sempre resta ao menos um Dono.
- Um usuário de um comércio não enxerga nem altera dados de outro (teste automatizado obrigatório).

### F1-11 — Configurações do comércio (3 pontos)

Como dono, quero ajustar fuso, política de excedente, modo do limite, retroatividade, horário de cobrança e chave Pix.

- Padrões **[PREMISSA]**: fuso `America/Manaus`, excedente rejeitado, limite em modo aviso, 7 dias de retroatividade, cobrança das 8h às 20h, sem cobrança aos domingos e feriados, sem juros.

## Offline

### F1-12 — Lançar sem internet e sincronizar (13 pontos)

Como dono de taberna ribeirinha, quero usar o aplicativo por dias sem sinal, para não depender de internet para vender.

```gherkin
Funcionalidade: Sincronização offline

  Cenário: Lançamento offline chega ao servidor
    Dado que o aparelho está sem internet
    Quando a caixa lança uma compra de R$ 25,00 para "Zé da Balsa"
    Então o aplicativo mostra o saldo de R$ 25,00 marcado como "pendente de envio"
    Quando a internet volta
    Então o lançamento aparece no servidor com a mesma identificação
    E o aplicativo mostra "tudo sincronizado"

  Cenário: Lote reenviado não duplica
    Dado um lote de comandos já aplicado
    Quando o aparelho reenvia o mesmo lote
    Então cada comando volta como "duplicate"
    E nenhum lançamento novo é criado

  Cenário: Dois aparelhos lançam para o mesmo cliente
    Dado os aparelhos "balcão" e "barco", ambos sem internet
    Quando "balcão" lança uma compra de R$ 30,00 e "barco" lança uma compra de R$ 20,00 para o mesmo cliente
    E os dois sincronizam em qualquer ordem
    Então o saldo devedor é R$ 50,00 nos dois aparelhos

  Cenário: Lançamento feito há cinco dias sem sinal
    Dado que o comércio permite 3 dias de retroatividade
    E uma compra lançada offline há 5 dias, com a data daquele dia
    Quando o aparelho sincroniza
    Então o comando é aplicado

  Cenário: Pagamento offline maior que a dívida
    Dado que o comércio rejeita pagamento acima da dívida
    E um pagamento offline de R$ 50,00 para um cliente que deve R$ 30,00
    Quando o aparelho sincroniza
    Então o comando volta como "needs_review"
    E aparece na fila de revisão

  Cenário: Telefone duplicado criado offline
    Dado dois aparelhos que cadastraram offline clientes com o mesmo telefone
    Quando os dois sincronizam
    Então o segundo cadastro volta como "needs_review"

  Cenário: Relógio do aparelho adiantado
    Dado um comando com horário de gravação 2 horas no futuro
    Quando o aparelho sincroniza
    Então o comando volta como "needs_review"

  Cenário: Aparelho revogado
    Dado que o dono revogou o aparelho "barco"
    Quando "barco" tenta sincronizar
    Então recebe o código "DEVICE_REVOKED"
    E o aplicativo apaga os dados locais
```

Propriedade verificada por teste gerativo: para qualquer conjunto de comandos válidos, qualquer ordem de entrega e qualquer repetição resultam no mesmo saldo por cliente.

### F1-13 — Fila de revisão (5 pontos)

Como gerente, quero decidir o que fazer com lançamentos que o sistema não pôde aplicar sozinho, para não perder registros.

- A fila mostra o comando, o motivo e o aparelho de origem.
- Aprovar aplica o comando com a autorização de quem aprovou; descartar exige motivo. As duas decisões ficam auditadas.
- O PWA avisa de forma visível quando há itens pendentes.

### F1-14 — Dispositivos (3 pontos)

Como dono, quero ver os aparelhos com acesso e revogar um aparelho perdido.

## PWA

### F1-15 — Lançamento em até três toques (8 pontos)

Como caixa, quero lançar uma compra em três toques a partir da tela inicial: escolher o cliente, digitar o valor, confirmar.

- Teclado numérico; o valor aparece como "1.234,56".
- Fontes grandes, contraste alto, alvos de toque de 48 px, ícones com rótulo, uso com leitor de tela.
- Indicadores permanentes de estado: sem internet, sincronizando, pendências, itens em revisão.
- Teste de ponta a ponta do cenário offline → online.

### F1-16 — Painel (5 pontos)

Como dona, quero abrir o aplicativo e ver quanto tenho a receber, quem está devendo há mais tempo e quanto recebi no período.

## Cobrança

### F1-17 — Cobrar pelo WhatsApp (5 pontos)

Como dono, quero enviar um lembrete educado pelo WhatsApp, para receber sem constranger o cliente.

```gherkin
Funcionalidade: Cobrança respeitosa

  Contexto:
    Dado o comércio "Mercadinho da Socorro" no fuso "America/Manaus"
    E o cliente "Zé da Balsa" com saldo devedor de R$ 80,00, telefone e consentimento de comunicação

  Cenário: Dentro do horário
    Dado que agora são 10:00 de uma terça-feira em Manaus
    Quando a dona prepara uma cobrança
    Então recebe um texto com o valor de R$ 80,00 e um link do WhatsApp para o telefone do cliente

  Esquema do Cenário: Fora do horário
    Dado que agora são <hora> de uma terça-feira em Manaus
    Quando a dona prepara uma cobrança
    Então a operação é recusada com o código "OUTSIDE_COLLECTION_HOURS"
    E a resposta informa o próximo horário permitido

    Exemplos:
      | hora  |
      | 07:59 |
      | 20:00 |
      | 23:30 |

  Cenário: Domingo
    Dado que hoje é domingo e o comércio não cobra aos domingos
    Quando a dona prepara uma cobrança
    Então a operação é recusada com o código "OUTSIDE_COLLECTION_HOURS"

  Cenário: Sem consentimento
    Dado que "Zé da Balsa" não consentiu em receber mensagens
    Quando a dona prepara uma cobrança
    Então a operação é recusada com o código "CONSENT_REQUIRED"

  Cenário: Cliente sem dívida
    Dado que "Zé da Balsa" tem saldo devedor de R$ 0,00
    Quando a dona prepara uma cobrança
    Então a operação é recusada com o código "VALIDATION_FAILED"
```

- Os modelos de mensagem não expõem, não ameaçam e não mencionam terceiros (CDC art. 42). O texto é revisado pelo comerciante antes do envio.

### F1-18 — Comprovante (2 pontos)

Como caixa, quero enviar ao cliente o comprovante de cada compra e pagamento, com o saldo atualizado.

### F1-19 — Pix copia e cola (3 pontos)

Como dono, quero gerar um código Pix com o valor da dívida.

```gherkin
Funcionalidade: Pix copia e cola

  Cenário: Código com o valor da dívida
    Dado um comércio com chave Pix, nome e cidade configurados
    E um cliente com saldo devedor de R$ 80,00
    Quando a dona gera o código Pix
    Então o código segue o padrão BR Code
    E contém o valor "80.00"
    E termina com um CRC16 válido

  Cenário: Comércio sem chave Pix
    Dado um comércio sem chave Pix
    Quando a dona gera o código Pix
    Então a operação é recusada com o código "PIX_KEY_NOT_CONFIGURED"
```

## Relatórios e LGPD

### F1-20 — Exportar CSV (2 pontos)

Como dona, quero exportar os lançamentos de um período, para entregar ao contador.

### F1-21 — Direitos do titular (5 pontos)

Como dono, quero atender ao pedido de um cliente para ver ou apagar seus dados.

```gherkin
Funcionalidade: Direitos do titular

  Cenário: Exportação
    Quando o dono exporta os dados de "Zé da Balsa"
    Então recebe os dados cadastrais, os consentimentos e todos os lançamentos do cliente

  Cenário: Anonimização preserva o histórico
    Dado um cliente com saldo zero e 10 lançamentos
    Quando o dono anonimiza o cliente
    Então nome, apelido, telefone, comunidade, referência e observações são apagados
    E os 10 lançamentos continuam existindo com os mesmos valores

  Cenário: Anonimização com dívida
    Dado um cliente com saldo devedor de R$ 10,00
    Quando o dono tenta anonimizá-lo
    Então a operação é recusada com o código "CUSTOMER_HAS_DEBT"
```

- O consentimento de comunicação registra data, canal e evidência, e pode ser revogado.

## Riscos

| Risco | Impacto | Mitigação |
| --- | --- | --- |
| Regras do ledger mal entendidas | Saldo errado, perda de confiança | Domínio e testes primeiro; cenários acima validados com comerciantes |
| Navegador apagar dados locais antes de sincronizar | Perda de lançamentos | Armazenamento persistente, aviso de pendências antigas, sync ao abrir |
| Relógio errado no aparelho | Datas erradas | Tolerância e fila de revisão; data civil mostrada na confirmação |
| Fluxo de três toques não se sustentar com validações | Abandono | Teste com usuários reais em aparelho de entrada antes de fechar a Fase 1 |
| Mensagens de cobrança interpretadas como constrangimento | Risco jurídico | Modelos fixos revisados, horário bloqueado no servidor, consentimento obrigatório |

Total: 95 pontos.
