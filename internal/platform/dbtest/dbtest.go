// Package dbtest fornece bancos descartáveis para testes.
package dbtest

import (
	"context"
	"testing"
	"time"

	"github.com/JotaMarcos/click-money/internal/platform/db"
	"github.com/JotaMarcos/click-money/internal/platform/kernel"
	"github.com/JotaMarcos/click-money/internal/platform/migrate"
)

// NewSQLite devolve um banco SQLite em memória, já migrado e exclusivo do
// teste. O banco é fechado ao fim do teste.
func NewSQLite(t testing.TB) *db.DB {
	t.Helper()
	ctx := context.Background()
	d, err := db.Open(ctx, string(db.SQLite), ":memory:")
	if err != nil {
		t.Fatalf("abrir SQLite em memória: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })

	m, err := migrate.New(d)
	if err != nil {
		t.Fatalf("carregar migrações: %v", err)
	}
	if _, err := m.Up(ctx); err != nil {
		t.Fatalf("aplicar migrações: %v", err)
	}
	return d
}

// InsertTenant grava um comércio mínimo e devolve o identificador dele.
func InsertTenant(t testing.TB, d *db.DB, id string) kernel.TenantID {
	t.Helper()
	tenant, err := kernel.ParseTenantID(id)
	if err != nil {
		t.Fatalf("identificador de comércio %q: %v", id, err)
	}
	now := d.Dialect().TimeArg(time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC))
	err = d.WithinTenant(context.Background(), tenant, func(ctx context.Context) error {
		tx, err := db.Tx(ctx)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, d.Dialect().Rebind(`
			INSERT INTO tenants (id, name, timezone, plan, status, created_at, updated_at)
			VALUES (?, ?, 'America/Manaus', 'FREE', 'ACTIVE', ?, ?)`),
			tenant.String(), "Comércio de teste", now, now)
		return err
	})
	if err != nil {
		t.Fatalf("gravar comércio de teste: %v", err)
	}
	return tenant
}
