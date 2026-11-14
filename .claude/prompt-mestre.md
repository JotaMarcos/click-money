# PROMPT MESTRE — Click-Money em Go

## 0. Papel e postura

Você vai atuar como um time sênior completo (Arquiteto de Software, Analista de Negócio, Engenheiro de Software, Desenvolvedor Full Stack, DBA, QA e DevOps/SRE), cada um com mais de 20 anos de experiência em sistemas financeiros e de varejo. Seu trabalho é **projetar e implementar do zero** uma plataforma de gestão de crédito informal ("fiado") para pequenos comércios, pronta para produção e para ser vendida no mercado.

Regras de postura:
- Decida e justifique. Quando houver alternativas, registre a decisão como ADR (Architecture Decision Record) curto: contexto, decisão, consequências.
- Não invente requisitos sem marcar como **[PREMISSA]**. Não deixe nada "TODO" sem um issue correspondente.
- Código, commits, documentação e comentários **sem qualquer assinatura, autoria, menção a ferramentas de geração ou a autores anteriores**. Autoria fica a cargo do controle de versão do time.
- Idioma: domínio, UI, mensagens de erro e documentação em **português do Brasil**; identificadores de código em **inglês**; termos de domínio podem ter glossário PT↔EN.

---

## 1. Visão do produto

**Nome do produto:** **Click-Money**. Identificador técnico: `click-money` (repositório, imagens Docker, bancos `click_money`); módulo Go `github.com/<organizacao>/click-money`. Na interface e no marketing, use sempre a grafia "Click-Money".

**Problema.** Em Manaus e, sobretudo, no interior do Amazonas, o fiado é um instrumento de sobrevivência econômica: mercadinhos, tabernas, flutuantes, batelões/regatões, feirantes, farmácias, padarias e postos de combustível vendem a prazo para clientes cuja renda é irregular (pesca, extrativismo, agricultura familiar, diárias, benefícios sociais, salários pagos por quinzena). O controle hoje é feito em caderno, sujeito a perda (umidade, enchentes, incêndio), conflito ("eu já paguei") e inadimplência sem histórico.

**Solução.** Plataforma *offline-first* que permite ao comerciante:
1. Cadastrar clientes rapidamente (nome, telefone, apelido, comunidade/bairro, referência).
2. Lançar compras no fiado e receber pagamentos (dinheiro, Pix, cartão, troca/escambo registrado como "outros").
3. Ver saldo devedor por cliente e total a receber, com histórico imutável e auditável.
4. Cobrar de forma respeitosa pelo WhatsApp e enviar comprovante de cada lançamento ao cliente.
5. Definir limite de crédito e dia preferencial de pagamento por cliente.
6. Operar sem internet por dias e sincronizar quando houver sinal.
7. Contar com um **assistente inteligente** (agente de IA) que responde perguntas em linguagem natural, prioriza cobranças e sugere mensagens — sempre com confirmação humana.

**Público.** Microempreendedores (MEI), pequenos comércios e cooperativas; perfil de baixa familiaridade digital; aparelhos Android de entrada (2–3 GB de RAM, armazenamento limitado), conexão 3G/4G instável, internet via satélite ou rádio, quedas de energia.

**Modelo de negócio [PREMISSA].** Freemium (até N clientes ativos), plano pago mensal via Pix recorrente/boleto, plano para associações comerciais e prefeituras (programas de inclusão produtiva).

---

## 2. Contexto regional obrigatório (Manaus/AM e interior)

O sistema DEVE tratar explicitamente:

| Tema | Requisito |
| --- | --- |
| Fuso horário | Fuso **por comércio**, padrão `America/Manaus` (UTC−4, sem horário de verão). Suportar `America/Eirunepe` (UTC−5) para municípios do sudoeste do estado (ex.: Eirunepé, Envira, Ipixuna, Guajará, Boca do Acre, Pauini, Itamarati). "Hoje", "esta semana" e validações de "data futura" são sempre calculados no fuso do comércio — **nunca em UTC**. Persistir instantes em UTC (`timestamptz`) e datas civis como `date`. |
| Conectividade | Offline-first real: captura local, fila de envio com idempotência, sincronização incremental, payload mínimo (compressão gzip/brotli, respostas paginadas e enxutas), tolerância a latência de satélite (timeouts e retries com backoff exponencial e jitter). |
| Dispositivos | PWA instalável, bundle inicial ≤ 200 KB gzip, funcionar fluido em Android 8+ com 2 GB RAM, alvo Lighthouse ≥ 90 em perfil "Slow 4G". |
| Usabilidade | Fluxo de lançamento em até 3 toques; fontes grandes; alto contraste (uso ao sol); teclado numérico para valores; busca por apelido; ícones com rótulo; suporte a leitor de tela. |
| Pagamentos | Pix como meio principal (gerar QR Code estático "copia e cola" com o valor da dívida no padrão BR Code/EMV); dinheiro; cartão; "outros" com descrição. |
| Renda sazonal | Campo "dia/período preferencial de pagamento" (ex.: dia do benefício, quinzena, safra, fim do defeso) e lembretes agendados conforme esse período. |
| Idiomas | Arquitetura de i18n pronta (pt-BR padrão). São Gabriel da Cachoeira tem línguas indígenas cooficiais (Nheengatu, Tukano, Baniwa); não traduzir agora, mas não acoplar textos ao código. |
| Legislação | **LGPD** (base legal, consentimento para comunicações, exportação e exclusão de dados do titular, registro de tratamento); **CDC art. 42** (cobrança não pode expor ao ridículo nem constranger — modelos de mensagem devem ser respeitosos, sem horários abusivos: bloquear envio fora de 8h–20h no fuso local, e domingos/feriados configurável); sem juros por padrão — se habilitados, multa ≤ 2% e juros de mora simples configuráveis e exibidos de forma transparente. |
| Residência de dados | Hospedagem em região brasileira (ex.: São Paulo) para latência e conformidade. |

---

## 3. Stack tecnológica (obrigatória)

**Backend — Go**
- Go na versão estável mais recente (mínimo 1.25), módulos, `go.work` se houver múltiplos módulos.
- HTTP: `net/http` da biblioteca padrão com roteamento por padrão de método/rota (Go 1.22+). Middlewares próprios e enxutos. Proibido framework "mágico".
- Logs: `log/slog` (JSON em produção), com `request_id`, `tenant_id`, `trace_id`; redação de PII e segredos.
- Observabilidade: OpenTelemetry (traces + métricas), endpoint Prometheus, `pprof` protegido.
- Configuração: variáveis de ambiente validadas na inicialização (fail fast), arquivo `.env` apenas em desenvolvimento.
- Validação: validação explícita no DTO de entrada + invariantes no domínio.
- Contrato: **OpenAPI 3.1** como fonte da verdade (design-first), geração de servidor/tipos com `oapi-codegen`; cliente TypeScript gerado para o front.
- Concorrência: `context.Context` em toda a cadeia, cancelamento e deadlines; `errgroup` para paralelismo; nada de goroutine sem dono.
- Ferramentas: `golangci-lint` (config estrita), `govulncheck`, `gosec`, `gofumpt`, `go test -race`, fuzzing nativo.

**Persistência poliglota**

| Ambiente | Relacional | Documentos |
| --- | --- | --- |
| Produção / homologação | **PostgreSQL** (16+) | **MongoDB** (7+), replica set |
| Desenvolvimento local | **SQLite** (driver puro Go `modernc.org/sqlite`, sem CGO, modo WAL) | **SQLite** (tabela de documentos com JSON1) via adaptador que implementa as mesmas portas |
| Testes de integração / CI | PostgreSQL e MongoDB reais via **testcontainers-go** | idem |

Divisão de responsabilidades:
- **PostgreSQL = fonte da verdade transacional (ACID):** identidade, comércios, membros e papéis, clientes, **livro-razão (ledger)**, alocações de pagamentos, saldos projetados, assinaturas/planos, outbox transacional.
- **MongoDB = dados de alto volume, flexíveis ou derivados:** trilha de auditoria detalhada, log de sincronização por dispositivo, histórico de mensagens/cobranças enviadas e status de entrega, memória e rastros de execução do agente de IA (prompts, chamadas de ferramenta, custo, latência), templates de mensagens, modelos de leitura analíticos (read models) e relatórios pré-agregados.
- Nenhuma regra financeira depende do MongoDB. Se o MongoDB estiver fora, o núcleo continua operando (degradação graciosa; eventos ficam na outbox).
- Acesso a dados: SQL explícito (`pgx/v5` + `sqlc` com conjuntos de queries por dialeto quando necessário); driver oficial `mongo-go-driver v2`. **Sem ORM.**
- Migrações versionadas com `goose` (dialetos PostgreSQL e SQLite), executadas por binário `cmd/migrate`, nunca automaticamente no boot de produção.
- Portabilidade SQLite↔PostgreSQL: evitar recursos exclusivos no núcleo de queries de dev; recursos exclusivos de produção (RLS, índices parciais avançados, particionamento) entram em migrações específicas de PostgreSQL com testes em container.

**Frontend**
- PWA offline-first em TypeScript (React + Vite), cliente da API gerado do OpenAPI, cache de servidor com TanStack Query, **IndexedDB (Dexie)** como base local, fila de comandos (outbox) com reenvio idempotente, roteamento real com URLs (deep links e botão voltar), Workbox para service worker.
- Design system próprio e acessível (WCAG 2.2 AA), tokens de cor/tipografia, temas claro/escuro, componentes testados.

**Integrações**
- WhatsApp: fase 1 por link `wa.me` (sem custo); fase 2 pela **API oficial WhatsApp Business Cloud** com templates aprovados, opt-in registrado e webhooks de status.
- Pix: geração de BR Code estático (sem PSP) na fase 1; integração com PSP (cobrança dinâmica + webhook de liquidação) na fase 2 por porta `PaymentProvider`.
- E-mail transacional e SMS como fallback por porta `Notifier`.

---

## 4. Arquitetura

**Estilo:** monólito modular com arquitetura hexagonal (ports & adapters) e DDD tático leve. Fronteiras de módulo rígidas para permitir extração futura em serviços, sem pagar o custo de microserviços agora.

**Bounded contexts (módulos):**
1. `identity` — usuários, credenciais, sessões, MFA opcional, recuperação de acesso.
2. `tenancy` — comércio (tenant), membros, papéis (Dono, Gerente, Caixa), plano, configurações (fuso, horário de cobrança, juros).
3. `customers` — clientes, contatos, consentimentos LGPD, limite de crédito, dia preferencial, arquivamento.
4. `ledger` — lançamentos (compra no fiado, pagamento, estorno, ajuste), alocação FIFO, saldo, extrato. **Núcleo crítico.**
5. `collections` — régua de cobrança, lembretes agendados, mensagens, comprovantes.
6. `sync` — protocolo de sincronização de dispositivos offline.
7. `reporting` — indicadores, aging da carteira (0–30, 31–60, 61–90, 90+), exportações (CSV/PDF).
8. `assistant` — agente de IA (ver seção 7).
9. `platform` (não é domínio) — config, logs, banco, HTTP, auth, telemetria, outbox, relógio, gerador de IDs.

**Estrutura de diretórios (referência):**
```
/cmd
  /api        # servidor HTTP
  /worker     # consumidores da outbox, agendador de lembretes, projeções para MongoDB
  /migrate    # migrações
/internal
  /<modulo>
    /domain   # entidades, value objects, regras, erros de domínio (sem dependências externas)
    /app      # casos de uso (commands/queries), portas de saída (interfaces)
    /adapters
      /http       # handlers, DTOs, mapeamento de erros
      /postgres   # repositórios
      /sqlite     # repositórios de dev
      /mongo      # repositórios de documentos
  /platform
/api          # openapi.yaml
/migrations   # postgres/ e sqlite/
/web          # PWA
/deploy       # Dockerfile, compose, IaC, dashboards
/docs         # ADRs, C4, runbooks, glossário
```
Regras: `domain` não importa nada de `adapters` nem de bibliotecas de infraestrutura; módulos só se comunicam por interfaces públicas do `app` ou por eventos de domínio; injeção de dependências manual na `main` (composition root), sem container de DI; interfaces definidas no lado de quem consome.

**Fluxo de escrita:** Handler HTTP → DTO validado → Caso de uso → Agregado de domínio → Repositório (transação) → grava evento na **outbox** na mesma transação → worker publica para projeções (MongoDB), notificações e auditoria.

**Padrões de projeto exigidos (onde fizer sentido, sem cerimônia):** Repository, Unit of Work (transação explícita por caso de uso), Transactional Outbox, CQRS leve (escrita no PostgreSQL, leituras pesadas em read models), Strategy (meios de pagamento, regras de juros, provedores de LLM), Adapter (WhatsApp, Pix, LLM), Specification (filtros de carteira), Idempotency Key, Value Objects (`Money`, `Phone`, `CivilDate`, `TenantID`), Functional Options para configuração de componentes, Circuit Breaker e Retry com backoff para integrações externas.

**Diagramas:** entregar C4 (Contexto, Contêineres, Componentes do `ledger`) em Mermaid, diagrama ER e diagramas de sequência para: lançar fiado offline e sincronizar; pagamento com alocação FIFO; cobrança via WhatsApp; consulta ao assistente.

---

## 5. Modelo de domínio e regras de negócio

**Dinheiro:** `Money` como inteiro de centavos (`int64`) + moeda (`BRL`). Proibido `float` em qualquer cálculo financeiro, no backend e no frontend. Parse de entrada em formato brasileiro ("1.234,56"). Arredondamento bancário documentado onde houver divisão (juros).

**Identificadores:** UUIDv7 gerados **no cliente** para entidades criadas offline (permite idempotência e ordenação temporal).

**Ledger (livro-razão):**
- Lançamentos são **imutáveis e somente-inserção** (append-only). Correção = lançamento de **estorno** referenciando o original, com motivo obrigatório e autor. Nunca `UPDATE`/`DELETE` em lançamentos (garantir também por permissão no banco e trigger).
- Tipos: `CREDIT_SALE` (compra no fiado), `PAYMENT`, `REVERSAL`, `ADJUSTMENT` (somente papel Dono, com motivo), `INTEREST` (se habilitado).
- Cada lançamento: `id`, `tenant_id`, `customer_id`, `type`, `amount_cents > 0` (CHECK), `occurred_on` (data civil no fuso do comércio), `recorded_at` (instante UTC), `payment_method`, `description`, `device_id`, `author_id`, `idempotency_key` (único por tenant), `reverses_entry_id`.
- **Saldo projetado** em tabela `customer_balances` (saldo, nº de compras em aberto, data da compra mais antiga em aberto, `version`) atualizada **na mesma transação** do lançamento, com controle de concorrência otimista. Saldo nunca é recalculado varrendo o histórico em requisições de listagem.
- **Alocação FIFO** de pagamentos às compras mais antigas em tabela `payment_allocations`, permitindo status por compra (Em aberto / Parcial / Paga) e aging correto.
- Pagamento maior que o saldo: online → rejeitar com erro de negócio claro, ou registrar o excedente como **crédito a favor do cliente** se a configuração do comércio permitir; offline → aceitar no dispositivo e, na sincronização, aplicar a regra configurada; conflitos que exijam decisão humana vão para uma **fila de revisão**.
- Limite de crédito: aviso (soft) ou bloqueio (hard) configurável; liberação pontual exige papel Gerente/Dono e fica auditada.
- Datas futuras proibidas (no fuso do comércio); datas retroativas permitidas até N dias configuráveis, acima disso exige papel Gerente.
- Fechamento de caixa diário por operador (total recebido por meio de pagamento).

**Clientes:**
- Telefone normalizado em E.164 (`+5592…`), validação de DDDs da Região Norte sem impedir outros DDDs.
- Unicidade de telefone por comércio **apenas entre clientes ativos** (índice único parcial `WHERE archived_at IS NULL`), permitindo recadastro após arquivamento.
- Arquivar só com saldo zero; exclusão definitiva apenas por solicitação LGPD (anonimização preservando a integridade do ledger).
- Campos: nome, apelido, telefone, comunidade/bairro, ponto de referência, observação, limite, dia preferencial, consentimento de comunicação (data, canal, evidência).

**Multi-tenant:** `tenant_id` em todas as tabelas de negócio, verificado em todo caso de uso e reforçado por **Row-Level Security** no PostgreSQL (`SET LOCAL app.tenant_id` por transação). Testes automatizados que tentam acesso cruzado entre tenants devem falhar.

---

## 6. Sincronização offline (protocolo)

- Cliente mantém outbox local de **comandos** (não de estados): `CreateCustomer`, `RecordCreditSale`, `RecordPayment`, `ReverseEntry`, cada um com `command_id` (UUIDv7), `device_id`, `client_recorded_at` e `base_version` quando aplicável.
- `POST /v1/sync/push` em lote (até N comandos), processamento idempotente (`command_id` já visto → devolver o mesmo resultado), resposta por comando: `applied | duplicate | rejected(motivo) | needs_review`.
- `GET /v1/sync/pull?cursor=` devolve mudanças desde o cursor (paginado, ordenado), incluindo saldos projetados atualizados.
- Como o ledger é append-only, lançamentos de dispositivos diferentes são comutativos; conflitos reais limitam-se a cadastro (ex.: telefone duplicado) e regras de saldo/limite → fila de revisão com UI própria.
- Registro de dispositivo, revogação remota (aparelho perdido) e criptografia em repouso dos dados locais sensíveis (Web Crypto, chave derivada da sessão).
- Testes de propriedade: qualquer ordem de entrega dos mesmos comandos converge para o mesmo saldo.

---

## 7. Assistente inteligente (IA agêntica em Go)

**Objetivo:** reduzir trabalho do comerciante sem jamais tirar dele o controle sobre dinheiro e comunicação com clientes.

**Arquitetura:**
- Porta `LLMProvider` (interface Go) com adaptadores intercambiáveis: provedores comerciais via API e modelos abertos auto-hospedados. Seleção por configuração; fallback e timeout por provedor.
- Laço do agente implementado explicitamente em Go: planejar → chamar ferramenta → observar → responder, com limite de passos, orçamento de tokens/custo por tenant e cancelamento por `context`.
- **Ferramentas (tools) tipadas**, cada uma um caso de uso já existente, com esquema JSON gerado a partir de structs Go:
  - Leitura: `consultar_saldo_cliente`, `listar_inadimplentes(filtros)`, `resumo_do_dia`, `aging_da_carteira`, `historico_cliente`.
  - Sugestão: `sugerir_prioridade_de_cobranca`, `redigir_mensagem_de_cobranca` (respeitando CDC, tom gentil, horário permitido).
  - Escrita: `preparar_lancamento` e `preparar_cobranca` — **apenas preparam**; a execução exige confirmação explícita do usuário na UI (human-in-the-loop). O agente nunca grava no ledger nem envia mensagem sozinho.
- Entradas: texto e, na fase 2, áudio (transcrição) — comerciantes da região usam muito áudio no WhatsApp.
- **Segurança:** dados de clientes (observações, nomes) tratados como *dados não confiáveis* (defesa contra prompt injection: delimitação, nenhuma instrução vinda de dados é executada, ferramentas com autorização verificada no backend e não no prompt); minimização de PII enviada ao provedor (pseudonimização de nomes/telefones quando possível); consentimento do comerciante para uso de IA; opção de desligar.
- **Observabilidade e avaliação:** cada execução registrada no MongoDB (passos, ferramentas, latência, tokens, custo, resultado, feedback 👍/👎); conjunto de avaliação (evals) versionado com perguntas reais anonimizadas e respostas esperadas, executado no CI para detectar regressões ao trocar modelo ou prompt.
- **Analítica/ML (fase 3):** score de risco de inadimplência explicável (regressão logística/gradient boosting treinado offline, servido em Go via modelo exportado), previsão de recebíveis por semana e detecção de anomalias em lançamentos — sempre como apoio, nunca como decisão automática de negar crédito.

---

## 8. Segurança

- Senhas com **Argon2id** (parâmetros documentados); política de senha razoável + login alternativo por código via WhatsApp/SMS (público com baixa familiaridade digital).
- Access token curto (15 min) + refresh token rotativo, armazenado como hash, com detecção de reuso e revogação por dispositivo. No navegador: cookies `HttpOnly`, `Secure`, `SameSite=Strict` + proteção CSRF; nunca token em `localStorage`.
- RBAC por papel no tenant; autorização verificada no caso de uso.
- Rate limiting e bloqueio progressivo em login, recuperação de senha e endpoints de envio de mensagem.
- Cabeçalhos de segurança, CORS restrito e retornando 403 (não 500) para origem não permitida, limite de tamanho de corpo, timeouts de servidor (`ReadHeaderTimeout` etc.).
- Segredos via gerenciador de segredos; nada sensível em repositório; varredura de segredos no CI.
- Auditoria: quem, quando, de qual dispositivo, o quê (antes/depois) para toda ação sensível.
- Modelagem de ameaças (STRIDE) documentada para o ledger, sync e assistente; checklist OWASP ASVS nível 2.

---

## 9. API

- REST versionada (`/v1`), JSON, envelope consistente `{ "data": ..., "meta": ... }`; erros no padrão **RFC 9457 (Problem Details)** com `code` estável de negócio (ex.: `PAYMENT_EXCEEDS_BALANCE`, `CREDIT_LIMIT_EXCEEDED`, `CUSTOMER_HAS_DEBT`), mensagens em pt-BR.
- Paginação por **cursor** (keyset) em listas; filtros e ordenação no banco — nunca carregar tudo em memória e filtrar na aplicação.
- Cabeçalho `Idempotency-Key` obrigatório em todo POST que cria lançamento.
- `ETag`/`If-None-Match` para leituras frequentes; compressão de resposta.
- Endpoints de saúde: `/livez`, `/readyz` (PostgreSQL obrigatório; MongoDB reportado como degradado sem derrubar a prontidão).

---

## 10. Banco de dados (diretrizes de DBA)

- Nomenclatura `snake_case`, tabelas no plural, PK UUID, `created_at`/`updated_at` `timestamptz`, FKs com `ON DELETE RESTRICT` no núcleo financeiro (nunca cascata apagando lançamentos).
- CHECKs para invariantes (`amount_cents > 0`, tipos válidos), índices compostos guiados pelas queries reais (`EXPLAIN ANALYZE` documentado para as 10 queries principais).
- Particionamento por tempo da tabela de lançamentos quando passar de volume definido; *autovacuum* ajustado.
- Usuário de aplicação sem permissão de DDL nem de `UPDATE/DELETE` em lançamentos; usuário de migração separado.
- Backups com PITR, retenção definida, **teste de restauração mensal** automatizado; RPO ≤ 5 min, RTO ≤ 1 h.
- MongoDB: esquemas validados (`$jsonSchema`), índices TTL para logs com retenção definida pela LGPD, índices por `tenant_id`.
- Seeds de desenvolvimento realistas (nomes e comunidades fictícios da região), nunca dados reais.

---

## 11. Qualidade (QA)

- Pirâmide de testes: unitários no domínio (tabelados, sem banco), integração com testcontainers, contrato contra o OpenAPI, E2E do PWA (Playwright) incluindo cenário **offline → online**.
- Cobertura mínima: 90% em `ledger/domain` e `ledger/app`; 75% no geral (cobertura é piso, não meta).
- Fuzzing do parse de valores monetários, telefones e do protocolo de sync; testes de propriedade da alocação FIFO e da convergência do sync.
- Testes de fuso horário: lançamento às 21h em Manaus deve ficar com a data local do dia, não do dia seguinte.
- Testes de carga (k6): metas p95 < 300 ms em leitura e < 500 ms em escrita com 500 tenants simultâneos.
- Testes de acessibilidade automatizados (axe) e testes manuais em aparelho Android de entrada sob 3G.

---

## 12. DevOps / SRE

- Binários estáticos (`CGO_ENABLED=0`), imagem Docker multi-stage `distroless`/`scratch`, usuário não-root, SBOM e assinatura de imagem.
- `docker compose` de desenvolvimento opcional (PostgreSQL + MongoDB + observabilidade) — o padrão local continua sendo SQLite, sem dependências.
- `Makefile`/`Taskfile` com: `dev`, `test`, `test-integration`, `lint`, `generate`, `migrate-up`, `build`, `e2e`.
- CI (GitHub Actions ou equivalente): lint → testes unitários com `-race` → integração → `govulncheck` → build → scan de imagem → deploy em homologação → smoke tests → aprovação → produção (blue/green ou canário).
- Infraestrutura como código (Terraform/OpenTofu); ambientes dev/homolog/prod isolados.
- Observabilidade: dashboards (RED por endpoint, filas da outbox, atraso de sync, custo de IA por tenant), alertas com SLO (disponibilidade 99,5% [PREMISSA]), runbooks para incidentes comuns.
- Versionamento semântico, changelog gerado de Conventional Commits, feature flags para funcionalidades de fase 2/3.

---

## 13. Processo de codificação e boas práticas

- Trunk-based com branches curtas, PR pequeno com descrição, revisão obrigatória, CI verde.
- Conventional Commits; nenhuma assinatura de ferramenta ou autor nas mensagens.
- Go idiomático: erros como valores com `fmt.Errorf("...: %w", err)`, erros de domínio tipados e checados com `errors.Is/As`; sem `panic` fora da inicialização; funções pequenas; nomes claros; comentários explicam o *porquê*; documentação Go em todo pacote exportado.
- SOLID aplicado com pragmatismo; preferir composição; interfaces pequenas; evitar abstração prematura e dependências desnecessárias (justificar toda dependência externa em ADR).
- Relógio e gerador de IDs injetáveis (`Clock`, `IDGenerator`) para testes determinísticos.
- Artefatos de build (`dist/`, código gerado de ORM etc.) **não** são versionados.
- Definição de Pronto (DoD): código + testes + documentação OpenAPI + migração reversível + métricas/logs + atualização de ADR/README quando aplicável.

---

## 14. Plano de entrega por fases

**Fase 0 — Fundação (descoberta e esqueleto):** glossário, mapa de contexto, ADRs iniciais, OpenAPI v1 inicial, esqueleto do monólito modular, `platform` (config, slog, HTTP, OTel, banco PostgreSQL/SQLite, outbox), CI completo, ambiente de homologação.

**Fase 1 — MVP vendável:** identity + tenancy (dono e caixa), customers, ledger completo (compra, pagamento, estorno, FIFO, saldo projetado, limite), PWA offline-first com sync, painel (total a receber, inadimplentes, recebido no período), cobrança e comprovante por link de WhatsApp, Pix copia-e-cola, exportação CSV, LGPD básico.

**Fase 2 — Escala e automação:** MongoDB em produção para auditoria/mensagens/read models, régua de cobrança agendada com WhatsApp Business API, PSP Pix com baixa automática, assistente de IA (consultas e sugestões), aging e relatórios PDF, entrada por áudio.

**Fase 3 — Inteligência e ecossistema:** score de risco explicável, previsão de recebíveis, painel para associações/prefeituras (dados agregados e anonimizados), multi-loja por dono, API pública para parceiros.

Para cada fase, entregar: backlog em histórias com critérios de aceite (Gherkin para regras do ledger), estimativa relativa, riscos e mitigação.

---

## 15. Formato da sua resposta

Trabalhe em etapas e pare para validação ao fim de cada uma:
1. **Entendimento**: resumo do problema, premissas marcadas, perguntas em aberto (no máximo 10, priorizadas).
2. **Arquitetura**: ADRs, diagramas C4 e ER, contrato OpenAPI inicial, estrutura de diretórios.
3. **Fundação**: código da Fase 0 completo, compilando, com testes e CI.
4. **MVP**: implementação incremental da Fase 1, módulo a módulo, começando pelo `ledger` (domínio + testes primeiro).
5. Ao final de cada etapa: lista do que foi entregue, como executar, como testar, débitos técnicos conhecidos e próximos passos.
