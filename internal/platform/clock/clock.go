// Package clock fornece a porta de relógio usada por todo o sistema.
//
// Nenhum código de negócio chama time.Now diretamente: o instante vem de um
// Clock injetado, o que torna os testes determinísticos e concentra em um só
// lugar a regra de que "hoje" depende do fuso do comércio.
package clock

import (
	"sync"
	"time"

	"github.com/JotaMarcos/click-money/internal/platform/kernel"
)

// Clock informa o instante atual.
type Clock interface {
	Now() time.Time
}

// System é o relógio real, sempre em UTC.
type System struct{}

// Now devolve o instante atual em UTC.
func (System) Now() time.Time { return time.Now().UTC() }

// Today devolve a data civil de agora no fuso informado, que deve ser o do
// comércio.
func Today(c Clock, loc *time.Location) kernel.CivilDate {
	return kernel.CivilDateOf(c.Now(), loc)
}

// Fake é um relógio controlado pelo teste. É seguro para uso concorrente.
type Fake struct {
	mu  sync.Mutex
	now time.Time
}

// NewFake cria um relógio parado no instante informado.
func NewFake(now time.Time) *Fake { return &Fake{now: now.UTC()} }

// Now devolve o instante configurado.
func (f *Fake) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

// Set move o relógio para o instante informado.
func (f *Fake) Set(now time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = now.UTC()
}

// Advance adianta o relógio.
func (f *Fake) Advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = f.now.Add(d)
}
