// Package migrate aplica as migrações versionadas do banco relacional.
//
// As migrações nunca rodam na inicialização da API: são executadas pelo
// binário cmd/migrate, como etapa do pipeline, com um usuário de banco
// próprio.
package migrate

import (
	"context"
	"fmt"
	"io/fs"

	"github.com/pressly/goose/v3"

	"github.com/JotaMarcos/click-money/internal/platform/db"
	"github.com/JotaMarcos/click-money/migrations"
)

// Migrator aplica e reverte migrações em um banco.
type Migrator struct {
	provider *goose.Provider
}

// New cria um Migrator com as migrações embutidas do dialeto do banco.
func New(d *db.DB) (*Migrator, error) {
	var (
		dialect goose.Dialect
		files   fs.FS
	)
	switch d.Dialect() {
	case db.Postgres:
		dialect, files = goose.DialectPostgres, migrations.Postgres()
	case db.SQLite:
		dialect, files = goose.DialectSQLite3, migrations.SQLite()
	default:
		return nil, fmt.Errorf("migrate: dialeto desconhecido %q", d.Dialect())
	}
	provider, err := goose.NewProvider(dialect, d.SQL(), files)
	if err != nil {
		return nil, fmt.Errorf("migrate: carregar migrações: %w", err)
	}
	return &Migrator{provider: provider}, nil
}

// Up aplica todas as migrações pendentes e devolve quantas foram aplicadas.
func (m *Migrator) Up(ctx context.Context) (int, error) {
	results, err := m.provider.Up(ctx)
	if err != nil {
		return len(results), fmt.Errorf("migrate: aplicar migrações: %w", err)
	}
	return len(results), nil
}

// Down reverte a migração mais recente.
func (m *Migrator) Down(ctx context.Context) error {
	if _, err := m.provider.Down(ctx); err != nil {
		return fmt.Errorf("migrate: reverter migração: %w", err)
	}
	return nil
}

// Version devolve a versão aplicada no banco (0 se nenhuma).
func (m *Migrator) Version(ctx context.Context) (int64, error) {
	v, err := m.provider.GetDBVersion(ctx)
	if err != nil {
		return 0, fmt.Errorf("migrate: ler versão do banco: %w", err)
	}
	return v, nil
}

// Pending informa quantas migrações ainda não foram aplicadas.
func (m *Migrator) Pending(ctx context.Context) (int, error) {
	current, target, err := m.provider.GetVersions(ctx)
	if err != nil {
		return 0, fmt.Errorf("migrate: comparar versões: %w", err)
	}
	if current >= target {
		return 0, nil
	}
	pending := 0
	for _, src := range m.provider.ListSources() {
		if src.Version > current {
			pending++
		}
	}
	return pending, nil
}
