package outbox_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/JotaMarcos/click-money/internal/platform/clock"
	"github.com/JotaMarcos/click-money/internal/platform/db"
	"github.com/JotaMarcos/click-money/internal/platform/dbtest"
	"github.com/JotaMarcos/click-money/internal/platform/idgen"
	"github.com/JotaMarcos/click-money/internal/platform/kernel"
	"github.com/JotaMarcos/click-money/internal/platform/outbox"
)

const tenantID = "01933b7e-0000-7000-8000-00000000000a"

var (
	aggregateA = mustID("01933b7e-0000-7000-8000-0000000000a1")
	aggregateB = mustID("01933b7e-0000-7000-8000-0000000000b1")
)

func mustID(s string) kernel.ID {
	id, err := kernel.ParseID(s)
	if err != nil {
		panic(err)
	}
	return id
}

// recorder é um Handler que guarda o que recebeu e falha sob demanda.
type recorder struct {
	mu        sync.Mutex
	delivered []outbox.Event
	failFor   map[string]bool // por tipo de evento
}

func (r *recorder) Handle(_ context.Context, ev outbox.Event) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.failFor[ev.Type] {
		return errors.New("consumidor fora do ar")
	}
	r.delivered = append(r.delivered, ev)
	return nil
}

func (r *recorder) types() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.delivered))
	for i, ev := range r.delivered {
		out[i] = ev.Type
	}
	return out
}

func (r *recorder) setFail(eventType string, fail bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.failFor == nil {
		r.failFor = make(map[string]bool)
	}
	r.failFor[eventType] = fail
}

type fixture struct {
	db     *db.DB
	clock  *clock.Fake
	writer *outbox.Writer
	relay  *outbox.Relay
	rec    *recorder
	tenant kernel.TenantID
}

func newFixture(t *testing.T, opts ...outbox.Option) *fixture {
	t.Helper()
	d := dbtest.NewSQLite(t)
	c := clock.NewFake(time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC))
	rec := &recorder{}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	opts = append([]outbox.Option{outbox.WithJitter(func() float64 { return 0 })}, opts...)
	return &fixture{
		db:     d,
		clock:  c,
		writer: outbox.NewWriter(d.Dialect(), c, idgen.NewUUIDv7(c)),
		relay:  outbox.NewRelay(d, rec, c, log, opts...),
		rec:    rec,
		tenant: dbtest.InsertTenant(t, d, tenantID),
	}
}

func (f *fixture) append(t *testing.T, aggregate kernel.ID, eventType string) {
	t.Helper()
	err := f.db.WithinTenant(context.Background(), f.tenant, func(ctx context.Context) error {
		return f.writer.Append(ctx, outbox.NewEvent{
			TenantID: f.tenant, AggregateType: "customer", AggregateID: aggregate,
			Type: eventType, Payload: map[string]int{"amount_cents": 2550},
		})
	})
	if err != nil {
		t.Fatalf("gravar evento %s: %v", eventType, err)
	}
}

func (f *fixture) process(t *testing.T) int {
	t.Helper()
	n, err := f.relay.ProcessBatch(context.Background())
	if err != nil {
		t.Fatalf("ProcessBatch: %v", err)
	}
	return n
}

// drain processa rodadas até a fila não ter mais nada pronto.
func (f *fixture) drain(t *testing.T) {
	t.Helper()
	for range 100 {
		if f.process(t) == 0 {
			return
		}
	}
	t.Fatal("a fila não esvaziou em 100 rodadas")
}

func (f *fixture) stats(t *testing.T) outbox.Stats {
	t.Helper()
	s, err := f.relay.Stats(context.Background())
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	return s
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestCommittedEventIsDelivered(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.append(t, aggregateA, "CreditSaleRecorded")

	if s := f.stats(t); s.Pending != 1 || s.Failed != 0 {
		t.Fatalf("antes da entrega: %+v", s)
	}
	f.drain(t)

	if got := f.rec.types(); !equal(got, []string{"CreditSaleRecorded"}) {
		t.Fatalf("entregues = %v", got)
	}
	ev := f.rec.delivered[0]
	if ev.TenantID != f.tenant || ev.AggregateID != aggregateA || ev.AggregateType != "customer" {
		t.Errorf("evento entregue com dados errados: %+v", ev)
	}
	if string(ev.Payload) != `{"amount_cents":2550}` || !ev.ID.IsV7() || !ev.OccurredAt.Equal(f.clock.Now()) {
		t.Errorf("payload, id ou instante errados: %s %s %s", ev.Payload, ev.ID, ev.OccurredAt)
	}
	if s := f.stats(t); s.Pending != 0 {
		t.Errorf("após a entrega: %+v", s)
	}

	// Nova rodada não reentrega o que já foi publicado.
	f.drain(t)
	if len(f.rec.types()) != 1 {
		t.Errorf("evento publicado foi entregue de novo: %v", f.rec.types())
	}
}

func TestRolledBackEventIsNeverDelivered(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	boom := errors.New("regra de negócio violada")

	err := f.db.WithinTenant(context.Background(), f.tenant, func(ctx context.Context) error {
		if err := f.writer.Append(ctx, outbox.NewEvent{
			TenantID: f.tenant, AggregateType: "customer", AggregateID: aggregateA, Type: "PaymentRecorded",
		}); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("erro = %v", err)
	}

	f.drain(t)
	if got := f.rec.types(); len(got) != 0 {
		t.Fatalf("evento de transação desfeita foi entregue: %v", got)
	}
	if s := f.stats(t); s.Pending != 0 {
		t.Errorf("fila deveria estar vazia: %+v", s)
	}
}

func TestFailedDeliveryIsRetriedWithGrowingBackoff(t *testing.T) {
	t.Parallel()
	f := newFixture(t, outbox.WithBackoff(time.Second, time.Minute))
	f.append(t, aggregateA, "PaymentRecorded")
	f.rec.setFail("PaymentRecorded", true)

	// Esperas crescentes: 1s, 2s, 4s, 8s. Antes do prazo o evento não é lido.
	for attempt, wait := range []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second} {
		if n := f.process(t); n != 1 {
			t.Fatalf("tentativa %d: lidos %d eventos, quero 1", attempt+1, n)
		}
		if s := f.stats(t); s.Pending != 1 {
			t.Fatalf("tentativa %d: o evento deveria continuar na fila: %+v", attempt+1, s)
		}
		f.clock.Advance(wait - time.Millisecond)
		if n := f.process(t); n != 0 {
			t.Fatalf("tentativa %d: evento relido antes do prazo de %s", attempt+1, wait)
		}
		f.clock.Advance(time.Millisecond)
	}

	f.rec.setFail("PaymentRecorded", false)
	f.drain(t)
	if got := f.rec.types(); !equal(got, []string{"PaymentRecorded"}) {
		t.Fatalf("após o consumidor voltar: %v", got)
	}
	if got := f.rec.delivered[0].Attempts; got != 4 {
		t.Errorf("tentativas falhas registradas = %d, quero 4", got)
	}
}

func TestBackoffIsCappedAndJittered(t *testing.T) {
	t.Parallel()
	// Jitter máximo (quase 1): a espera cresce até 20% além do valor base.
	f := newFixture(t,
		outbox.WithBackoff(time.Second, 4*time.Second),
		outbox.WithJitter(func() float64 { return 0.5 }),
		outbox.WithMaxAttempts(20))
	f.append(t, aggregateA, "PaymentRecorded")
	f.rec.setFail("PaymentRecorded", true)

	// Base 1s, 2s, 4s, 4s (teto), cada uma acrescida de 10%.
	for attempt, base := range []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 4 * time.Second} {
		if n := f.process(t); n != 1 {
			t.Fatalf("tentativa %d: lidos %d", attempt+1, n)
		}
		wait := base + base/10
		f.clock.Advance(wait - time.Millisecond)
		if n := f.process(t); n != 0 {
			t.Fatalf("tentativa %d: relido antes de %s", attempt+1, wait)
		}
		f.clock.Advance(time.Millisecond)
	}
}

func TestEventFailsPermanentlyAfterMaxAttempts(t *testing.T) {
	t.Parallel()
	f := newFixture(t, outbox.WithMaxAttempts(3), outbox.WithBackoff(time.Second, time.Second))
	f.append(t, aggregateA, "PaymentRecorded")
	f.rec.setFail("PaymentRecorded", true)

	for range 3 {
		f.process(t)
		f.clock.Advance(2 * time.Second)
	}
	if s := f.stats(t); s.Pending != 0 || s.Failed != 1 {
		t.Fatalf("após esgotar as tentativas: %+v", s)
	}

	// Mesmo com o consumidor de volta, um evento falho não é reentregue
	// sozinho: exige intervenção, e o alerta dispara por Stats.Failed.
	f.rec.setFail("PaymentRecorded", false)
	f.clock.Advance(time.Hour)
	f.drain(t)
	if got := f.rec.types(); len(got) != 0 {
		t.Errorf("evento falho foi entregue: %v", got)
	}
}

func TestOrderIsPreservedPerAggregate(t *testing.T) {
	t.Parallel()
	f := newFixture(t, outbox.WithBackoff(time.Second, time.Second))
	f.append(t, aggregateA, "A1")
	f.append(t, aggregateA, "A2")
	f.append(t, aggregateB, "B1")
	f.append(t, aggregateA, "A3")
	f.rec.setFail("A1", true)

	// A1 falha: B1 segue normalmente, mas A2 e A3 esperam por A1.
	f.drain(t)
	if got := f.rec.types(); !equal(got, []string{"B1"}) {
		t.Fatalf("com A1 falhando, entregues = %v; quero só B1", got)
	}

	f.rec.setFail("A1", false)
	f.clock.Advance(2 * time.Second)
	f.drain(t)
	if got := f.rec.types(); !equal(got, []string{"B1", "A1", "A2", "A3"}) {
		t.Fatalf("ordem final = %v", got)
	}
}

func TestPermanentFailureKeepsAggregateBlocked(t *testing.T) {
	t.Parallel()
	f := newFixture(t, outbox.WithMaxAttempts(1))
	f.append(t, aggregateA, "A1")
	f.append(t, aggregateA, "A2")
	f.rec.setFail("A1", true)

	f.drain(t)
	f.clock.Advance(time.Hour)
	f.drain(t)
	// Entregar A2 sem A1 aplicaria eventos fora de ordem.
	if got := f.rec.types(); len(got) != 0 {
		t.Fatalf("A2 foi entregue apesar de A1 ter falhado: %v", got)
	}
	if s := f.stats(t); s.Failed != 1 || s.Pending != 1 {
		t.Errorf("estatísticas: %+v", s)
	}
}

func TestBatchSizeLimitsEachRound(t *testing.T) {
	t.Parallel()
	f := newFixture(t, outbox.WithBatchSize(2))
	for i, agg := range []string{"c1", "c2", "c3", "c4", "c5"} {
		f.append(t, mustID("01933b7e-0000-7000-8000-0000000000"+agg), "E")
		_ = i
	}
	if n := f.process(t); n != 2 {
		t.Fatalf("primeira rodada leu %d eventos, quero 2", n)
	}
	f.drain(t)
	if got := len(f.rec.types()); got != 5 {
		t.Fatalf("entregues %d eventos, quero 5", got)
	}
}

func TestStatsReportsOldestPendingAge(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	if s := f.stats(t); s != (outbox.Stats{}) {
		t.Fatalf("fila vazia: %+v", s)
	}
	f.append(t, aggregateA, "A1")
	f.clock.Advance(90 * time.Second)
	f.append(t, aggregateB, "B1")
	f.clock.Advance(30 * time.Second)

	if s := f.stats(t); s.Pending != 2 || s.OldestPendingAge != 2*time.Minute {
		t.Fatalf("estatísticas: %+v", s)
	}
}

func TestPurgeRemovesOnlyOldPublishedEvents(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.append(t, aggregateA, "antigo")
	f.drain(t)
	f.clock.Advance(8 * 24 * time.Hour)
	f.append(t, aggregateB, "recente")
	f.drain(t)
	f.append(t, aggregateB, "pendente")

	deleted, err := f.relay.Purge(context.Background(), 7*24*time.Hour)
	if err != nil || deleted != 1 {
		t.Fatalf("Purge = %d, %v; quero 1", deleted, err)
	}
	var remaining int
	if err := f.db.SQL().QueryRow("SELECT COUNT(*) FROM outbox_events").Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != 2 {
		t.Errorf("restaram %d eventos, quero 2 (o recente e o pendente)", remaining)
	}
}

func TestAppendValidatesAndRequiresTransaction(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()
	valid := outbox.NewEvent{TenantID: f.tenant, AggregateType: "customer", AggregateID: aggregateA, Type: "X"}

	if err := f.writer.Append(ctx, valid); !errors.Is(err, db.ErrNoTransaction) {
		t.Errorf("Append fora de transação: %v", err)
	}

	invalid := map[string]outbox.NewEvent{
		"sem comércio":         {AggregateType: "customer", AggregateID: aggregateA, Type: "X"},
		"sem agregado":         {TenantID: f.tenant, AggregateType: "customer", Type: "X"},
		"sem tipo":             {TenantID: f.tenant, AggregateType: "customer", AggregateID: aggregateA},
		"sem tipo de agregado": {TenantID: f.tenant, AggregateID: aggregateA, Type: "X"},
	}
	for name, ev := range invalid {
		err := f.db.WithinTenant(ctx, f.tenant, func(ctx context.Context) error { return f.writer.Append(ctx, ev) })
		if err == nil {
			t.Errorf("%s: Append deveria falhar", name)
		}
	}

	unserializable := valid
	unserializable.Payload = make(chan int)
	err := f.db.WithinTenant(ctx, f.tenant, func(ctx context.Context) error { return f.writer.Append(ctx, unserializable) })
	if err == nil {
		t.Error("payload não serializável deveria falhar")
	}
}

func TestRunDeliversUntilCanceled(t *testing.T) {
	t.Parallel()
	f := newFixture(t, outbox.WithPollInterval(5*time.Millisecond))
	f.append(t, aggregateA, "A1")
	f.append(t, aggregateA, "A2")

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- f.relay.Run(ctx) }()

	deadline := time.After(5 * time.Second)
	for len(f.rec.types()) < 2 {
		select {
		case <-deadline:
			t.Fatalf("Run não entregou os eventos a tempo: %v", f.rec.types())
		case <-time.After(5 * time.Millisecond):
		}
	}
	// Um evento gravado com o Relay já rodando também é entregue.
	f.append(t, aggregateB, "B1")
	for len(f.rec.types()) < 3 {
		select {
		case <-deadline:
			t.Fatalf("Run não entregou o evento novo: %v", f.rec.types())
		case <-time.After(5 * time.Millisecond):
		}
	}

	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Run = %v, quero context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run não parou após o cancelamento")
	}
	if got := f.rec.types(); !equal(got, []string{"A1", "A2", "B1"}) {
		t.Errorf("entregues = %v", got)
	}
}
