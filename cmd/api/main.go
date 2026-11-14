// Command api é o servidor HTTP do Click-Money.
//
// Este arquivo é a raiz de composição: lê a configuração, monta as
// dependências à mão e as entrega aos módulos. Não há container de injeção.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	_ "time/tzdata" // imagens distroless e scratch não trazem a base de fusos

	"github.com/JotaMarcos/click-money/internal/platform/clock"
	"github.com/JotaMarcos/click-money/internal/platform/config"
	"github.com/JotaMarcos/click-money/internal/platform/db"
	"github.com/JotaMarcos/click-money/internal/platform/httpx"
	"github.com/JotaMarcos/click-money/internal/platform/logging"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := run(ctx, os.Getenv, os.Stdout)
	stop()
	if err != nil {
		fmt.Fprintln(os.Stderr, "click-money api:", err)
		os.Exit(1)
	}
}

// run sobe o servidor e só retorna quando ctx é cancelado ou algo falha.
func run(ctx context.Context, getenv func(string) string, stdout io.Writer) error {
	cfg, err := loadConfig(getenv)
	if err != nil {
		return err
	}
	log := logging.New(stdout, cfg.LogLevel, cfg.IsProduction())
	slog.SetDefault(log)

	database, err := db.Open(ctx, cfg.DBDriver, cfg.DBURL)
	if err != nil {
		return err
	}
	defer func() {
		if err := database.Close(); err != nil {
			log.Warn("falha ao fechar o banco", "error", err)
		}
	}()

	ln, err := net.Listen("tcp", cfg.HTTPAddr)
	if err != nil {
		return fmt.Errorf("abrir porta %s: %w", cfg.HTTPAddr, err)
	}
	log.Info("servidor HTTP no ar", "addr", ln.Addr().String(), "env", cfg.Env, "db", cfg.DBDriver)

	handler := newHandler(cfg, log, clock.System{}, database)
	return httpx.Serve(ctx, httpx.NewServer(cfg.HTTPAddr, handler, log), ln, cfg.ShutdownTimeout, log)
}

// loadConfig lê a configuração do ambiente. O arquivo .env só é considerado
// em desenvolvimento e nunca se sobrepõe a uma variável já definida.
func loadConfig(getenv func(string) string) (config.Config, error) {
	if env := getenv("CM_ENV"); env == "" || env == config.EnvDevelopment {
		f, err := os.Open(".env")
		switch {
		case err == nil:
			defer f.Close()
			vars, err := config.ParseDotEnv(f)
			if err != nil {
				return config.Config{}, err
			}
			getenv = config.WithFallback(getenv, vars)
		case !errors.Is(err, os.ErrNotExist):
			return config.Config{}, fmt.Errorf("abrir .env: %w", err)
		}
	}
	return config.Load(getenv)
}

// newHandler monta as rotas e a cadeia de middlewares.
func newHandler(cfg config.Config, log *slog.Logger, c clock.Clock, database *db.DB) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /livez", httpx.Liveness())
	mux.Handle("GET /readyz", httpx.Readiness(log, readinessChecks(cfg, database)...))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteProblem(w, r, httpx.NewProblem("NOT_FOUND", "Recurso não encontrado."))
	})

	return httpx.Chain(mux,
		httpx.RequestID(),
		httpx.AccessLog(log, c),
		httpx.Recover(log),
		httpx.SecurityHeaders(cfg.IsProduction()),
		httpx.CORS(cfg.AllowedOrigins),
		httpx.BodyLimit(cfg.MaxBodyBytes),
	)
}

// readinessChecks lista as dependências verificadas em /readyz. O banco
// relacional é obrigatório. O banco de documentos, quando configurado, é
// opcional: fora do ar ele degrada o serviço sem derrubar a prontidão.
func readinessChecks(cfg config.Config, database *db.DB) []httpx.HealthCheck {
	checks := []httpx.HealthCheck{
		{Name: cfg.DBDriver, Required: true, Check: database.Ping},
	}
	if cfg.MongoURL != "" {
		// O adaptador do MongoDB entra na Fase 2. Até lá a dependência é
		// reportada como degradada, sem derrubar a prontidão, para que a
		// configuração antecipada não passe despercebida.
		checks = append(checks, httpx.HealthCheck{
			Name:  "mongodb",
			Check: func(context.Context) error { return errors.New("adaptador do MongoDB ainda não implementado") },
		})
	}
	return checks
}
