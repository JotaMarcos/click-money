package httpx

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"golang.org/x/sync/errgroup"
)

// Estados de saúde, conforme o contrato da API.
const (
	HealthOK          = "ok"
	HealthDegraded    = "degraded"
	HealthUnavailable = "unavailable"
)

// HealthCheck verifica uma dependência do serviço.
type HealthCheck struct {
	// Name identifica a dependência na resposta, como "postgres".
	Name string
	// Required indica que, sem essa dependência, o serviço não está pronto.
	// Uma dependência opcional indisponível apenas degrada o serviço.
	Required bool
	// Check devolve nil quando a dependência responde.
	Check func(ctx context.Context) error
}

// HealthResponse é o corpo de /livez e /readyz.
type HealthResponse struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks,omitempty"`
}

// healthCheckTimeout limita cada verificação, para que uma dependência
// lenta não prenda a sonda do orquestrador.
const healthCheckTimeout = 2 * time.Second

// Liveness responde 200 enquanto o processo consegue atender requisições.
// Não consulta dependências: reiniciar o processo não conserta um banco fora.
func Liveness() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeHealth(w, http.StatusOK, HealthResponse{Status: HealthOK})
	})
}

// Readiness executa as verificações em paralelo.
//
// Responde 503 se alguma dependência obrigatória falhar, e 200 com status
// "degraded" se só dependências opcionais falharem.
func Readiness(log *slog.Logger, checks ...HealthCheck) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), healthCheckTimeout)
		defer cancel()

		results := make([]string, len(checks))
		var g errgroup.Group
		for i, c := range checks {
			g.Go(func() error {
				results[i] = HealthOK
				if err := c.Check(ctx); err != nil {
					results[i] = HealthUnavailable
					if !c.Required {
						results[i] = HealthDegraded
					}
					log.WarnContext(ctx, "verificação de saúde falhou",
						"check", c.Name, "required", c.Required, "error", err)
				}
				return nil
			})
		}
		_ = g.Wait() // as goroutines nunca devolvem erro; o resultado está em results

		resp := HealthResponse{Status: HealthOK, Checks: make(map[string]string, len(checks))}
		status := http.StatusOK
		for i, c := range checks {
			resp.Checks[c.Name] = results[i]
			switch results[i] {
			case HealthUnavailable:
				resp.Status, status = HealthUnavailable, http.StatusServiceUnavailable
			case HealthDegraded:
				if resp.Status == HealthOK {
					resp.Status = HealthDegraded
				}
			}
		}
		writeHealth(w, status, resp)
	})
}

func writeHealth(w http.ResponseWriter, status int, resp HealthResponse) {
	h := w.Header()
	h.Set("Content-Type", "application/json; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(resp) // falha de escrita: o cliente já se foi
}
