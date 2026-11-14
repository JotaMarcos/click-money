package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func decode(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	var rec map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatalf("log não é JSON válido: %v\n%s", err, buf.String())
	}
	return rec
}

func TestContextIdentifiersAreAttached(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	log := New(&buf, slog.LevelInfo, true)

	ctx := WithRequestID(context.Background(), "req-1")
	ctx = WithTenantID(ctx, "tenant-1")
	ctx = WithTraceID(ctx, "trace-1")
	log.With("componente", "teste").WithGroup("g").InfoContext(ctx, "olá", "n", 1)

	rec := decode(t, &buf)
	if rec["componente"] != "teste" || rec["msg"] != "olá" {
		t.Errorf("registro inesperado: %v", rec)
	}
	group, _ := rec["g"].(map[string]any)
	for key, want := range map[string]string{"request_id": "req-1", "tenant_id": "tenant-1", "trace_id": "trace-1"} {
		if group[key] != want {
			t.Errorf("%s = %v, quero %s (registro: %v)", key, group[key], want, rec)
		}
	}
}

func TestContextWithoutIdentifiersAddsNothing(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	New(&buf, slog.LevelInfo, true).InfoContext(context.Background(), "sem contexto")
	rec := decode(t, &buf)
	for _, key := range []string{"request_id", "tenant_id", "trace_id"} {
		if _, ok := rec[key]; ok {
			t.Errorf("%s não deveria aparecer: %v", key, rec)
		}
	}
	if RequestID(context.Background()) != "" || TenantID(context.Background()) != "" || TraceID(context.Background()) != "" {
		t.Error("contexto vazio deveria devolver identificadores vazios")
	}
}

func TestSensitiveAttributesAreRedacted(t *testing.T) {
	t.Parallel()
	const (
		password = "s3nh4-muito-secreta"
		token    = "tok_abcdef123456"
		phone    = "+5592991234567"
		email    = "raimundo@exemplo.com.br"
	)
	var buf bytes.Buffer
	log := New(&buf, slog.LevelInfo, true)
	log.Info("login",
		"password", password,
		"refresh_token", token,
		"Authorization", "Bearer "+token,
		"Set-Cookie", "cm_access="+token,
		"X-CSRF-Token", token,
		"pix_key", phone,
		"db_url", "postgres://app:"+password+"@db/click_money",
		"phone", phone,
		"customer_email", email,
		slog.Group("cliente", "telefone", phone, "senha", password),
		"amount_cents", 2550,
	)

	out := buf.String()
	for _, secret := range []string{password, token, phone, email} {
		if strings.Contains(out, secret) {
			t.Errorf("valor sensível %q vazou no log:\n%s", secret, out)
		}
	}

	rec := decode(t, &buf)
	if rec["password"] != Redacted || rec["refresh_token"] != Redacted || rec["Authorization"] != Redacted {
		t.Errorf("segredos deveriam ser redigidos por completo: %v", rec)
	}
	if rec["phone"] != "***67" || rec["customer_email"] != "***br" {
		t.Errorf("dados pessoais deveriam ser mascarados: phone=%v email=%v", rec["phone"], rec["customer_email"])
	}
	group, _ := rec["cliente"].(map[string]any)
	if group["telefone"] != "***67" || group["senha"] != Redacted {
		t.Errorf("atributos em grupo deveriam ser protegidos: %v", group)
	}
	if rec["amount_cents"] != float64(2550) {
		t.Errorf("atributo comum alterado: %v", rec["amount_cents"])
	}
}

func TestShortPersonalDataIsFullyRedacted(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	New(&buf, slog.LevelInfo, true).Info("x", "phone", "9267")
	if rec := decode(t, &buf); rec["phone"] != Redacted {
		t.Errorf("phone = %v", rec["phone"])
	}
}

func TestTextOutputAndLevel(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	log := New(&buf, slog.LevelWarn, false)
	log.Info("ignorado")
	log.Warn("registrado", "senha", "abc12345")
	out := buf.String()
	if strings.Contains(out, "ignorado") || !strings.Contains(out, "registrado") {
		t.Errorf("nível não respeitado:\n%s", out)
	}
	if strings.Contains(out, "abc12345") || strings.HasPrefix(out, "{") {
		t.Errorf("saída de texto inesperada:\n%s", out)
	}
}
