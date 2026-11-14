package kernel

import (
	"errors"
	"testing"
)

func TestParsePhone(t *testing.T) {
	t.Parallel()

	accepted := []struct{ in, want string }{
		{"(92) 99123-4567", "+5592991234567"},
		{"92991234567", "+5592991234567"},
		{"92 99123.4567", "+5592991234567"},
		{"092 99123-4567", "+5592991234567"},
		{"5592991234567", "+5592991234567"},
		{"+55 92 99123-4567", "+5592991234567"},
		{"+55 97 3412-1234", "+559734121234"},
		{"(97) 3412-1234", "+559734121234"},
		{"11 98888-7777", "+5511988887777"},
		{"+1 (415) 555-2671", "+14155552671"},
		{"+351 912 345 678", "+351912345678"},
	}
	for _, tc := range accepted {
		t.Run("aceita "+tc.in, func(t *testing.T) {
			t.Parallel()
			got, err := ParsePhone(tc.in)
			if err != nil {
				t.Fatalf("ParsePhone(%q) erro inesperado: %v", tc.in, err)
			}
			if got.String() != tc.want {
				t.Fatalf("ParsePhone(%q) = %s, quero %s", tc.in, got, tc.want)
			}
		})
	}

	rejected := []string{
		"", "   ", "abc", "99123-4567", "(00) 99123-4567", "(20) 99123-4567",
		"(92) 89123-4567", "(92) 9123-4567", "(92) 991234-56789", "+", "+0123456789",
		"+55 92 1234", "+1234", "92 99123-4567 ramal 2", "+55 (92) 6123-4567",
	}
	for _, in := range rejected {
		t.Run("recusa "+in, func(t *testing.T) {
			t.Parallel()
			if got, err := ParsePhone(in); !errors.Is(err, ErrInvalidPhone) {
				t.Fatalf("ParsePhone(%q) = %v, %v; quero ErrInvalidPhone", in, got, err)
			}
		})
	}
}

func TestPhoneAttributes(t *testing.T) {
	t.Parallel()
	manaus, _ := ParsePhone("(92) 99123-4567")
	saoPaulo, _ := ParsePhone("(11) 98888-7777")
	foreign, _ := ParsePhone("+14155552671")

	if manaus.AreaCode() != "92" || !manaus.IsNorthRegion() || !manaus.IsBrazilian() {
		t.Errorf("Manaus: DDD %q, norte %v", manaus.AreaCode(), manaus.IsNorthRegion())
	}
	if saoPaulo.AreaCode() != "11" || saoPaulo.IsNorthRegion() {
		t.Errorf("São Paulo: DDD %q, norte %v", saoPaulo.AreaCode(), saoPaulo.IsNorthRegion())
	}
	if foreign.AreaCode() != "" || foreign.IsNorthRegion() || foreign.IsBrazilian() {
		t.Error("telefone estrangeiro tratado como brasileiro")
	}
	if manaus.WhatsAppNumber() != "5592991234567" {
		t.Errorf("WhatsAppNumber = %s", manaus.WhatsAppNumber())
	}
	if got := manaus.Masked(); got != "+5592*******67" {
		t.Errorf("Masked = %s", got)
	}
	if !(Phone{}).IsZero() || manaus.IsZero() || (Phone{}).Masked() != "" {
		t.Error("valor zero tratado errado")
	}
}

// FuzzParsePhone verifica que o parse nunca entra em pânico e que todo
// telefone aceito já está em forma canônica.
func FuzzParsePhone(f *testing.F) {
	for _, seed := range []string{"(92) 99123-4567", "+55 97 3412-1234", "+14155552671", "092991234567", "", "+"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, in string) {
		p, err := ParsePhone(in)
		if err != nil {
			return
		}
		s := p.String()
		if len(s) < 9 || len(s) > 16 || s[0] != '+' {
			t.Fatalf("ParsePhone(%q) = %q fora do formato E.164", in, s)
		}
		again, err := ParsePhone(s)
		if err != nil || again != p {
			t.Fatalf("forma canônica %q não é estável: %v, %v", s, again, err)
		}
	})
}
