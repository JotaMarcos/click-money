package httpx

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"

	"github.com/JotaMarcos/click-money/internal/platform/clock"
	"github.com/JotaMarcos/click-money/internal/platform/logging"
)

// Middleware envolve um handler com um comportamento transversal.
type Middleware func(http.Handler) http.Handler

// Chain aplica os middlewares de modo que o primeiro da lista seja o mais
// externo, isto é, o primeiro a ver a requisição.
func Chain(h http.Handler, mws ...Middleware) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}

const requestIDHeader = "X-Request-ID"

// RequestID garante que toda requisição tenha um identificador, propagado no
// contexto, nos logs e no cabeçalho de resposta. Um identificador enviado
// pelo cliente só é aproveitado se for curto e de caracteres seguros, para
// que não sirva de vetor de injeção em logs.
func RequestID() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := r.Header.Get(requestIDHeader)
			if !validRequestID(id) {
				id = newRequestID()
			}
			w.Header().Set(requestIDHeader, id)
			next.ServeHTTP(w, r.WithContext(logging.WithRequestID(r.Context(), id)))
		})
	}
}

func validRequestID(id string) bool {
	if len(id) < 8 || len(id) > 64 {
		return false
	}
	for i := 0; i < len(id); i++ {
		c := id[i]
		ok := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_'
		if !ok {
			return false
		}
	}
	return true
}

func newRequestID() string {
	var b [16]byte
	// crypto/rand.Read não falha nas plataformas suportadas; se falhasse, o
	// identificador zerado ainda manteria a requisição atendível.
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// Recover transforma um pânico em resposta 500 e registra a pilha. Sem ele,
// o net/http derrubaria a conexão sem resposta e sem log estruturado.
func Recover(log *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				rec := recover()
				if rec == nil {
					return
				}
				// ErrAbortHandler é o modo oficial de abortar uma resposta;
				// precisa seguir adiante para o servidor tratar.
				if err, ok := rec.(error); ok && errors.Is(err, http.ErrAbortHandler) {
					panic(rec)
				}
				log.ErrorContext(r.Context(), "pânico ao atender requisição",
					"panic", rec, "stack", string(debug.Stack()))
				WriteProblem(w, r, NewProblem(CodeInternalError, "Não foi possível concluir a operação. Tente novamente."))
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// statusRecorder guarda o status e o tamanho da resposta para o log de acesso.
type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int64
}

func (s *statusRecorder) WriteHeader(code int) {
	if s.status == 0 {
		s.status = code
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	n, err := s.ResponseWriter.Write(b)
	s.bytes += int64(n)
	return n, err
}

// Unwrap permite que http.ResponseController alcance o writer original.
func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

// AccessLog registra uma linha por requisição. A query string fica de fora
// de propósito: buscas por nome ou telefone de cliente passam por ela.
func AccessLog(log *slog.Logger, c clock.Clock) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := c.Now()
			rec := &statusRecorder{ResponseWriter: w}
			next.ServeHTTP(rec, r)

			status := rec.status
			if status == 0 {
				status = http.StatusOK
			}
			level := slog.LevelInfo
			if status >= http.StatusInternalServerError {
				level = slog.LevelError
			}
			// r.Pattern é preenchido pelo ServeMux no mesmo *http.Request e
			// evita cardinalidade alta (o caminho real contém identificadores).
			route := r.Pattern
			if route == "" {
				route = "sem rota"
			}
			log.LogAttrs(r.Context(), level, "requisição atendida",
				slog.String("method", r.Method),
				slog.String("route", route),
				slog.Int("status", status),
				slog.Int64("bytes", rec.bytes),
				slog.Duration("duration", c.Now().Sub(start)),
			)
		})
	}
}

// SecurityHeaders define os cabeçalhos de segurança de uma API que só serve
// JSON. Com hsts, acrescenta Strict-Transport-Security (ambientes com TLS).
func SecurityHeaders(hsts bool) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Referrer-Policy", "no-referrer")
			h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
			h.Set("Cross-Origin-Resource-Policy", "same-site")
			h.Set("Permissions-Policy", "geolocation=(), camera=(), microphone=()")
			// Respostas da API carregam dados financeiros de clientes: nada
			// de cache compartilhado. Handlers com ETag sobrescrevem.
			h.Set("Cache-Control", "no-store")
			if hsts {
				h.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
			}
			next.ServeHTTP(w, r)
		})
	}
}

const corsAllowedHeaders = "Content-Type, X-CSRF-Token, Idempotency-Key, If-None-Match, X-Request-ID"

// CORS aceita apenas as origens da lista (ou a mesma origem do servidor).
// Uma origem fora da lista recebe 403, nunca 500, e o handler não executa.
func CORS(allowedOrigins []string) Middleware {
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, o := range allowedOrigins {
		allowed[strings.ToLower(o)] = struct{}{}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin == "" {
				// Sem Origin não é uma requisição de navegador entre origens.
				next.ServeHTTP(w, r)
				return
			}

			w.Header().Add("Vary", "Origin")
			_, listed := allowed[strings.ToLower(origin)]
			if !listed && !sameOrigin(origin, r) {
				WriteProblem(w, r, NewProblem(CodeOriginNotAllowed, "Origem não permitida."))
				return
			}

			h := w.Header()
			h.Set("Access-Control-Allow-Origin", origin)
			h.Set("Access-Control-Allow-Credentials", "true")
			h.Set("Access-Control-Expose-Headers", "ETag, X-Request-ID, Retry-After")

			if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
				h.Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
				h.Set("Access-Control-Allow-Headers", corsAllowedHeaders)
				h.Set("Access-Control-Max-Age", "600")
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// sameOrigin informa se origin aponta para o próprio servidor, caso em que o
// navegador envia Origin em POSTs mesmo sem haver cruzamento de origem.
func sameOrigin(origin string, r *http.Request) bool {
	_, host, ok := strings.Cut(origin, "://")
	return ok && strings.EqualFold(host, r.Host)
}

// BodyLimit recusa corpos maiores que maxBytes. Quando o tamanho é declarado
// em Content-Length a recusa é imediata; caso contrário acontece na leitura,
// e DecodeJSON devolve o erro que vira 413.
func BodyLimit(maxBytes int64) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.ContentLength > maxBytes {
				WriteProblem(w, r, NewProblem(CodePayloadTooLarge, "O corpo da requisição passa do limite permitido."))
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			next.ServeHTTP(w, r)
		})
	}
}
