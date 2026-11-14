package idgen

import (
	"sync"
	"testing"
	"time"

	"github.com/JotaMarcos/click-money/internal/platform/clock"
	"github.com/JotaMarcos/click-money/internal/platform/kernel"
)

func TestUUIDv7IsValidAndCarriesTime(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 11, 13, 21, 0, 0, 123_000_000, time.UTC)
	gen := NewUUIDv7(clock.NewFake(now))

	id, err := gen.NewID()
	if err != nil {
		t.Fatal(err)
	}
	if !id.IsV7() {
		t.Fatalf("%s não é UUIDv7", id)
	}
	if !id.Time().Equal(now) {
		t.Fatalf("instante embutido = %s, quero %s", id.Time(), now)
	}
	parsed, err := kernel.ParseID(id.String())
	if err != nil || parsed != id {
		t.Fatalf("ida e volta pelo texto falhou: %v, %v", parsed, err)
	}
}

func TestUUIDv7IsStrictlyIncreasing(t *testing.T) {
	t.Parallel()
	fake := clock.NewFake(time.Date(2026, 11, 13, 21, 0, 0, 0, time.UTC))
	gen := NewUUIDv7(fake)

	// Mais identificadores do que o contador de 12 bits comporta em um
	// milissegundo, com o relógio parado e depois voltando no tempo.
	const total = 3 * (maxSeq + 1)
	prev, err := gen.NewID()
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i < total; i++ {
		if i == total/2 {
			fake.Advance(-time.Hour)
		}
		id, err := gen.NewID()
		if err != nil {
			t.Fatal(err)
		}
		if id.Compare(prev) <= 0 {
			t.Fatalf("identificador %d (%s) não é maior que o anterior (%s)", i, id, prev)
		}
		if !id.IsV7() {
			t.Fatalf("identificador %d (%s) não é UUIDv7", i, id)
		}
		prev = id
	}
}

func TestUUIDv7ConcurrentUniqueness(t *testing.T) {
	t.Parallel()
	gen := NewUUIDv7(clock.System{})

	const workers, perWorker = 8, 2000
	ids := make(chan kernel.ID, workers*perWorker)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range perWorker {
				id, err := gen.NewID()
				if err != nil {
					t.Error(err)
					return
				}
				ids <- id
			}
		}()
	}
	wg.Wait()
	close(ids)

	seen := make(map[kernel.ID]struct{}, workers*perWorker)
	for id := range ids {
		if _, dup := seen[id]; dup {
			t.Fatalf("identificador repetido: %s", id)
		}
		seen[id] = struct{}{}
	}
	if len(seen) != workers*perWorker {
		t.Fatalf("gerados %d, quero %d", len(seen), workers*perWorker)
	}
}

func TestSequenceIsDeterministic(t *testing.T) {
	t.Parallel()
	a, b := NewSequence(), NewSequence()
	var prev kernel.ID
	for i := range 5 {
		x, _ := a.NewID()
		y, _ := b.NewID()
		if x != y {
			t.Fatalf("sequências divergiram no passo %d: %s e %s", i, x, y)
		}
		if !x.IsV7() || x.Compare(prev) <= 0 {
			t.Fatalf("passo %d: %s inválido ou fora de ordem", i, x)
		}
		prev = x
	}
	if prev.String() != "00000000-0000-7000-8000-000000000005" {
		t.Fatalf("quinto identificador = %s", prev)
	}
}
