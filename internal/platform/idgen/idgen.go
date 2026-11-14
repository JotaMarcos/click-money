// Package idgen fornece a porta de geração de identificadores e a
// implementação de UUIDv7 (RFC 9562) usada no servidor.
package idgen

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"sync"

	"github.com/JotaMarcos/click-money/internal/platform/clock"
	"github.com/JotaMarcos/click-money/internal/platform/kernel"
)

// Generator cria identificadores novos.
type Generator interface {
	NewID() (kernel.ID, error)
}

// maxSeq é o maior valor do contador de 12 bits (campo rand_a do UUIDv7).
const maxSeq = 0x0FFF

// UUIDv7 gera identificadores ordenáveis por tempo.
//
// Dentro do mesmo milissegundo, o campo rand_a funciona como contador, de
// modo que identificadores gerados em sequência pelo mesmo processo são
// estritamente crescentes mesmo se o relógio parar ou voltar.
type UUIDv7 struct {
	clock clock.Clock

	mu     sync.Mutex
	lastMs int64
	seq    uint16
}

// NewUUIDv7 cria um gerador que lê o tempo do relógio informado.
func NewUUIDv7(c clock.Clock) *UUIDv7 { return &UUIDv7{clock: c} }

// NewID devolve um UUIDv7 novo. Só falha se a fonte de aleatoriedade do
// sistema operacional falhar.
func (g *UUIDv7) NewID() (kernel.ID, error) {
	var id kernel.ID
	if _, err := rand.Read(id[8:]); err != nil {
		return kernel.ID{}, fmt.Errorf("idgen: ler aleatoriedade: %w", err)
	}

	ms, seq := g.next()
	id[0] = byte(ms >> 40)
	id[1] = byte(ms >> 32)
	id[2] = byte(ms >> 24)
	id[3] = byte(ms >> 16)
	id[4] = byte(ms >> 8)
	id[5] = byte(ms)
	binary.BigEndian.PutUint16(id[6:8], 0x7000|seq) // versão 7 + contador
	id[8] = id[8]&0x3F | 0x80                       // variante da RFC
	return id, nil
}

func (g *UUIDv7) next() (ms int64, seq uint16) {
	g.mu.Lock()
	defer g.mu.Unlock()

	now := g.clock.Now().UnixMilli()
	switch {
	case now > g.lastMs:
		g.lastMs, g.seq = now, 0
	case g.seq < maxSeq:
		g.seq++
	default:
		// Contador esgotado neste milissegundo: avança o tempo lógico.
		g.lastMs++
		g.seq = 0
	}
	return g.lastMs, g.seq
}

// Sequence é um gerador determinístico para testes: devolve UUIDv7 válidos e
// crescentes, sem aleatoriedade.
type Sequence struct {
	mu   sync.Mutex
	next uint64
}

// NewSequence cria um gerador determinístico que começa em 1.
func NewSequence() *Sequence { return &Sequence{next: 1} }

// NewID devolve o próximo identificador da sequência.
func (s *Sequence) NewID() (kernel.ID, error) {
	s.mu.Lock()
	n := s.next
	s.next++
	s.mu.Unlock()

	var id kernel.ID
	id[6] = 0x70
	id[8] = 0x80
	binary.BigEndian.PutUint64(id[8:], uint64(id[8])<<56|n)
	return id, nil
}
