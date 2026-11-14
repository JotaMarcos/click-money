package httpx

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"
)

// Timeouts do servidor. São folgados porque o público usa 3G instável e
// enlaces de satélite, mas existem para que conexões lentas ou maliciosas
// não esgotem o servidor.
const (
	readHeaderTimeout = 10 * time.Second
	readTimeout       = 60 * time.Second
	writeTimeout      = 90 * time.Second
	idleTimeout       = 120 * time.Second
	maxHeaderBytes    = 32 << 10
)

// NewServer cria um http.Server com todos os timeouts definidos.
func NewServer(addr string, handler http.Handler, log *slog.Logger) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
		MaxHeaderBytes:    maxHeaderBytes,
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelWarn),
	}
}

// Serve atende requisições em ln até ctx ser cancelado e então encerra de
// forma graciosa: para de aceitar conexões e espera as requisições em
// andamento por até shutdownTimeout.
func Serve(ctx context.Context, srv *http.Server, ln net.Listener, shutdownTimeout time.Duration, log *slog.Logger) error {
	serveErr := make(chan error, 1)
	go func() {
		// A goroutine termina quando Shutdown ou Close fecha o listener;
		// o canal com buffer garante que ela nunca fique bloqueada.
		serveErr <- srv.Serve(ln)
	}()

	select {
	case err := <-serveErr:
		return fmt.Errorf("httpx: servidor parou: %w", err)
	case <-ctx.Done():
	}

	log.Info("encerrando servidor HTTP", "prazo", shutdownTimeout.String())
	// O contexto de encerramento não deriva de ctx, que já foi cancelado.
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		_ = srv.Close() // prazo esgotado: derruba as conexões restantes
		return fmt.Errorf("httpx: encerramento gracioso: %w", err)
	}
	if err := <-serveErr; !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("httpx: servidor parou: %w", err)
	}
	return nil
}
