package clock

import (
	"testing"
	"time"
	_ "time/tzdata"
)

func TestTodayFollowsTenantTimezone(t *testing.T) {
	t.Parallel()
	manaus, err := time.LoadLocation("America/Manaus")
	if err != nil {
		t.Fatal(err)
	}
	eirunepe, err := time.LoadLocation("America/Eirunepe")
	if err != nil {
		t.Fatal(err)
	}

	// 21h em Manaus e 20h em Eirunepé: em UTC já é o dia seguinte.
	c := NewFake(time.Date(2026, 11, 14, 1, 0, 0, 0, time.UTC))
	if got := Today(c, manaus).String(); got != "2026-11-13" {
		t.Errorf("Manaus: %s", got)
	}
	if got := Today(c, eirunepe).String(); got != "2026-11-13" {
		t.Errorf("Eirunepé: %s", got)
	}
	if got := Today(c, time.UTC).String(); got != "2026-11-14" {
		t.Errorf("UTC: %s", got)
	}

	// Entre 04h e 05h UTC os dois fusos do Amazonas estão em dias diferentes.
	c.Advance(3*time.Hour + 30*time.Minute)
	if got := Today(c, manaus).String(); got != "2026-11-14" {
		t.Errorf("Manaus depois da meia-noite: %s", got)
	}
	if got := Today(c, eirunepe).String(); got != "2026-11-13" {
		t.Errorf("Eirunepé antes da meia-noite: %s", got)
	}
}

func TestFakeAndSystem(t *testing.T) {
	t.Parallel()
	local := time.FixedZone("x", -4*3600)
	c := NewFake(time.Date(2026, 1, 1, 12, 0, 0, 0, local))
	if c.Now().Location() != time.UTC || c.Now().Hour() != 16 {
		t.Errorf("Fake deveria normalizar para UTC: %s", c.Now())
	}
	c.Set(time.Date(2026, 6, 1, 0, 0, 0, 0, local))
	if c.Now().Hour() != 4 || c.Now().Location() != time.UTC {
		t.Errorf("Set deveria normalizar para UTC: %s", c.Now())
	}

	before := time.Now().Add(-time.Second)
	now := System{}.Now()
	if now.Location() != time.UTC || now.Before(before) {
		t.Errorf("System.Now inesperado: %s", now)
	}
}
