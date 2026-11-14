// Command worker executa o trabalho em segundo plano do Click-Money:
// acompanhamento e entrega da outbox e, nas próximas fases, lembretes
// agendados e projeções para o banco de documentos.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata" // imagens distroless e scratch não trazem a base de fusos

	"golang.org/x/sync/errgroup"

	"github.com/JotaMarcos/click-money/internal/platform/clock"
	"github.com/JotaMarcos/click-money/internal/platform/config"
	"github.com/JotaMarcos/click-money/internal/platform/db"
	"github.com/JotaMarcos/click-money/internal/platform/logging"
	"github.com/JotaMarcos/click-money/internal/platform/outbox"
)

// statsInterval é a frequência com que o estado da fila é registrado.
const statsInterval = 30 * time.Second

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := run(ctx, os.Getenv, os.Stdout, statsInterval)
	stop()
	if err != nil {
		fmt.Fprintln(os.Stderr, "click-money worker:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, getenv func(string) string, stdout io.Writer, interval time.Duration) error {
	cfg, err := config.Load(getenv)
	if err != nil {
		return err
	}
	log := logging.New(stdout, cfg.LogLevel, cfg.IsProduction())
	slog.SetDefault(log)

	database, err := db.Open(ctx, cfg.DBDriver, cfg.DBURL)
	if err != nil {
		return err
	}
	defer database.Close()

	// Ainda não há consumidores registrados (projeções e notificações chegam
	// nas Fases 1 e 2). Enquanto isso o Relay não entrega nada: publicar sem
	// consumidor marcaria os eventos como entregues e eles se perderiam. O
	// handler abaixo só existe para Stats; Run não é chamado.
	relay := outbox.NewRelay(database, outbox.HandlerFunc(func(context.Context, outbox.Event) error {
		return errors.New("nenhum consumidor registrado")
	}), clock.System{}, log)

	log.Info("worker no ar", "env", cfg.Env, "db", cfg.DBDriver)

	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error { return reportStats(ctx, relay, log, interval) })
	if err := g.Wait(); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	log.Info("worker encerrado")
	return nil
}

// reportStats registra periodicamente o tamanho e a idade da fila, até ctx
// ser cancelado.
func reportStats(ctx context.Context, relay *outbox.Relay, log *slog.Logger, interval time.Duration) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		stats, err := relay.Stats(ctx)
		switch {
		case err != nil && ctx.Err() == nil:
			log.ErrorContext(ctx, "falha ao ler a fila da outbox", "error", err)
		case err == nil:
			log.InfoContext(ctx, "fila da outbox",
				"pending", stats.Pending, "failed", stats.Failed,
				"oldest_pending_age", stats.OldestPendingAge.String())
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
