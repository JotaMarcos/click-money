# Backlog — Fase 0 (Fundação)

Objetivo: um esqueleto que compila, testa, empacota e sobe, sobre o qual a Fase 1 constrói o produto. Não entrega funcionalidade ao comerciante.

Estimativa em pontos relativos (1, 2, 3, 5, 8). Prioridade: P1 bloqueia a Fase 1; P2 pode terminar em paralelo com o início dela.

## Histórias

### F0-01 — Esqueleto do repositório (P1, 3 pontos)

Como pessoa desenvolvedora, quero clonar o repositório e rodar o sistema com um comando, para começar a contribuir sem instalar banco nem serviços.

Critérios de aceite:

- `make dev` sobe a API em `localhost:8080` usando SQLite, sem Docker.
- `make test`, `make lint`, `make generate` e `make build` funcionam em uma máquina só com Go e Node instalados.
- A estrutura segue [estrutura-de-diretorios.md](../architecture/estrutura-de-diretorios.md).
- Um teste de arquitetura falha quando `domain` importa `adapters` ou infraestrutura, e quando um módulo importa o interior de outro.

### F0-02 — Tipos compartilhados (P1, 5 pontos)

Como pessoa desenvolvedora, quero `Money`, `CivilDate`, `Phone` e `TenantID` prontos e testados, para não reimplementar regras básicas em cada módulo.

Critérios de aceite:

```gherkin
Funcionalidade: Parse de valores em formato brasileiro

  Esquema do Cenário: Valores aceitos
    Quando eu converto o texto "<texto>"
    Então o resultado é <centavos> centavos

    Exemplos:
      | texto     | centavos |
      | 12        | 1200     |
      | 12,5      | 1250     |
      | 12,50     | 1250     |
      | 1.234,56  | 123456   |
      | 0,01      | 1        |

  Esquema do Cenário: Valores rejeitados
    Quando eu converto o texto "<texto>"
    Então recebo um erro de valor inválido

    Exemplos:
      | texto      |
      | 12,345     |
      | 1,234.56   |
      | -5,00      |
      | 12.34      |
      | abc        |
      |            |

Funcionalidade: Data civil no fuso do comércio

  Cenário: Noite em Manaus
    Dado um comércio no fuso "America/Manaus"
    E que o instante atual é "2026-11-14T01:00:00Z"
    Quando eu calculo a data de hoje do comércio
    Então a data é "2026-11-13"

  Cenário: Noite em Eirunepé
    Dado um comércio no fuso "America/Eirunepe"
    E que o instante atual é "2026-11-14T01:00:00Z"
    Quando eu calculo a data de hoje do comércio
    Então a data é "2026-11-13"

Funcionalidade: Normalização de telefone

  Esquema do Cenário: Telefones aceitos
    Quando eu normalizo o telefone "<entrada>"
    Então o resultado é "<e164>"

    Exemplos:
      | entrada           | e164            |
      | (92) 99123-4567   | +5592991234567  |
      | 92991234567       | +5592991234567  |
      | +55 97 3412-1234  | +559734121234   |
      | 11 98888-7777     | +5511988887777  |
```

- Soma e subtração de `Money` devolvem erro em overflow.
- Há teste de fuzzing para o parse de dinheiro e de telefone: nunca entra em pânico e, quando aceita, formatar e converter de novo devolve o mesmo valor.
- `float32` e `float64` são barrados por lint nos pacotes financeiros.

### F0-03 — Configuração e logs (P1, 3 pontos)

Como pessoa de operação, quero que o serviço recuse subir com configuração inválida e registre logs estruturados sem dados pessoais, para diagnosticar problemas sem vazar informação.

Critérios de aceite:

- Variável obrigatória ausente ou inválida encerra o processo na inicialização, com mensagem que nomeia a variável.
- Em produção os logs são JSON com `request_id`, `tenant_id` e `trace_id`.
- Telefone, senha, token e cookie nunca aparecem em log; há teste que comprova a redação.
- `.env` só é lido quando o ambiente é de desenvolvimento.

### F0-04 — Servidor HTTP e contrato (P1, 5 pontos)

Como pessoa desenvolvedora do PWA, quero uma API que siga o contrato OpenAPI desde o primeiro endpoint, para gerar o cliente e confiar nos tipos.

Critérios de aceite:

- `make generate` produz o servidor Go e o cliente TypeScript a partir de `api/openapi.yaml`; o CI falha se o código gerado estiver desatualizado.
- `GET /livez` responde 200 enquanto o processo vive.
- `GET /readyz` responde 503 sem o banco relacional, e 200 com `status: degraded` quando só o banco de documentos está fora.
- Erros saem como `application/problem+json` com `code` e `request_id`.
- Origem não permitida recebe 403, não 500.
- Corpo acima do limite recebe 413.
- O servidor define `ReadHeaderTimeout`, `ReadTimeout`, `WriteTimeout` e `IdleTimeout`, e encerra de forma graciosa ao receber sinal.
- Respostas trazem os cabeçalhos de segurança definidos pelo time de segurança.

### F0-05 — Banco, Unit of Work e migrações (P1, 8 pontos)

Como pessoa desenvolvedora, quero abrir uma transação por caso de uso com o tenant já definido, para que o isolamento não dependa de cada query.

Critérios de aceite:

- `cmd/migrate up` e `down` funcionam em PostgreSQL e em SQLite.
- Toda migração é reversível; o teste aplica, reverte e aplica de novo em PostgreSQL real.
- A API não executa migração ao iniciar.
- Em PostgreSQL, a transação aberta pelo Unit of Work executa `SET LOCAL app.tenant_id`.

```gherkin
Funcionalidade: Isolamento entre comércios

  Cenário: Leitura cruzada
    Dado os comércios "A" e "B", cada um com um registro
    Quando uma transação do comércio "A" consulta a tabela sem filtro
    Então ela enxerga apenas o registro do comércio "A"

  Cenário: Escrita cruzada
    Dado uma transação do comércio "A"
    Quando ela tenta inserir um registro com o tenant do comércio "B"
    Então o banco rejeita a operação

  Cenário: Transação sem tenant
    Dado uma transação sem tenant definido
    Quando ela consulta uma tabela de negócio
    Então o banco rejeita a operação
```

### F0-06 — Outbox e worker (P1, 5 pontos)

Como pessoa desenvolvedora, quero gravar eventos junto com a mudança e tê-los entregues depois, para que integrações não travem a escrita.

Critérios de aceite:

```gherkin
Funcionalidade: Outbox transacional

  Cenário: Evento gravado com a mudança
    Quando um caso de uso grava uma mudança e um evento na mesma transação
    E a transação é confirmada
    Então o worker entrega o evento ao consumidor

  Cenário: Transação desfeita
    Quando a transação é desfeita
    Então nenhum evento é entregue

  Cenário: Consumidor fora do ar
    Dado que o consumidor falha
    Quando o worker tenta entregar o evento
    Então o evento permanece na fila
    E a próxima tentativa é agendada com intervalo crescente

  Cenário: Entrega repetida
    Dado um evento já processado pelo consumidor
    Quando o worker o entrega de novo
    Então o consumidor não repete o efeito
```

- Tamanho e idade da fila são expostos como métricas.

### F0-07 — Observabilidade (P2, 3 pontos)

Como pessoa de operação, quero traces, métricas e perfis desde o início, para medir antes de otimizar.

Critérios de aceite:

- Cada requisição gera um trace; o `trace_id` aparece no log da mesma requisição.
- `/metrics` expõe taxa, erros e duração por rota.
- `pprof` só responde em porta interna ou com credencial.

### F0-08 — Empacotamento e CI (P1, 5 pontos)

Como time, queremos um pipeline que barre código com defeito conhecido, para manter a branch principal sempre publicável.

Critérios de aceite:

- Imagem multi-stage, `distroless`, usuário não-root, binário com `CGO_ENABLED=0`.
- Pipeline: lint, testes com `-race`, integração com testcontainers, `govulncheck`, `gosec`, varredura de segredos, build, scan da imagem, SBOM.
- O pipeline falha se a cobertura ficar abaixo de 90% em `ledger/domain` e `ledger/app` ou de 75% no geral (os pisos de `ledger` passam a valer quando o módulo existir).
- `deploy/compose.yaml` sobe PostgreSQL, MongoDB em replica set e a pilha de observabilidade.

### F0-09 — Ambiente de homologação (P2, 8 pontos)

Como time, queremos um ambiente igual ao de produção, para validar cada entrega antes de publicar.

Critérios de aceite:

- Infraestrutura descrita em OpenTofu, em região brasileira.
- Deploy em homologação automático a partir da branch principal, seguido de smoke test em `/readyz`.
- Segredos só no gerenciador de segredos.

Depende da confirmação do provedor de nuvem ([ADR 0013](../adr/0013-hospedagem.md)).

### F0-10 — Esqueleto do PWA (P1, 5 pontos)

Como pessoa desenvolvedora do front, quero o projeto React com service worker, base local e orçamento de bundle verificados, para construir as telas da Fase 1 sobre terreno firme.

Critérios de aceite:

- O PWA é instalável e abre sem rede depois da primeira visita.
- O CI falha se o bundle inicial passar de 200 KB gzip.
- Textos vêm de arquivos de mensagens; não há texto de interface no código.
- A página inicial passa na verificação automática de acessibilidade (axe) sem violações.

## Riscos

| Risco | Impacto | Mitigação |
| --- | --- | --- |
| `oapi-codegen` não gerar a partir de OpenAPI 3.1 | Atrasa F0-04 | Contrato em subconjunto conservador; plano B no ADR 0009 |
| Divergência de comportamento entre SQLite e PostgreSQL | Defeito que só aparece em produção | Suíte de contrato dos repositórios rodando nos dois; integração obrigatória no CI |
| Provedor de nuvem indefinido | Bloqueia F0-09 | Demais histórias não dependem dele |
| Orçamento de 200 KB do bundle | Retrabalho no front | Medição no CI desde o esqueleto; Preact como plano B |

Total: 50 pontos.
