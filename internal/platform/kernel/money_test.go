package kernel

import (
	"errors"
	"math"
	"testing"
)

func TestParseBRL(t *testing.T) {
	t.Parallel()

	accepted := []struct {
		in   string
		want int64
	}{
		{"12", 1200},
		{"12,5", 1250},
		{"12,50", 1250},
		{"1.234,56", 123456},
		{"1234,56", 123456},
		{"0,01", 1},
		{"0", 0},
		{"0,00", 0},
		{"1.234", 123400},
		{"1.234.567,89", 123456789},
		{"  25,50  ", 2550},
		{"R$ 25,50", 2550},
		{"R$25,50", 2550},
		{"007", 700},
		{"999999999999999,99", 99999999999999999},
	}
	for _, tc := range accepted {
		t.Run("aceita "+tc.in, func(t *testing.T) {
			t.Parallel()
			got, err := ParseBRL(tc.in)
			if err != nil {
				t.Fatalf("ParseBRL(%q) erro inesperado: %v", tc.in, err)
			}
			if got.Cents() != tc.want {
				t.Fatalf("ParseBRL(%q) = %d centavos, quero %d", tc.in, got.Cents(), tc.want)
			}
		})
	}

	rejected := []string{
		"", "   ", "R$", "abc", "12,345", "1,234.56", "-5,00", "+5,00", "12.34",
		"12,", ",50", "1.23,00", "1..234", ".123", "0.123,00", "12,5a", "1 234,56",
		"1,2,3", "12.345.6", "1234.567", "9999999999999999", "１２",
	}
	for _, in := range rejected {
		t.Run("recusa "+in, func(t *testing.T) {
			t.Parallel()
			if got, err := ParseBRL(in); !errors.Is(err, ErrInvalidMoney) {
				t.Fatalf("ParseBRL(%q) = %v, %v; quero ErrInvalidMoney", in, got, err)
			}
		})
	}
}

func TestMoneyString(t *testing.T) {
	t.Parallel()
	cases := []struct {
		cents int64
		want  string
	}{
		{0, "0,00"},
		{1, "0,01"},
		{10, "0,10"},
		{100, "1,00"},
		{2550, "25,50"},
		{99999, "999,99"},
		{100000, "1.000,00"},
		{123456789, "1.234.567,89"},
		{-2550, "-25,50"},
		{-5, "-0,05"},
		{math.MaxInt64, "92.233.720.368.547.758,07"},
		{math.MinInt64, "-92.233.720.368.547.758,08"},
	}
	for _, tc := range cases {
		if got := FromCents(tc.cents).String(); got != tc.want {
			t.Errorf("FromCents(%d).String() = %q, quero %q", tc.cents, got, tc.want)
		}
	}
	if got := FromCents(123456).Format(); got != "R$ 1.234,56" {
		t.Errorf("Format() = %q", got)
	}
}

func TestMoneyArithmetic(t *testing.T) {
	t.Parallel()

	sum, err := FromCents(1050).Add(FromCents(250))
	if err != nil || sum.Cents() != 1300 {
		t.Fatalf("Add = %v, %v", sum, err)
	}
	diff, err := FromCents(1050).Sub(FromCents(2000))
	if err != nil || diff.Cents() != -950 {
		t.Fatalf("Sub = %v, %v", diff, err)
	}

	overflows := []struct {
		name string
		op   func() (Money, error)
	}{
		{"soma acima do máximo", func() (Money, error) { return FromCents(math.MaxInt64).Add(FromCents(1)) }},
		{"soma abaixo do mínimo", func() (Money, error) { return FromCents(math.MinInt64).Add(FromCents(-1)) }},
		{"subtração acima do máximo", func() (Money, error) { return FromCents(math.MaxInt64).Sub(FromCents(-1)) }},
		{"subtração abaixo do mínimo", func() (Money, error) { return FromCents(math.MinInt64).Sub(FromCents(1)) }},
		{"subtração do mínimo", func() (Money, error) { return FromCents(0).Sub(FromCents(math.MinInt64)) }},
	}
	for _, tc := range overflows {
		if _, err := tc.op(); !errors.Is(err, ErrMoneyOverflow) {
			t.Errorf("%s: erro = %v, quero ErrMoneyOverflow", tc.name, err)
		}
	}

	// Os extremos válidos não podem ser confundidos com overflow.
	if _, err := FromCents(math.MaxInt64).Add(FromCents(0)); err != nil {
		t.Errorf("MaxInt64 + 0: %v", err)
	}
	if _, err := FromCents(math.MinInt64).Sub(FromCents(0)); err != nil {
		t.Errorf("MinInt64 - 0: %v", err)
	}
	if got, err := FromCents(-1).Sub(FromCents(math.MaxInt64)); err != nil || got.Cents() != math.MinInt64 {
		t.Errorf("-1 - MaxInt64 = %v, %v", got, err)
	}
}

func TestMoneyPredicates(t *testing.T) {
	t.Parallel()
	zero, pos, neg := FromCents(0), FromCents(5), FromCents(-5)
	if !zero.IsZero() || zero.IsPositive() || zero.IsNegative() {
		t.Error("zero classificado errado")
	}
	if !pos.IsPositive() || pos.IsZero() || pos.IsNegative() {
		t.Error("positivo classificado errado")
	}
	if !neg.IsNegative() || neg.IsZero() || neg.IsPositive() {
		t.Error("negativo classificado errado")
	}
	if pos.Cmp(neg) != 1 || neg.Cmp(pos) != -1 || pos.Cmp(pos) != 0 {
		t.Error("Cmp incorreto")
	}
	if pos.Min(neg) != neg || neg.Min(pos) != neg {
		t.Error("Min incorreto")
	}
}

// FuzzParseBRL verifica que o parse nunca entra em pânico e que todo valor
// aceito sobrevive a formatar e converter de novo.
func FuzzParseBRL(f *testing.F) {
	for _, seed := range []string{"12", "12,5", "1.234,56", "R$ 0,01", "-1", "1,234.56", "", "999.999.999.999.999,99"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, in string) {
		m, err := ParseBRL(in)
		if err != nil {
			return
		}
		if m.IsNegative() {
			t.Fatalf("ParseBRL(%q) devolveu valor negativo: %d", in, m.Cents())
		}
		again, err := ParseBRL(m.String())
		if err != nil {
			t.Fatalf("ParseBRL(%q) aceito, mas a forma canônica %q foi recusada: %v", in, m.String(), err)
		}
		if again != m {
			t.Fatalf("ida e volta divergiu: %q -> %d -> %q -> %d", in, m.Cents(), m.String(), again.Cents())
		}
	})
}
