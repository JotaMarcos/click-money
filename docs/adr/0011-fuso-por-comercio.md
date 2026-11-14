# ADR 0011 — Fuso horário por comércio e datas civis

- Status: Aceita
- Data: 2026-11-13

## Contexto

O Amazonas tem dois fusos: `America/Manaus` (UTC−4) e, no sudoeste do estado, `America/Eirunepe` (UTC−5). Um lançamento às 21h em Manaus acontece à 1h do dia seguinte em UTC. Se "hoje" for calculado em UTC, a compra cai no dia errado, o fechamento de caixa não bate e a cobrança sai fora do horário permitido.

## Decisão

- Cada comércio tem um fuso IANA em `tenants.timezone`, com padrão `America/Manaus`. São aceitos os fusos brasileiros.
- **Instantes** são gravados em UTC (`timestamptz`): `recorded_at`, `created_at`, `updated_at`.
- **Datas civis** são gravadas como `date`: `occurred_on`, data do fechamento de caixa, dia preferencial de pagamento. O value object `CivilDate` não tem fuso nem hora.
- "Hoje", "esta semana", a validação de data futura, o limite de retroatividade e a janela de cobrança (8h–20h) são sempre calculados no fuso do comércio. Nenhum código de domínio chama `time.Now()`: o instante vem da porta `Clock` e o fuso vem do tenant.
- Uma regra de lint proíbe `time.Now` e `time.Local` fora de `internal/platform/clock`.
- Os binários embutem a base de fusos (`time/tzdata`), porque imagens `distroless` e `scratch` não a trazem.
- No PWA, a data civil é calculada com `Intl.DateTimeFormat` no fuso do comércio, não no fuso do aparelho.
- A API recebe e devolve datas civis como `YYYY-MM-DD` e instantes como RFC 3339 em UTC.

## Alternativas

- **Um fuso fixo para todo o sistema**: descartado. Erra para os municípios em UTC−5 e impede vender fora do estado.
- **Gravar hora local com deslocamento**: descartado. Complica comparação e ordenação sem ganho.
- **Derivar a data civil de `recorded_at` na leitura**: descartado. Lançamentos offline e retroativos têm data civil diferente do instante de registro.

## Consequências

- O fechamento de caixa e os relatórios por dia agrupam por `occurred_on`, sem conversão.
- Mudar o fuso de um comércio não reescreve o histórico: as datas civis já gravadas permanecem.
- Testes obrigatórios: 21h em `America/Manaus` e 20h em `America/Eirunepe` ficam com a data local do dia.
