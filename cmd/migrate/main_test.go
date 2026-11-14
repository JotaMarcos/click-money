package main

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestMigrateUpStatusDown(t *testing.T) {
	t.Parallel()
	env := map[string]string{"CM_DB_URL": filepath.Join(t.TempDir(), "dev.db")}
	getenv := func(k string) string { return env[k] }
	ctx := context.Background()

	exec := func(arg string) string {
		t.Helper()
		var out bytes.Buffer
		if err := run(ctx, []string{arg}, getenv, &out); err != nil {
			t.Fatalf("migrate %s: %v", arg, err)
		}
		return out.String()
	}

	if out := exec("status"); !strings.Contains(out, "versão 0") || strings.Contains(out, " 0 migração(ões) pendente(s)") {
		t.Errorf("status em banco novo: %s", out)
	}
	if out := exec("up"); !strings.Contains(out, "aplicada(s)") || !strings.Contains(out, " 0 migração(ões) pendente(s)") {
		t.Errorf("up: %s", out)
	}
	if out := exec("up"); !strings.Contains(out, "0 migração(ões) aplicada(s)") {
		t.Errorf("segundo up deveria ser nulo: %s", out)
	}
	if out := exec("down"); !strings.Contains(out, "revertida") {
		t.Errorf("down: %s", out)
	}
}

func TestMigrateRejectsBadInvocation(t *testing.T) {
	t.Parallel()
	none := func(string) string { return "" }
	for _, args := range [][]string{nil, {"up", "down"}} {
		if err := run(context.Background(), args, none, &bytes.Buffer{}); !errors.Is(err, errUsage) {
			t.Errorf("args %v: erro = %v", args, err)
		}
	}

	env := map[string]string{"CM_DB_URL": filepath.Join(t.TempDir(), "dev.db")}
	if err := run(context.Background(), []string{"apagar"}, func(k string) string { return env[k] }, &bytes.Buffer{}); !errors.Is(err, errUsage) {
		t.Errorf("comando desconhecido: %v", err)
	}

	pg := func(k string) string { return map[string]string{"CM_DB_DRIVER": "postgres"}[k] }
	if err := run(context.Background(), []string{"up"}, pg, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "CM_MIGRATE_DB_URL") {
		t.Errorf("postgres sem URL: %v", err)
	}
}
