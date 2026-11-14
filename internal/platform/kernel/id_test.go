package kernel

import (
	"errors"
	"testing"
	"time"
)

const sampleV7 = "01933b7e-8c1a-7c3e-9f4b-2a6d5e8f1b0c"

func TestParseID(t *testing.T) {
	t.Parallel()
	id, err := ParseID(sampleV7)
	if err != nil {
		t.Fatal(err)
	}
	if id.String() != sampleV7 {
		t.Fatalf("String = %s", id)
	}
	if !id.IsV7() || id.Version() != 7 || id.IsZero() {
		t.Fatalf("versão %d, v7 %v", id.Version(), id.IsV7())
	}
	upper, err := ParseID("01933B7E-8C1A-7C3E-9F4B-2A6D5E8F1B0C")
	if err != nil || upper != id {
		t.Fatalf("maiúsculas: %v, %v", upper, err)
	}

	for _, in := range []string{
		"", "01933b7e8c1a7c3e9f4b2a6d5e8f1b0c", "01933b7e-8c1a-7c3e-9f4b-2a6d5e8f1b0",
		"01933b7e-8c1a-7c3e-9f4b-2a6d5e8f1b0g", "01933b7e_8c1a_7c3e_9f4b_2a6d5e8f1b0c",
		"{01933b7e-8c1a-7c3e-9f4b-2a6d5e8f1b0c}",
	} {
		if _, err := ParseID(in); !errors.Is(err, ErrInvalidID) {
			t.Errorf("ParseID(%q) erro = %v, quero ErrInvalidID", in, err)
		}
	}
}

func TestIDVersionAndTime(t *testing.T) {
	t.Parallel()
	v4, _ := ParseID("f47ac10b-58cc-4372-a567-0e02b2c3d479")
	if v4.IsV7() || v4.Version() != 4 {
		t.Error("UUIDv4 identificado como v7")
	}
	// Versão 7 com variante errada não é um UUIDv7 válido.
	badVariant, _ := ParseID("01933b7e-8c1a-7c3e-1f4b-2a6d5e8f1b0c")
	if badVariant.IsV7() {
		t.Error("variante inválida aceita como v7")
	}

	id, _ := ParseID(sampleV7)
	want := time.UnixMilli(0x01933b7e8c1a).UTC()
	if !id.Time().Equal(want) {
		t.Errorf("Time = %s, quero %s", id.Time(), want)
	}
}

func TestIDCompareAndText(t *testing.T) {
	t.Parallel()
	older, _ := ParseID("01933b7e-8c1a-7c3e-9f4b-2a6d5e8f1b0c")
	newer, _ := ParseID("01933b7e-8c1b-7000-8000-000000000000")
	if older.Compare(newer) != -1 || newer.Compare(older) != 1 || older.Compare(older) != 0 {
		t.Error("Compare não segue a ordem temporal")
	}

	text, err := older.MarshalText()
	if err != nil || string(text) != sampleV7 {
		t.Fatalf("MarshalText = %s, %v", text, err)
	}
	var back ID
	if err := back.UnmarshalText(text); err != nil || back != older {
		t.Fatalf("UnmarshalText = %v, %v", back, err)
	}
	if err := back.UnmarshalText([]byte("x")); err == nil {
		t.Fatal("UnmarshalText deveria recusar texto inválido")
	}
}

func TestTenantID(t *testing.T) {
	t.Parallel()
	tenant, err := ParseTenantID(sampleV7)
	if err != nil || tenant.String() != sampleV7 || tenant.IsZero() {
		t.Fatalf("ParseTenantID = %v, %v", tenant, err)
	}
	if _, err := ParseTenantID("nada"); !errors.Is(err, ErrInvalidID) {
		t.Fatalf("erro = %v", err)
	}
	if !(TenantID{}).IsZero() {
		t.Fatal("TenantID zero deveria ser nulo")
	}
}

func TestErrorSemantics(t *testing.T) {
	t.Parallel()
	base := NewError("CUSTOMER_HAS_DEBT", "Cliente com dívida.")
	other := NewError("CUSTOMER_HAS_DEBT", "Outra mensagem.")
	wrapped := errors.Join(errors.New("contexto"), other)

	if !errors.Is(wrapped, base) {
		t.Error("erros com o mesmo código deveriam casar em errors.Is")
	}
	if errors.Is(base, ErrInvalidMoney) || errors.Is(base, errors.New("x")) {
		t.Error("códigos diferentes não podem casar")
	}
	if CodeOf(wrapped) != "CUSTOMER_HAS_DEBT" || CodeOf(errors.New("x")) != "" {
		t.Error("CodeOf incorreto")
	}
	field := NewFieldError("amount_cents", "Informe um valor.")
	if field.Error() != "VALIDATION_FAILED (amount_cents): Informe um valor." {
		t.Errorf("Error() = %s", field.Error())
	}
	if base.Error() != "CUSTOMER_HAS_DEBT: Cliente com dívida." {
		t.Errorf("Error() = %s", base.Error())
	}
}
