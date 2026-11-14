// Command migrate aplica e reverte as migrações do banco relacional.
//
// Uso: migrate up | down | status
//
// Lê CM_DB_DRIVER (sqlite ou postgres) e a URL do banco em CM_MIGRATE_DB_URL
// ou, na falta dela, em CM_DB_URL. No PostgreSQL a URL deve ser a do usuário
// de migração, que é dono das tabelas e diferente do usuário da aplicação.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/JotaMarcos/click-money/internal/platform/config"
	"github.com/JotaMarcos/click-money/internal/platform/db"
	"github.com/JotaMarcos/click-money/internal/platform/migrate"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := run(ctx, os.Args[1:], os.Getenv, os.Stdout)
	stop()
	if err != nil {
		fmt.Fprintln(os.Stderr, "click-money migrate:", err)
		os.Exit(1)
	}
}

var errUsage = errors.New("uso: migrate up | down | status")

func run(ctx context.Context, args []string, getenv func(string) string, stdout io.Writer) error {
	if len(args) != 1 {
		return errUsage
	}

	driver := getenv("CM_DB_DRIVER")
	if driver == "" {
		driver = config.DriverSQLite
	}
	dsn := getenv("CM_MIGRATE_DB_URL")
	if dsn == "" {
		dsn = getenv("CM_DB_URL")
	}
	if dsn == "" {
		if driver != config.DriverSQLite {
			return errors.New("CM_MIGRATE_DB_URL (ou CM_DB_URL) é obrigatória")
		}
		dsn = "click_money.db"
	}

	database, err := db.Open(ctx, driver, dsn)
	if err != nil {
		return err
	}
	defer database.Close()

	m, err := migrate.New(database)
	if err != nil {
		return err
	}

	switch args[0] {
	case "up":
		applied, err := m.Up(ctx)
		if err != nil {
			return err
		}
		fmt.Fprintf(stdout, "%d migração(ões) aplicada(s)\n", applied)
	case "down":
		if err := m.Down(ctx); err != nil {
			return err
		}
		fmt.Fprintln(stdout, "última migração revertida")
	case "status":
	default:
		return errUsage
	}

	version, err := m.Version(ctx)
	if err != nil {
		return err
	}
	pending, err := m.Pending(ctx)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "banco %s na versão %d, %d migração(ões) pendente(s)\n", driver, version, pending)
	return nil
}
