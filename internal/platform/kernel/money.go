package kernel

import (
	"errors"
	"math"
	"strconv"
	"strings"
)

// Erros de Money.
var (
	// ErrInvalidMoney indica texto que não é um valor em reais válido.
	ErrInvalidMoney = NewError(CodeValidationFailed, "Valor inválido. Use o formato 1.234,56.")
	// ErrMoneyOverflow indica operação cujo resultado não cabe em int64.
	ErrMoneyOverflow = errors.New("kernel: overflow em operação monetária")
)

// maxIntegerDigits limita a parte inteira aceita no parse. Quinze dígitos de
// reais vezes cem ainda cabem com folga em int64.
const maxIntegerDigits = 15

// Money é um valor em reais (BRL) guardado como inteiro de centavos.
//
// Não existe conversão de ou para ponto flutuante: todo cálculo financeiro é
// feito sobre inteiros. O valor zero de Money é R$ 0,00.
type Money struct {
	cents int64
}

// FromCents cria um Money a partir de centavos.
func FromCents(cents int64) Money { return Money{cents: cents} }

// Cents devolve o valor em centavos.
func (m Money) Cents() int64 { return m.cents }

// IsZero informa se o valor é R$ 0,00.
func (m Money) IsZero() bool { return m.cents == 0 }

// IsPositive informa se o valor é maior que zero.
func (m Money) IsPositive() bool { return m.cents > 0 }

// IsNegative informa se o valor é menor que zero.
func (m Money) IsNegative() bool { return m.cents < 0 }

// Cmp devolve -1, 0 ou +1 conforme m seja menor, igual ou maior que o.
func (m Money) Cmp(o Money) int {
	switch {
	case m.cents < o.cents:
		return -1
	case m.cents > o.cents:
		return 1
	default:
		return 0
	}
}

// Add soma dois valores e devolve ErrMoneyOverflow se o resultado não couber.
func (m Money) Add(o Money) (Money, error) {
	if (o.cents > 0 && m.cents > math.MaxInt64-o.cents) ||
		(o.cents < 0 && m.cents < math.MinInt64-o.cents) {
		return Money{}, ErrMoneyOverflow
	}
	return Money{cents: m.cents + o.cents}, nil
}

// Sub subtrai o de m e devolve ErrMoneyOverflow se o resultado não couber.
func (m Money) Sub(o Money) (Money, error) {
	if (o.cents < 0 && m.cents > math.MaxInt64+o.cents) ||
		(o.cents > 0 && m.cents < math.MinInt64+o.cents) {
		return Money{}, ErrMoneyOverflow
	}
	return Money{cents: m.cents - o.cents}, nil
}

// Min devolve o menor entre m e o.
func (m Money) Min(o Money) Money {
	if o.cents < m.cents {
		return o
	}
	return m
}

// String formata o valor no padrão brasileiro, sem símbolo: "1.234,56".
func (m Money) String() string {
	// Trabalha com o módulo em uint64 para que math.MinInt64 não estoure.
	abs := uint64(m.cents)
	if m.cents < 0 {
		abs = -abs
	}
	integer := strconv.FormatUint(abs/100, 10)
	frac := abs % 100

	var b strings.Builder
	if m.cents < 0 {
		b.WriteByte('-')
	}
	head := len(integer) % 3
	if head > 0 {
		b.WriteString(integer[:head])
	}
	for i := head; i < len(integer); i += 3 {
		if i > 0 {
			b.WriteByte('.')
		}
		b.WriteString(integer[i : i+3])
	}
	b.WriteByte(',')
	if frac < 10 {
		b.WriteByte('0')
	}
	b.WriteString(strconv.FormatUint(frac, 10))
	return b.String()
}

// Format devolve o valor com o símbolo da moeda: "R$ 1.234,56".
func (m Money) Format() string { return "R$ " + m.String() }

// ParseBRL converte um texto em formato brasileiro para Money.
//
// Aceita "12", "12,5", "12,50", "1234,56" e "1.234,56", com "R$" opcional na
// frente. Recusa sinal, mais de duas casas decimais, separador de milhar fora
// de posição e qualquer formato ambíguo, como "12.34" ou "1,234.56": uma
// entrada duvidosa nunca é "corrigida".
func ParseBRL(s string) (Money, error) {
	s = strings.TrimSpace(s)
	if rest, ok := strings.CutPrefix(s, "R$"); ok {
		s = strings.TrimSpace(rest)
	}
	if s == "" {
		return Money{}, ErrInvalidMoney
	}

	intPart, fracPart, hasComma := strings.Cut(s, ",")
	if hasComma && (fracPart == "" || len(fracPart) > 2 || !allDigits(fracPart)) {
		return Money{}, ErrInvalidMoney
	}

	digits, ok := integerDigits(intPart)
	if !ok || len(digits) > maxIntegerDigits {
		return Money{}, ErrInvalidMoney
	}

	reais, err := strconv.ParseInt(digits, 10, 64)
	if err != nil {
		return Money{}, ErrInvalidMoney
	}

	var cents int64
	if hasComma {
		cents, err = strconv.ParseInt(fracPart, 10, 64)
		if err != nil {
			return Money{}, ErrInvalidMoney
		}
		if len(fracPart) == 1 {
			cents *= 10
		}
	}
	return Money{cents: reais*100 + cents}, nil
}

// integerDigits valida a parte inteira e devolve só os dígitos. Com ponto, os
// grupos precisam ter exatamente três dígitos depois do primeiro, e o
// primeiro não pode começar com zero.
func integerDigits(s string) (string, bool) {
	if s == "" {
		return "", false
	}
	if !strings.Contains(s, ".") {
		return s, allDigits(s)
	}
	groups := strings.Split(s, ".")
	first := groups[0]
	if first == "" || len(first) > 3 || first[0] == '0' || !allDigits(first) {
		return "", false
	}
	for _, g := range groups[1:] {
		if len(g) != 3 || !allDigits(g) {
			return "", false
		}
	}
	return strings.Join(groups, ""), true
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
