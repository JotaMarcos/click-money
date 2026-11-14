package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/JotaMarcos/click-money/internal/platform/clock"
	"github.com/JotaMarcos/click-money/internal/platform/config"
	"github.com/JotaMarcos/click-money/internal/platform/dbtest"
)

func testHandler(t *testing.T, mongoURL string) http.Handler {
	t.Helper()
	cfg := config.Config{
		Env:            config.EnvDevelopment,
		DBDriver:       config.DriverSQLite,
		MongoURL:       mongoURL,
		AllowedOrigins: []string{"http://localhost:5173"},
		MaxBodyBytes:   1 << 20,
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return newHandler(cfg, log, clock.System{}, dbtest.NewSQLite(t))
}

func get(h http.Handler, target string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestHealthEndpoints(t *testing.T) {
	t.Parallel()
	h := testHandler(t, "")

	rec := get(h, "/livez", nil)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"status":"ok"`) {
		t.Errorf("/livez: %d %s", rec.Code, rec.Body.String())
	}

	rec = get(h, "/readyz", nil)
	var body struct {
		Status string            `json:"status"`
		Checks map[string]string `json:"checks"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK || body.Status != "ok" || body.Checks["sqlite"] != "ok" {
		t.Errorf("/readyz: %d %+v", rec.Code, body)
	}
	if rec.Header().Get("X-Request-ID") == "" || rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("cabeçalhos padrão ausentes: %v", rec.Header())
	}
}

func TestReadyzDegradesWithoutDocumentStore(t *testing.T) {
	t.Parallel()
	rec := get(testHandler(t, "mongodb://localhost:27017"), "/readyz", nil)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"status":"degraded"`) {
		t.Errorf("MongoDB fora deveria degradar sem derrubar: %d %s", rec.Code, rec.Body.String())
	}
}

func TestUnknownRouteAndForeignOrigin(t *testing.T) {
	t.Parallel()
	h := testHandler(t, "")

	rec := get(h, "/v1/nao-existe", nil)
	if rec.Code != http.StatusNotFound || rec.Header().Get("Content-Type") != "application/problem+json" {
		t.Errorf("rota inexistente: %d %s", rec.Code, rec.Header().Get("Content-Type"))
	}
	if !strings.Contains(rec.Body.String(), `"code":"NOT_FOUND"`) {
		t.Errorf("corpo: %s", rec.Body.String())
	}

	rec = get(h, "/readyz", map[string]string{"Origin": "https://malicioso.exemplo.net"})
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), `"code":"ORIGIN_NOT_ALLOWED"`) {
		t.Errorf("origem não permitida: %d %s", rec.Code, rec.Body.String())
	}
}

func TestRunFailsFastOnInvalidConfig(t *testing.T) {
	t.Parallel()
	getenv := func(k string) string {
		return map[string]string{"CM_ENV": "production", "CM_DB_DRIVER": "postgres"}[k]
	}
	err := run(context.Background(), getenv, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "CM_DB_URL") {
		t.Fatalf("erro = %v, quero menção a CM_DB_URL", err)
	}
}

func TestRunServesAndStopsOnCancel(t *testing.T) {
	t.Parallel()
	env := map[string]string{
		"CM_ENV":       "development",
		"CM_HTTP_ADDR": "127.0.0.1:0",
		"CM_DB_DRIVER": "sqlite",
		"CM_DB_URL":    ":memory:",
	}
	var out syncBuffer
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- run(ctx, func(k string) string { return env[k] }, &out) }()

	// O endereço real (porta escolhida pelo sistema) sai no log de subida.
	var addr string
	deadline := time.Now().Add(5 * time.Second)
	for addr == "" && time.Now().Before(deadline) {
		if _, rest, ok := strings.Cut(out.String(), "addr="); ok {
			addr, _, _ = strings.Cut(rest, " ")
		} else {
			time.Sleep(10 * time.Millisecond)
		}
	}
	if addr == "" {
		cancel()
		t.Fatalf("o servidor não anunciou o endereço:\n%s", out.String())
	}

	resp, err := http.Get("http://" + addr + "/readyz")
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("/readyz no servidor real: %d", resp.StatusCode)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("run = %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("run não retornou após o cancelamento")
	}
}

// syncBuffer é um bytes.Buffer seguro para escrita pelo servidor e leitura
// pelo teste ao mesmo tempo.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}
