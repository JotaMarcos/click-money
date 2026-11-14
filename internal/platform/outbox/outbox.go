// Package outbox implementa a caixa de saída transacional.
//
// Um caso de uso grava seus eventos com Writer.Append dentro da mesma
// transação da mudança de estado. Depois, o Relay (no cmd/worker) lê os
// eventos pendentes e os entrega a um Handler. A entrega é "ao menos uma
// vez": todo Handler precisa ser idempotente, usando Event.ID como chave.
package outbox

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"time"

	"github.com/JotaMarcos/click-money/internal/platform/clock"
	"github.com/JotaMarcos/click-money/internal/platform/db"
	"github.com/JotaMarcos/click-money/internal/platform/idgen"
	"github.com/JotaMarcos/click-money/internal/platform/kernel"
)

// Event é um evento de domínio gravado na outbox.
type Event struct {
	ID            kernel.ID
	TenantID      kernel.TenantID
	AggregateType string
	AggregateID   kernel.ID
	Type          string
	Payload       json.RawMessage
	OccurredAt    time.Time
	// Attempts é o número de entregas que já falharam.
	Attempts int
}

// NewEvent descreve um evento a gravar. O payload é serializado em JSON e
// não deve carregar dados pessoais além do necessário: consumidores buscam
// detalhes pelo identificador do agregado.
type NewEvent struct {
	TenantID      kernel.TenantID
	AggregateType string
	AggregateID   kernel.ID
	Type          string
	Payload       any
}

// Writer grava eventos na transação corrente.
type Writer struct {
	dialect db.Dialect
	clock   clock.Clock
	ids     idgen.Generator
}

// NewWriter cria um Writer.
func NewWriter(dialect db.Dialect, c clock.Clock, ids idgen.Generator) *Writer {
	return &Writer{dialect: dialect, clock: c, ids: ids}
}

// Append grava o evento na transação aberta pelo Unit of Work. Se a
// transação for desfeita, o evento some junto com a mudança que o originou.
func (w *Writer) Append(ctx context.Context, ev NewEvent) error {
	if ev.TenantID.IsZero() || ev.AggregateID.IsZero() || ev.AggregateType == "" || ev.Type == "" {
		return errors.New("outbox: evento sem comércio, agregado ou tipo")
	}
	tx, err := db.Tx(ctx)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(ev.Payload)
	if err != nil {
		return fmt.Errorf("outbox: serializar evento %s: %w", ev.Type, err)
	}
	id, err := w.ids.NewID()
	if err != nil {
		return fmt.Errorf("outbox: gerar identificador: %w", err)
	}

	now := w.dialect.TimeArg(w.clock.Now())
	_, err = tx.ExecContext(ctx, w.dialect.Rebind(`
		INSERT INTO outbox_events
			(id, tenant_id, aggregate_type, aggregate_id, event_type, payload, occurred_at, next_attempt_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`),
		id.String(), ev.TenantID.String(), ev.AggregateType, ev.AggregateID.String(),
		ev.Type, string(payload), now, now)
	if err != nil {
		return fmt.Errorf("outbox: gravar evento %s: %w", ev.Type, err)
	}
	return nil
}

// Handler recebe os eventos entregues pelo Relay. Um erro faz o evento ser
// reentregue mais tarde; por isso Handle precisa ser idempotente.
type Handler interface {
	Handle(ctx context.Context, ev Event) error
}

// HandlerFunc adapta uma função a Handler.
type HandlerFunc func(ctx context.Context, ev Event) error

// Handle chama f.
func (f HandlerFunc) Handle(ctx context.Context, ev Event) error { return f(ctx, ev) }

// Stats resume o estado da fila, para métricas e alertas.
type Stats struct {
	// Pending é o número de eventos aguardando entrega.
	Pending int
	// Failed é o número de eventos que esgotaram as tentativas.
	Failed int
	// OldestPendingAge é a idade do evento pendente mais antigo.
	OldestPendingAge time.Duration
}

// Relay entrega os eventos pendentes da outbox.
type Relay struct {
	db      *db.DB
	handler Handler
	clock   clock.Clock
	log     *slog.Logger

	batchSize    int
	maxAttempts  int
	baseBackoff  time.Duration
	maxBackoff   time.Duration
	pollInterval time.Duration
	jitter       func() float64
}

// Option ajusta um Relay.
type Option func(*Relay)

// WithBatchSize define quantos eventos são lidos por rodada.
func WithBatchSize(n int) Option { return func(r *Relay) { r.batchSize = n } }

// WithMaxAttempts define após quantas falhas um evento é dado como perdido.
func WithMaxAttempts(n int) Option { return func(r *Relay) { r.maxAttempts = n } }

// WithBackoff define o intervalo inicial e o teto entre tentativas.
func WithBackoff(base, limit time.Duration) Option {
	return func(r *Relay) { r.baseBackoff, r.maxBackoff = base, limit }
}

// WithPollInterval define a espera entre rodadas quando a fila está vazia.
func WithPollInterval(d time.Duration) Option { return func(r *Relay) { r.pollInterval = d } }

// WithJitter substitui a fonte do jitter (valores em [0, 1)), para testes.
func WithJitter(f func() float64) Option { return func(r *Relay) { r.jitter = f } }

// NewRelay cria um Relay com padrões conservadores: lotes de 50, dez
// tentativas, espera de 1 s a 5 min entre tentativas.
func NewRelay(d *db.DB, h Handler, c clock.Clock, log *slog.Logger, opts ...Option) *Relay {
	r := &Relay{
		db: d, handler: h, clock: c, log: log,
		batchSize:    50,
		maxAttempts:  10,
		baseBackoff:  time.Second,
		maxBackoff:   5 * time.Minute,
		pollInterval: time.Second,
		jitter:       rand.Float64,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Run entrega eventos até ctx ser cancelado. Erros de uma rodada são
// registrados e a rodada seguinte tenta de novo; Run só retorna com o
// cancelamento.
func (r *Relay) Run(ctx context.Context) error {
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}

		n, err := r.ProcessBatch(ctx)
		if err != nil && ctx.Err() == nil {
			r.log.ErrorContext(ctx, "rodada da outbox falhou", "error", err)
		}
		// Enquanto houver eventos, a próxima rodada começa já: como cada
		// rodada lê no máximo um evento por agregado, é assim que uma
		// sequência de eventos do mesmo agregado escoa sem esperar.
		wait := r.pollInterval
		if err == nil && n > 0 {
			wait = 0
		}
		timer.Reset(wait)
	}
}

// ProcessBatch lê um lote de eventos prontos e tenta entregar cada um.
// Devolve quantos eventos foram lidos.
//
// A ordem por agregado é preservada: um evento só é lido quando não há
// evento anterior do mesmo agregado ainda por publicar. Por isso cada rodada
// traz no máximo um evento por agregado, e um evento que falha segura os
// seguintes do mesmo agregado até ser entregue.
func (r *Relay) ProcessBatch(ctx context.Context) (int, error) {
	processed := 0
	err := r.db.WithinSystem(ctx, func(ctx context.Context) error {
		tx, err := db.Tx(ctx)
		if err != nil {
			return err
		}
		events, err := r.claim(ctx, tx)
		if err != nil {
			return err
		}
		processed = len(events)

		for _, ev := range events {
			if err := r.deliver(ctx, tx, ev); err != nil {
				return err
			}
		}
		return nil
	})
	return processed, err
}

func (r *Relay) claim(ctx context.Context, tx db.Executor) ([]Event, error) {
	dialect := r.db.Dialect()
	query := `
		SELECT e.id, e.tenant_id, e.aggregate_type, e.aggregate_id, e.event_type,
		       e.payload, e.occurred_at, e.attempts
		FROM outbox_events e
		WHERE e.published_at IS NULL
		  AND e.failed_at IS NULL
		  AND e.next_attempt_at <= ?
		  AND NOT EXISTS (
		        SELECT 1 FROM outbox_events p
		        WHERE p.aggregate_id = e.aggregate_id
		          AND p.published_at IS NULL
		          AND p.id < e.id)
		ORDER BY e.id
		LIMIT ?`
	if dialect == db.Postgres {
		// Vários workers podem rodar em paralelo sem entregar o mesmo evento.
		query += ` FOR UPDATE OF e SKIP LOCKED`
	}

	rows, err := tx.QueryContext(ctx, dialect.Rebind(query), dialect.TimeArg(r.clock.Now()), r.batchSize)
	if err != nil {
		return nil, fmt.Errorf("outbox: ler eventos pendentes: %w", err)
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		ev, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, ev)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("outbox: percorrer eventos pendentes: %w", err)
	}
	return events, nil
}

func scanEvent(rows *sql.Rows) (Event, error) {
	var (
		ev                    Event
		id, tenant, aggregate string
		payload               []byte
		occurredAt            db.Time
	)
	if err := rows.Scan(&id, &tenant, &ev.AggregateType, &aggregate, &ev.Type, &payload, &occurredAt, &ev.Attempts); err != nil {
		return Event{}, fmt.Errorf("outbox: ler evento: %w", err)
	}
	var err error
	if ev.ID, err = kernel.ParseID(id); err != nil {
		return Event{}, fmt.Errorf("outbox: identificador de evento %q: %w", id, err)
	}
	if ev.TenantID, err = kernel.ParseTenantID(tenant); err != nil {
		return Event{}, fmt.Errorf("outbox: comércio do evento %s: %w", id, err)
	}
	if ev.AggregateID, err = kernel.ParseID(aggregate); err != nil {
		return Event{}, fmt.Errorf("outbox: agregado do evento %s: %w", id, err)
	}
	ev.Payload = payload
	ev.OccurredAt = occurredAt.Time
	return ev, nil
}

// deliver entrega um evento e grava o resultado: publicado, reagendado com
// espera crescente ou, esgotadas as tentativas, falho. Só devolve erro se
// não conseguir gravar o resultado.
func (r *Relay) deliver(ctx context.Context, tx db.Executor, ev Event) error {
	dialect := r.db.Dialect()
	now := r.clock.Now()

	handleErr := r.handler.Handle(ctx, ev)
	if handleErr == nil {
		_, err := tx.ExecContext(ctx, dialect.Rebind(
			`UPDATE outbox_events SET published_at = ?, last_error = NULL WHERE id = ?`),
			dialect.TimeArg(now), ev.ID.String())
		if err != nil {
			return fmt.Errorf("outbox: marcar evento %s como publicado: %w", ev.ID, err)
		}
		return nil
	}

	attempts := ev.Attempts + 1
	logAttrs := []any{"event_id", ev.ID.String(), "event_type", ev.Type, "attempts", attempts, "error", handleErr}

	if attempts >= r.maxAttempts {
		r.log.ErrorContext(ctx, "evento da outbox esgotou as tentativas", logAttrs...)
		_, err := tx.ExecContext(ctx, dialect.Rebind(
			`UPDATE outbox_events SET attempts = ?, last_error = ?, failed_at = ? WHERE id = ?`),
			attempts, truncate(handleErr.Error()), dialect.TimeArg(now), ev.ID.String())
		if err != nil {
			return fmt.Errorf("outbox: marcar evento %s como falho: %w", ev.ID, err)
		}
		return nil
	}

	r.log.WarnContext(ctx, "entrega de evento da outbox falhou", logAttrs...)
	next := now.Add(r.backoff(attempts))
	_, err := tx.ExecContext(ctx, dialect.Rebind(
		`UPDATE outbox_events SET attempts = ?, last_error = ?, next_attempt_at = ? WHERE id = ?`),
		attempts, truncate(handleErr.Error()), dialect.TimeArg(next), ev.ID.String())
	if err != nil {
		return fmt.Errorf("outbox: reagendar evento %s: %w", ev.ID, err)
	}
	return nil
}

// backoff devolve a espera antes da próxima tentativa: exponencial, limitada
// ao teto e com jitter de até 20% para que consumidores não tentem todos ao
// mesmo tempo quando uma dependência volta.
func (r *Relay) backoff(attempts int) time.Duration {
	d := r.baseBackoff
	for i := 1; i < attempts && d < r.maxBackoff; i++ {
		d *= 2
	}
	if d > r.maxBackoff {
		d = r.maxBackoff
	}
	return d + time.Duration(r.jitter()*0.2*float64(d))
}

// truncate limita o texto de erro guardado no banco.
func truncate(s string) string {
	const limit = 500
	if len(s) <= limit {
		return s
	}
	return s[:limit]
}

// Stats devolve o tamanho e a idade da fila.
func (r *Relay) Stats(ctx context.Context) (Stats, error) {
	var stats Stats
	err := r.db.WithinSystem(ctx, func(ctx context.Context) error {
		tx, err := db.Tx(ctx)
		if err != nil {
			return err
		}
		var oldest db.Time
		row := tx.QueryRowContext(ctx, `
			SELECT
				COUNT(*) FILTER (WHERE published_at IS NULL AND failed_at IS NULL),
				COUNT(*) FILTER (WHERE failed_at IS NOT NULL),
				MIN(occurred_at) FILTER (WHERE published_at IS NULL AND failed_at IS NULL)
			FROM outbox_events
			WHERE published_at IS NULL`)
		if err := row.Scan(&stats.Pending, &stats.Failed, &oldest); err != nil {
			return fmt.Errorf("outbox: ler estatísticas: %w", err)
		}
		if oldest.Valid {
			stats.OldestPendingAge = r.clock.Now().Sub(oldest.Time)
		}
		return nil
	})
	return stats, err
}

// Purge apaga eventos publicados há mais de olderThan e devolve quantos
// foram apagados.
func (r *Relay) Purge(ctx context.Context, olderThan time.Duration) (int64, error) {
	var deleted int64
	err := r.db.WithinSystem(ctx, func(ctx context.Context) error {
		tx, err := db.Tx(ctx)
		if err != nil {
			return err
		}
		dialect := r.db.Dialect()
		res, err := tx.ExecContext(ctx, dialect.Rebind(
			`DELETE FROM outbox_events WHERE published_at IS NOT NULL AND published_at < ?`),
			dialect.TimeArg(r.clock.Now().Add(-olderThan)))
		if err != nil {
			return fmt.Errorf("outbox: expurgar eventos publicados: %w", err)
		}
		deleted, err = res.RowsAffected()
		return err
	})
	return deleted, err
}
