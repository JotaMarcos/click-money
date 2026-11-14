# ADR 0014 — PWA offline-first

- Status: Aceita
- Data: 2026-11-13

## Contexto

O comerciante usa Android de entrada (2 a 3 GB de RAM, pouco armazenamento), com 3G instável ou sem sinal por dias. Publicar em loja de aplicativos acrescenta custo, atrito de atualização e consumo de espaço.

## Decisão

- PWA instalável em TypeScript estrito, com React e Vite.
- **Base local**: IndexedDB via Dexie guarda clientes, lançamentos, saldos e a outbox de comandos. A tela sempre lê da base local; a rede só atualiza a base.
- **Escrita**: toda ação grava na base local e enfileira um comando com `command_id` UUIDv7. Um sincronizador envia a fila por `/v1/sync/push` e busca mudanças por `/v1/sync/pull`, com backoff exponencial e jitter. Online ou offline, o caminho é o mesmo.
- **Saldo local**: o aparelho calcula um saldo provisório somando os comandos pendentes ao último saldo recebido do servidor. O valor do servidor prevalece após a sincronização, e a tela marca o que ainda é provisório.
- **Service worker** com Workbox: pré-cache do shell da aplicação e atualização controlada (o usuário é avisado e a troca ocorre fora de um lançamento em andamento).
- **TanStack Query** apenas para leituras que só existem online (relatórios, exportações, configurações).
- **Orçamento**: bundle inicial de até 200 KB gzip, verificado no CI. Rotas secundárias são carregadas sob demanda.
- **Design system próprio**, com tokens de cor e tipografia, temas claro e escuro, contraste alto e alvos de toque de pelo menos 48 px. Sem biblioteca de componentes.
- **Textos** em arquivos de mensagens (pt-BR padrão), nunca no código.
- **Dinheiro** em centavos inteiros; a entrada usa teclado numérico e máscara "1.234,56".
- **Sessão** por cookie `HttpOnly` (ADR 0010). Dados locais sensíveis cifrados com Web Crypto (ADR 0006).

## Alternativas

- **Aplicativo nativo ou React Native**: descartado. Loja, atualização e tamanho de instalação pesam para este público; fica como possibilidade se o PWA esbarrar em limite do navegador.
- **Preact, Svelte ou Solid**: bundles menores, mas a especificação pede React. Preact é o plano B (ADR 0012).
- **Tela lendo direto da API com cache**: descartado. Não sustenta dias sem internet.

## Consequências

- O navegador pode apagar o IndexedDB sob pressão de armazenamento. O PWA pede armazenamento persistente (`navigator.storage.persist()`) e avisa quando há comandos pendentes há muito tempo.
- Sincronização em segundo plano (Background Sync) não é garantida em todos os navegadores; o sincronizador roda com o aplicativo aberto e ao voltar a conexão.
- Manter regras de saldo provisório no cliente duplica uma parte pequena da lógica do ledger; o servidor continua sendo a autoridade.
