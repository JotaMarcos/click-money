package main

import (
	"bytes"
	"context"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/JotaMarcos/click-money/internal/platform/db"
	"github.com/JotaMarcos/click-money/internal/platform/migrate"
)

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

func TestWorkerReportsQueueAndStopsOnCancel(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "dev.db")

	// O worker não migra o banco; o teste faz o papel do cmd/migrate.
	setup, err := db.Open(context.Background(), "sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	m, err := migrate.New(setup)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Up(context.Background()); err != nil {
		t.Fatal(err)
	}
	setup.Close()

	env := map[string]string{"CM_DB_URL": path}
	var out syncBuffer
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- run(ctx, func(k string) string { return env[k] }, &out, 10*time.Millisecond) }()

	deadline := time.Now().Add(5 * time.Second)
	for strings.Count(out.String(), "fila da outbox") < 2 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if got := out.String(); strings.Count(got, "fila da outbox") < 2 || !strings.Contains(got, "pending=0") {
		cancel()
		t.Fatalf("o worker deveria registrar a fila periodicamente:\n%s", got)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("run = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("o worker não parou após o cancelamento")
	}
	if !strings.Contains(out.String(), "worker encerrado") {
		t.Errorf("encerramento não registrado:\n%s", out.String())
	}
}

func TestWorkerFailsFastOnInvalidConfig(t *testing.T) {
	t.Parallel()
	env := map[string]string{"CM_DB_DRIVER": "oracle"}
	err := run(context.Background(), func(k string) string { return env[k] }, io.Discard, time.Second)
	if err == nil || !strings.Contains(err.Error(), "CM_DB_DRIVER") {
		t.Fatalf("erro = %v", err)
	}
}
