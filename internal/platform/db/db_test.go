package db_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/JotaMarcos/click-money/internal/platform/db"
	"github.com/JotaMarcos/click-money/internal/platform/dbtest"
	"github.com/JotaMarcos/click-money/internal/platform/kernel"
	"github.com/JotaMarcos/click-money/internal/platform/migrate"
)

const (
	tenantA = "01933b7e-0000-7000-8000-00000000000a"
	tenantB = "01933b7e-0000-7000-8000-00000000000b"
)

func countTenants(t *testing.T, d *db.DB) int {
	t.Helper()
	var n int
	err := d.WithinSystem(context.Background(), func(ctx context.Context) error {
		tx, err := db.Tx(ctx)
		if err != nil {
			return err
		}
		return tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM tenants").Scan(&n)
	})
	if err != nil {
		t.Fatalf("contar comércios: %v", err)
	}
	return n
}

func insertTenant(ctx context.Context, d *db.DB, id, plan string) error {
	tx, err := db.Tx(ctx)
	if err != nil {
		return err
	}
	now := d.Dialect().TimeArg(time.Now())
	_, err = tx.ExecContext(ctx, d.Dialect().Rebind(
		`INSERT INTO tenants (id, name, timezone, plan, status, created_at, updated_at)
		 VALUES (?, 'Taberna do Raimundo', 'America/Manaus', ?, 'ACTIVE', ?, ?)`), id, plan, now, now)
	return err
}

func TestOpenRejectsUnknownDriver(t *testing.T) {
	t.Parallel()
	if _, err := db.Open(context.Background(), "mysql", "x"); err == nil {
		t.Fatal("esperava erro para driver desconhecido")
	}
}

func TestOpenFailsWhenDatabaseIsUnreachable(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	// Porta 1 em localhost: conexão recusada de imediato.
	if _, err := db.Open(ctx, "postgres", "postgres://app:x@127.0.0.1:1/click_money?connect_timeout=2"); err == nil {
		t.Fatal("esperava erro ao abrir banco inalcançável")
	}
}

func TestSQLiteFileUsesWALAndForeignKeys(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	d, err := db.Open(ctx, "sqlite", filepath.Join(t.TempDir(), "dev.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	var journal string
	var foreignKeys int
	if err := d.SQL().QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&journal); err != nil {
		t.Fatal(err)
	}
	if err := d.SQL().QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
		t.Fatal(err)
	}
	if journal != "wal" || foreignKeys != 1 {
		t.Errorf("journal_mode = %s, foreign_keys = %d", journal, foreignKeys)
	}
	if d.Dialect() != db.SQLite {
		t.Errorf("Dialect = %s", d.Dialect())
	}
}

func TestMigrationsAreReversible(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	d, err := db.Open(ctx, "sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	m, err := migrate.New(d)
	if err != nil {
		t.Fatal(err)
	}

	tableExists := func(name string) bool {
		var n int
		if err := d.SQL().QueryRowContext(ctx,
			"SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?", name).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n == 1
	}

	if pending, err := m.Pending(ctx); err != nil || pending == 0 {
		t.Fatalf("banco novo deveria ter migrações pendentes: %d, %v", pending, err)
	}
	applied, err := m.Up(ctx)
	if err != nil || applied == 0 {
		t.Fatalf("Up = %d, %v", applied, err)
	}
	if !tableExists("tenants") || !tableExists("outbox_events") {
		t.Fatal("tabelas da plataforma não foram criadas")
	}
	top, err := m.Version(ctx)
	if err != nil || top == 0 {
		t.Fatalf("Version = %d, %v", top, err)
	}
	if pending, err := m.Pending(ctx); err != nil || pending != 0 {
		t.Fatalf("Pending após Up = %d, %v", pending, err)
	}
	if again, err := m.Up(ctx); err != nil || again != 0 {
		t.Fatalf("segundo Up deveria ser nulo: %d, %v", again, err)
	}

	// Reverte tudo, uma migração por vez, e reaplica.
	for v := top; v > 0; {
		if err := m.Down(ctx); err != nil {
			t.Fatalf("Down a partir da versão %d: %v", v, err)
		}
		if v, err = m.Version(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if tableExists("tenants") || tableExists("outbox_events") {
		t.Fatal("Down deveria remover as tabelas")
	}
	if _, err := m.Up(ctx); err != nil {
		t.Fatalf("reaplicar após Down: %v", err)
	}
	if !tableExists("tenants") || !tableExists("outbox_events") {
		t.Fatal("tabelas não foram recriadas")
	}
}

func TestWithinTenantCommitsAndRollsBack(t *testing.T) {
	t.Parallel()
	d := dbtest.NewSQLite(t)
	ctx := context.Background()
	tenant, _ := kernel.ParseTenantID(tenantA)

	if err := d.WithinTenant(ctx, tenant, func(ctx context.Context) error {
		return insertTenant(ctx, d, tenantA, "FREE")
	}); err != nil {
		t.Fatalf("transação confirmada: %v", err)
	}
	if n := countTenants(t, d); n != 1 {
		t.Fatalf("após commit há %d comércios", n)
	}

	boom := errors.New("regra violada")
	err := d.WithinTenant(ctx, tenant, func(ctx context.Context) error {
		if err := insertTenant(ctx, d, tenantB, "FREE"); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("erro = %v, quero o erro do caso de uso", err)
	}
	if n := countTenants(t, d); n != 1 {
		t.Fatalf("após rollback há %d comércios", n)
	}
}

func TestWithinTenantRollsBackOnPanic(t *testing.T) {
	t.Parallel()
	d := dbtest.NewSQLite(t)
	tenant, _ := kernel.ParseTenantID(tenantA)

	func() {
		defer func() {
			if recover() == nil {
				t.Error("o pânico deveria ser repassado")
			}
		}()
		_ = d.WithinTenant(context.Background(), tenant, func(ctx context.Context) error {
			if err := insertTenant(ctx, d, tenantA, "FREE"); err != nil {
				t.Error(err)
			}
			panic("falha inesperada")
		})
	}()

	// Se a transação tivesse ficado aberta, a única conexão do SQLite estaria
	// presa e esta consulta travaria.
	if n := countTenants(t, d); n != 0 {
		t.Fatalf("após pânico há %d comércios", n)
	}
}

func TestUnitOfWorkGuards(t *testing.T) {
	t.Parallel()
	d := dbtest.NewSQLite(t)
	ctx := context.Background()
	tenant, _ := kernel.ParseTenantID(tenantA)

	if _, err := db.Tx(ctx); !errors.Is(err, db.ErrNoTransaction) {
		t.Errorf("Tx fora de transação: %v", err)
	}
	if err := d.WithinTenant(ctx, kernel.TenantID{}, func(context.Context) error { return nil }); err == nil {
		t.Error("transação de negócio sem comércio deveria ser recusada")
	}
	err := d.WithinTenant(ctx, tenant, func(ctx context.Context) error {
		return d.WithinTenant(ctx, tenant, func(context.Context) error { return nil })
	})
	if !errors.Is(err, db.ErrNestedTransaction) {
		t.Errorf("transação aninhada: %v", err)
	}
	err = d.WithinSystem(ctx, func(ctx context.Context) error {
		return d.WithinSystem(ctx, func(context.Context) error { return nil })
	})
	if !errors.Is(err, db.ErrNestedTransaction) {
		t.Errorf("transação de sistema aninhada: %v", err)
	}

	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := d.WithinSystem(canceled, func(context.Context) error { return nil }); err == nil {
		t.Error("contexto cancelado deveria impedir a transação")
	}
}

func TestConstraintsAreEnforced(t *testing.T) {
	t.Parallel()
	d := dbtest.NewSQLite(t)
	ctx := context.Background()
	tenant := dbtest.InsertTenant(t, d, tenantA)

	err := d.WithinTenant(ctx, tenant, func(ctx context.Context) error {
		return insertTenant(ctx, d, tenantB, "GRATIS")
	})
	if err == nil {
		t.Error("plano fora da lista deveria violar o CHECK")
	}

	now := d.Dialect().TimeArg(time.Now())
	insertEvent := func(tenantID, payload string) error {
		return d.WithinTenant(ctx, tenant, func(ctx context.Context) error {
			tx, err := db.Tx(ctx)
			if err != nil {
				return err
			}
			_, err = tx.ExecContext(ctx, `INSERT INTO outbox_events
				(id, tenant_id, aggregate_type, aggregate_id, event_type, payload, occurred_at, next_attempt_at)
				VALUES ('01933b7e-0000-7000-8000-0000000000e1', ?, 'x', '01933b7e-0000-7000-8000-0000000000a1', 'X', ?, ?, ?)`,
				tenantID, payload, now, now)
			return err
		})
	}
	if err := insertEvent(tenantB, `{}`); err == nil {
		t.Error("evento de comércio inexistente deveria violar a chave estrangeira")
	}
	if err := insertEvent(tenantA, `{invalido`); err == nil {
		t.Error("payload que não é JSON deveria violar o CHECK")
	}
	if err := insertEvent(tenantA, `{}`); err != nil {
		t.Errorf("evento válido recusado: %v", err)
	}

	// ON DELETE RESTRICT: um comércio com eventos não pode ser apagado.
	err = d.WithinTenant(ctx, tenant, func(ctx context.Context) error {
		tx, err := db.Tx(ctx)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, "DELETE FROM tenants WHERE id = ?", tenantA)
		return err
	})
	if err == nil {
		t.Error("apagar comércio com eventos deveria ser recusado")
	}
}

func TestDialectHelpers(t *testing.T) {
	t.Parallel()
	const q = "SELECT a FROM t WHERE b = ? AND c = ? LIMIT ?"
	if got := db.SQLite.Rebind(q); got != q {
		t.Errorf("SQLite.Rebind alterou a query: %s", got)
	}
	if got := db.Postgres.Rebind(q); got != "SELECT a FROM t WHERE b = $1 AND c = $2 LIMIT $3" {
		t.Errorf("Postgres.Rebind = %s", got)
	}

	manaus := time.FixedZone("AMT", -4*3600)
	instant := time.Date(2026, 11, 13, 21, 0, 0, 123456000, manaus)
	if got := db.SQLite.TimeArg(instant); got != "2026-11-14T01:00:00.123456Z" {
		t.Errorf("SQLite.TimeArg = %v", got)
	}
	if got, ok := db.Postgres.TimeArg(instant).(time.Time); !ok || got.Location() != time.UTC || !got.Equal(instant) {
		t.Errorf("Postgres.TimeArg = %v", db.Postgres.TimeArg(instant))
	}
}

func TestTimeScan(t *testing.T) {
	t.Parallel()
	want := time.Date(2026, 11, 14, 1, 0, 0, 123456000, time.UTC)

	for name, src := range map[string]any{
		"texto":     "2026-11-14T01:00:00.123456Z",
		"bytes":     []byte("2026-11-14T01:00:00.123456Z"),
		"time.Time": want.In(time.FixedZone("AMT", -4*3600)),
	} {
		var got db.Time
		if err := got.Scan(src); err != nil || !got.Valid || !got.Time.Equal(want) || got.Time.Location() != time.UTC {
			t.Errorf("%s: %+v, %v", name, got, err)
		}
	}

	null := db.Time{Time: want, Valid: true}
	if err := null.Scan(nil); err != nil || null.Valid || !null.Time.IsZero() {
		t.Errorf("NULL: %+v, %v", null, err)
	}
	var bad db.Time
	if err := bad.Scan("13/11/2026"); err == nil {
		t.Error("formato inesperado deveria falhar")
	}
	if err := bad.Scan(42); err == nil {
		t.Error("tipo inesperado deveria falhar")
	}
}

func TestSQLiteTimestampsSortChronologically(t *testing.T) {
	t.Parallel()
	// A ordenação por texto só equivale à ordenação por tempo porque o
	// formato tem largura fixa; este teste protege essa propriedade.
	base := time.Date(2026, 11, 13, 23, 59, 59, 999999000, time.UTC)
	instants := []time.Time{base, base.Add(time.Microsecond), base.Add(time.Second), base.Add(24 * time.Hour)}
	for i := 1; i < len(instants); i++ {
		prev := db.SQLite.TimeArg(instants[i-1]).(string)
		next := db.SQLite.TimeArg(instants[i]).(string)
		if !(prev < next) || len(prev) != len(next) {
			t.Errorf("%s deveria vir antes de %s", prev, next)
		}
	}
}
