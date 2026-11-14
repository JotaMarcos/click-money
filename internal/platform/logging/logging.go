// Package logging configura o log estruturado (log/slog) do sistema.
//
// Duas garantias valem para todo registro: os identificadores de correlação
// presentes no contexto (request_id, tenant_id, trace_id) são anexados
// automaticamente, e atributos com dados pessoais ou segredos são redigidos
// antes de sair do processo.
package logging

import (
	"context"
	"io"
	"log/slog"
	"strings"
)

// Redacted é o texto que substitui valores sensíveis.
const Redacted = "[REDIGIDO]"

type ctxKey int

const (
	keyRequestID ctxKey = iota
	keyTenantID
	keyTraceID
)

// WithRequestID devolve um contexto que carrega o identificador da requisição.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, keyRequestID, id)
}

// RequestID devolve o identificador da requisição guardado no contexto.
func RequestID(ctx context.Context) string { return stringFrom(ctx, keyRequestID) }

// WithTenantID devolve um contexto que carrega o identificador do comércio.
func WithTenantID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, keyTenantID, id)
}

// TenantID devolve o identificador do comércio guardado no contexto.
func TenantID(ctx context.Context) string { return stringFrom(ctx, keyTenantID) }

// WithTraceID devolve um contexto que carrega o identificador do trace.
func WithTraceID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, keyTraceID, id)
}

// TraceID devolve o identificador do trace guardado no contexto.
func TraceID(ctx context.Context) string { return stringFrom(ctx, keyTraceID) }

func stringFrom(ctx context.Context, key ctxKey) string {
	v, _ := ctx.Value(key).(string)
	return v
}

// New cria o logger do processo. Com jsonOutput o formato é JSON (produção);
// sem ele, texto legível (desenvolvimento).
func New(w io.Writer, level slog.Leveler, jsonOutput bool) *slog.Logger {
	opts := &slog.HandlerOptions{Level: level, ReplaceAttr: redact}
	var h slog.Handler
	if jsonOutput {
		h = slog.NewJSONHandler(w, opts)
	} else {
		h = slog.NewTextHandler(w, opts)
	}
	return slog.New(&contextHandler{Handler: h})
}

// contextHandler anexa a cada registro os identificadores de correlação
// presentes no contexto.
type contextHandler struct {
	slog.Handler
}

func (h *contextHandler) Handle(ctx context.Context, rec slog.Record) error {
	if id := RequestID(ctx); id != "" {
		rec.AddAttrs(slog.String("request_id", id))
	}
	if id := TenantID(ctx); id != "" {
		rec.AddAttrs(slog.String("tenant_id", id))
	}
	if id := TraceID(ctx); id != "" {
		rec.AddAttrs(slog.String("trace_id", id))
	}
	return h.Handler.Handle(ctx, rec)
}

func (h *contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &contextHandler{Handler: h.Handler.WithAttrs(attrs)}
}

func (h *contextHandler) WithGroup(name string) slog.Handler {
	return &contextHandler{Handler: h.Handler.WithGroup(name)}
}

// secretFragments são trechos de nome de atributo cujo valor nunca pode ser
// registrado.
var secretFragments = []string{
	"password", "senha", "token", "secret", "segredo", "cookie",
	"authorization", "csrf", "pix_key", "chave_pix", "dsn", "db_url", "mongo_url",
}

// maskedFragments são trechos de nome de atributo com dado pessoal que pode
// ser registrado parcialmente, para diagnóstico.
var maskedFragments = []string{"phone", "telefone", "email", "e-mail"}

// redact é o ReplaceAttr aplicado a todos os atributos. A decisão é pelo
// nome do atributo: quem registra um dado pessoal sob um nome neutro burla a
// proteção, por isso os nomes acima devem ser usados de forma consistente.
func redact(_ []string, a slog.Attr) slog.Attr {
	key := strings.ToLower(a.Key)
	for _, frag := range secretFragments {
		if strings.Contains(key, frag) {
			return slog.String(a.Key, Redacted)
		}
	}
	for _, frag := range maskedFragments {
		if strings.Contains(key, frag) {
			return slog.String(a.Key, mask(a.Value.String()))
		}
	}
	return a
}

// mask preserva os dois últimos caracteres de um valor e oculta o restante.
func mask(v string) string {
	const visible = 2
	if len(v) <= visible+2 {
		return Redacted
	}
	return "***" + v[len(v)-visible:]
}
