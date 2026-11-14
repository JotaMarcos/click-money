# ADR 0004 — Dinheiro como inteiro de centavos

- Status: Aceita
- Data: 2026-11-13

## Contexto

Ponto flutuante binário não representa valores como 0,10 com exatidão. Em um livro-razão, um centavo de diferença vira disputa com o cliente.

## Decisão

- O value object `Money` guarda `int64` de centavos e a moeda (`BRL`, única suportada). Vive em `internal/platform/kernel`.
- `float32` e `float64` são proibidos em cálculo financeiro, no backend e no frontend. No Go, uma regra de lint (`forbidigo`) barra os tipos nos pacotes `ledger`, `customers`, `collections` e `reporting`. No TypeScript, valores trafegam como `number` inteiro de centavos e só viram texto na formatação.
- Na API e no banco o campo é sempre `amount_cents` (inteiro). O banco usa `BIGINT`.
- O parse aceita o formato brasileiro ("1.234,56", "12,5", "12"): no máximo duas casas decimais, sem sinal, com limite superior de R$ 1.000.000,00 por lançamento **[PREMISSA]**. Entrada ambígua é rejeitada, nunca "corrigida".
- Soma e subtração verificam overflow e devolvem erro.
- Onde houver divisão (juros de mora, quando habilitados) o arredondamento é bancário (meio para o par), implementado sobre inteiros.

## Alternativas

- **`decimal` de biblioteca (ex.: shopspring/decimal)**: descartado. Dependência a mais para um problema que centavos inteiros resolvem; BRL tem exatamente duas casas.
- **`NUMERIC` no banco e `big.Rat` no Go**: descartado. Mais lento e sem benefício para uma moeda só.

## Consequências

- Não existe erro de arredondamento em soma, subtração e alocação.
- O limite de `int64` (cerca de 92 quatrilhões de reais) é inalcançável na prática; o overflow é verificado mesmo assim.
- Suportar outra moeda no futuro exigirá tratar casas decimais por moeda no `Money`.
