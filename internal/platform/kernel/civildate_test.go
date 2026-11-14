package kernel

import (
	"errors"
	"testing"
	"time"
	_ "time/tzdata" // os testes não podem depender da base de fusos da máquina
)

func mustLocation(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatalf("fuso %s: %v", name, err)
	}
	return loc
}

func mustDate(t *testing.T, s string) CivilDate {
	t.Helper()
	d, err := ParseCivilDate(s)
	if err != nil {
		t.Fatalf("data %s: %v", s, err)
	}
	return d
}

func TestCivilDateOfUsesTenantTimezone(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		zone    string
		instant string
		want    string
	}{
		{"21h em Manaus fica no mesmo dia", "America/Manaus", "2026-11-14T01:00:00Z", "2026-11-13"},
		{"20h em Eirunepé fica no mesmo dia", "America/Eirunepe", "2026-11-14T01:00:00Z", "2026-11-13"},
		{"23h59 em Manaus", "America/Manaus", "2026-11-14T03:59:59Z", "2026-11-13"},
		{"meia-noite em Manaus vira o dia", "America/Manaus", "2026-11-14T04:00:00Z", "2026-11-14"},
		{"meia-noite em Manaus ainda é ontem em Eirunepé", "America/Eirunepe", "2026-11-14T04:00:00Z", "2026-11-13"},
		{"meia-noite em Eirunepé vira o dia", "America/Eirunepe", "2026-11-14T05:00:00Z", "2026-11-14"},
		{"virada de ano em Manaus", "America/Manaus", "2027-01-01T03:30:00Z", "2026-12-31"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			instant, err := time.Parse(time.RFC3339, tc.instant)
			if err != nil {
				t.Fatal(err)
			}
			got := CivilDateOf(instant, mustLocation(t, tc.zone))
			if got.String() != tc.want {
				t.Fatalf("CivilDateOf = %s, quero %s", got, tc.want)
			}
		})
	}
}

func TestParseCivilDate(t *testing.T) {
	t.Parallel()
	d := mustDate(t, "2026-11-13")
	if d.Year() != 2026 || d.Month() != time.November || d.Day() != 13 {
		t.Fatalf("componentes errados: %v", d)
	}
	if d.FormatBR() != "13/11/2026" {
		t.Fatalf("FormatBR = %s", d.FormatBR())
	}
	if mustDate(t, "2028-02-29").Day() != 29 {
		t.Fatal("29 de fevereiro de ano bissexto deveria ser aceito")
	}

	for _, in := range []string{"", "2026-02-30", "2027-02-29", "13/11/2026", "2026-13-01", "2026-11-13T00:00:00Z", "0000-01-01"} {
		if _, err := ParseCivilDate(in); !errors.Is(err, ErrInvalidCivilDate) {
			t.Errorf("ParseCivilDate(%q) erro = %v, quero ErrInvalidCivilDate", in, err)
		}
	}
	if _, err := NewCivilDate(2026, time.April, 31); !errors.Is(err, ErrInvalidCivilDate) {
		t.Errorf("31 de abril deveria ser recusado, erro = %v", err)
	}
}

func TestCivilDateArithmetic(t *testing.T) {
	t.Parallel()
	d := mustDate(t, "2026-11-13")

	if got := d.AddDays(18).String(); got != "2026-12-01" {
		t.Errorf("AddDays(18) = %s", got)
	}
	if got := d.AddDays(-13).String(); got != "2026-10-31" {
		t.Errorf("AddDays(-13) = %s", got)
	}
	if got := mustDate(t, "2028-02-28").AddDays(1).String(); got != "2028-02-29" {
		t.Errorf("bissexto: %s", got)
	}
	if got := d.DaysSince(mustDate(t, "2026-11-01")); got != 12 {
		t.Errorf("DaysSince = %d", got)
	}
	if got := mustDate(t, "2026-11-01").DaysSince(d); got != -12 {
		t.Errorf("DaysSince negativo = %d", got)
	}
	if got := mustDate(t, "2027-01-01").DaysSince(mustDate(t, "2026-01-01")); got != 365 {
		t.Errorf("DaysSince de um ano = %d", got)
	}
	if d.Weekday() != time.Friday {
		t.Errorf("Weekday = %s", d.Weekday())
	}
}

func TestCivilDateCompare(t *testing.T) {
	t.Parallel()
	a, b, c := mustDate(t, "2026-11-13"), mustDate(t, "2026-11-14"), mustDate(t, "2027-01-01")
	if !a.Before(b) || !b.After(a) || a.After(b) || b.Before(a) {
		t.Error("ordem por dia incorreta")
	}
	if !b.Before(c) || c.Compare(a) != 1 || a.Compare(a) != 0 {
		t.Error("ordem por ano incorreta")
	}
	if !mustDate(t, "2026-10-31").Before(mustDate(t, "2026-11-01")) {
		t.Error("ordem por mês incorreta")
	}
	if !(CivilDate{}).IsZero() || a.IsZero() {
		t.Error("IsZero incorreto")
	}
}

func TestCivilDateText(t *testing.T) {
	t.Parallel()
	d := mustDate(t, "2026-01-05")
	text, err := d.MarshalText()
	if err != nil || string(text) != "2026-01-05" {
		t.Fatalf("MarshalText = %s, %v", text, err)
	}
	var back CivilDate
	if err := back.UnmarshalText(text); err != nil || back != d {
		t.Fatalf("UnmarshalText = %v, %v", back, err)
	}
	if err := back.UnmarshalText([]byte("ontem")); err == nil {
		t.Fatal("UnmarshalText deveria recusar texto inválido")
	}
}
