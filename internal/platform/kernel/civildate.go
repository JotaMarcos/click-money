package kernel

import (
	"fmt"
	"time"
)

// ErrInvalidCivilDate indica texto que não é uma data civil válida.
var ErrInvalidCivilDate = NewError(CodeValidationFailed, "Data inválida. Use o formato AAAA-MM-DD.")

const civilDateLayout = "2006-01-02"

// CivilDate é uma data de calendário sem hora e sem fuso, como "13/11/2026
// no balcão do comércio".
//
// O instante em que essa data começa depende do fuso do comércio; por isso a
// conversão a partir de um instante exige sempre uma localização explícita.
// O valor zero não é uma data válida (ver IsZero).
type CivilDate struct {
	year  int
	month time.Month
	day   int
}

// NewCivilDate cria uma data civil e recusa combinações inexistentes, como
// 30 de fevereiro.
func NewCivilDate(year int, month time.Month, day int) (CivilDate, error) {
	t := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	if t.Year() != year || t.Month() != month || t.Day() != day || year < 1 || year > 9999 {
		return CivilDate{}, ErrInvalidCivilDate
	}
	return CivilDate{year: year, month: month, day: day}, nil
}

// ParseCivilDate lê uma data no formato AAAA-MM-DD.
func ParseCivilDate(s string) (CivilDate, error) {
	t, err := time.Parse(civilDateLayout, s)
	if err != nil {
		return CivilDate{}, ErrInvalidCivilDate
	}
	return NewCivilDate(t.Year(), t.Month(), t.Day())
}

// CivilDateOf devolve a data civil do instante t na localização loc.
//
// É a única forma correta de responder "que dia é hoje para este comércio":
// 21h de 13/11 em Manaus já é 01h de 14/11 em UTC.
func CivilDateOf(t time.Time, loc *time.Location) CivilDate {
	y, m, d := t.In(loc).Date()
	return CivilDate{year: y, month: m, day: d}
}

// Year devolve o ano.
func (d CivilDate) Year() int { return d.year }

// Month devolve o mês.
func (d CivilDate) Month() time.Month { return d.month }

// Day devolve o dia do mês.
func (d CivilDate) Day() int { return d.day }

// IsZero informa se d é o valor zero, que não representa data alguma.
func (d CivilDate) IsZero() bool { return d == CivilDate{} }

// String devolve a data no formato AAAA-MM-DD.
func (d CivilDate) String() string {
	return fmt.Sprintf("%04d-%02d-%02d", d.year, int(d.month), d.day)
}

// FormatBR devolve a data no formato DD/MM/AAAA.
func (d CivilDate) FormatBR() string {
	return fmt.Sprintf("%02d/%02d/%04d", d.day, int(d.month), d.year)
}

// Compare devolve -1, 0 ou +1 conforme d seja anterior, igual ou posterior a o.
func (d CivilDate) Compare(o CivilDate) int {
	switch {
	case d.year != o.year:
		return cmpInt(d.year, o.year)
	case d.month != o.month:
		return cmpInt(int(d.month), int(o.month))
	default:
		return cmpInt(d.day, o.day)
	}
}

// Before informa se d é anterior a o.
func (d CivilDate) Before(o CivilDate) bool { return d.Compare(o) < 0 }

// After informa se d é posterior a o.
func (d CivilDate) After(o CivilDate) bool { return d.Compare(o) > 0 }

// AddDays devolve a data n dias depois de d (ou antes, se n for negativo).
func (d CivilDate) AddDays(n int) CivilDate {
	t := d.utcMidnight().AddDate(0, 0, n)
	return CivilDate{year: t.Year(), month: t.Month(), day: t.Day()}
}

// DaysSince devolve quantos dias de calendário separam o de d. O resultado é
// positivo quando d é posterior a o.
func (d CivilDate) DaysSince(o CivilDate) int {
	// Em UTC todo dia tem 24 horas, então a divisão é exata.
	return int(d.utcMidnight().Sub(o.utcMidnight()) / (24 * time.Hour))
}

// Weekday devolve o dia da semana.
func (d CivilDate) Weekday() time.Weekday { return d.utcMidnight().Weekday() }

// MarshalText implementa encoding.TextMarshaler (AAAA-MM-DD).
func (d CivilDate) MarshalText() ([]byte, error) { return []byte(d.String()), nil }

// UnmarshalText implementa encoding.TextUnmarshaler.
func (d *CivilDate) UnmarshalText(b []byte) error {
	parsed, err := ParseCivilDate(string(b))
	if err != nil {
		return err
	}
	*d = parsed
	return nil
}

func (d CivilDate) utcMidnight() time.Time {
	return time.Date(d.year, d.month, d.day, 0, 0, 0, 0, time.UTC)
}

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}
