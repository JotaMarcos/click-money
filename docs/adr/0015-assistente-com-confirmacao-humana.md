# ADR 0015 — Assistente de IA com confirmação humana

- Status: Aceita (implementação na Fase 2, atrás de feature flag)
- Data: 2026-11-13

## Contexto

O assistente responde perguntas em linguagem natural e sugere cobranças. Ele lê textos escritos por terceiros (nomes, observações de clientes), que podem conter instruções maliciosas, e conversa com provedores externos de LLM. Um erro dele não pode mover dinheiro nem falar com um cliente em nome do comerciante.

## Decisão

- **Porta `LLMProvider`** definida em `assistant/app`, com adaptadores para provedores comerciais e para modelos auto-hospedados. A seleção é por configuração, com timeout, fallback e circuit breaker por provedor.
- **Laço do agente escrito em Go**, sem framework: planejar, chamar ferramenta, observar, responder. Tem limite de passos, orçamento de tokens e de custo por tenant e respeita o cancelamento do `context`.
- **Ferramentas tipadas**: cada ferramenta é um caso de uso já existente de outro módulo, com esquema JSON gerado a partir de structs Go. A ferramenta roda com o tenant e o papel do usuário da sessão; a autorização é a do caso de uso, não a do prompt.
- **Escrita só preparada**: `preparar_lancamento` e `preparar_cobranca` gravam uma proposta com prazo de validade. A execução é uma chamada separada da API, feita pela interface depois que o usuário confirma, e passa pelo caso de uso normal do `ledger` ou de `collections`. O módulo `assistant` não tem porta de escrita para o ledger nem para envio de mensagens.
- **Dados como dados**: conteúdo vindo de clientes entra no prompt delimitado e marcado como não confiável. Nomes e telefones são trocados por pseudônimos antes de sair para o provedor e restaurados na resposta, quando a pergunta permite.
- **Consentimento**: o assistente só funciona depois que o Dono aceita o uso de IA, e pode ser desligado a qualquer momento.
- **Rastro**: cada execução (passos, ferramentas, latência, tokens, custo, resultado, avaliação do usuário) vai para o MongoDB, sem PII em log.
- **Avaliação**: um conjunto versionado de perguntas e respostas esperadas, incluindo casos adversariais de injeção de prompt, roda no CI quando o prompt, as ferramentas ou o modelo mudam.

## Alternativas

- **Framework de agentes**: descartado. Dependência pesada, laço opaco e difícil de limitar e auditar.
- **Agente com permissão de escrita e confirmação "no prompt"**: descartado. Uma injeção bem-sucedida gravaria no ledger.
- **Provedor único fixo**: descartado. Custo, disponibilidade e residência de dados mudam; a porta custa pouco.

## Consequências

- O pior caso de uma injeção de prompt é uma resposta errada ou uma proposta que o usuário ainda precisa confirmar.
- A pseudonimização limita perguntas que dependem do nome real (por exemplo, busca fonética); esses casos são resolvidos por ferramenta, não pelo modelo.
- O custo por tenant precisa de painel e de teto desde o primeiro dia da Fase 2.
- O provedor inicial de LLM segue em aberto e será decidido em ADR próprio antes da Fase 2.
