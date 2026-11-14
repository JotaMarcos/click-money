package httpx

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/JotaMarcos/click-money/internal/platform/clock"
	"github.com/JotaMarcos/click-money/internal/platform/kernel"
	"github.com/JotaMarcos/click-money/internal/platform/logging"
)

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func decodeProblem(t *testing.T, rec *httptest.ResponseRecorder) Problem {
	t.Helper()
	if ct := rec.Header().Get("Content-Type"); ct != problemContentType {
		t.Fatalf("Content-Type = %q, quero %q", ct, problemContentType)
	}
	var p Problem
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatalf("corpo não é um Problem: %v\n%s", err, rec.Body.String())
	}
	return p
}

func TestProblemFromError(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{"validação", kernel.NewFieldError("amount_cents", "Informe um valor."), 400, "VALIDATION_FAILED"},
		{"não encontrado", kernel.NewError(kernel.CodeNotFound, "Cliente não encontrado."), 404, "NOT_FOUND"},
		{"proibido", kernel.NewError(kernel.CodeForbidden, "Sem permissão."), 403, "FORBIDDEN"},
		{"conflito de versão", kernel.NewError(kernel.CodeVersionConflict, "Alterado por outra pessoa."), 409, "VERSION_CONFLICT"},
		{"telefone repetido", kernel.NewError("CUSTOMER_PHONE_TAKEN", "Telefone já cadastrado."), 409, "CUSTOMER_PHONE_TAKEN"},
		{"regra de negócio", kernel.NewError("PAYMENT_EXCEEDS_BALANCE", "Pagamento maior que a dívida."), 422, "PAYMENT_EXCEEDS_BALANCE"},
		{"regra embrulhada", fmt.Errorf("caso de uso: %w", kernel.NewError("CUSTOMER_HAS_DEBT", "Cliente com dívida.")), 422, "CUSTOMER_HAS_DEBT"},
		{"corpo grande", &http.MaxBytesError{Limit: 10}, 413, "PAYLOAD_TOO_LARGE"},
		{"erro interno", errors.New("pq: connection refused to 10.0.0.5"), 500, "INTERNAL_ERROR"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := ProblemFromError(tc.err)
			if p.Status != tc.wantStatus || p.Code != tc.wantCode {
				t.Fatalf("status %d código %s; quero %d %s", p.Status, p.Code, tc.wantStatus, tc.wantCode)
			}
			if p.Title == "" || p.Type == "" {
				t.Errorf("título ou tipo vazio: %+v", p)
			}
		})
	}

	p := ProblemFromError(errors.New("pq: connection refused to 10.0.0.5"))
	if strings.Contains(p.Detail, "10.0.0.5") || strings.Contains(p.Detail, "pq") {
		t.Errorf("detalhe interno vazou para o cliente: %q", p.Detail)
	}
	field := ProblemFromError(kernel.NewFieldError("amount_cents", "Informe um valor."))
	if len(field.Errors) != 1 || field.Errors[0].Field != "amount_cents" {
		t.Errorf("erro de campo ausente: %+v", field.Errors)
	}
	if got := NewProblem("PAYMENT_EXCEEDS_BALANCE", "").Type; got != "/problems/payment-exceeds-balance" {
		t.Errorf("Type = %s", got)
	}
}

func TestWriteErrorLogsOnlyInternalErrors(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, nil))

	req := httptest.NewRequest(http.MethodGet, "/v1/customers/1", nil)
	req = req.WithContext(logging.WithRequestID(req.Context(), "req-12345678"))

	rec := httptest.NewRecorder()
	WriteError(rec, req, log, kernel.NewError(kernel.CodeNotFound, "Cliente não encontrado."))
	p := decodeProblem(t, rec)
	if rec.Code != 404 || p.RequestID != "req-12345678" || p.Instance != "/v1/customers/1" {
		t.Errorf("resposta inesperada: %d %+v", rec.Code, p)
	}
	if buf.Len() != 0 {
		t.Errorf("erro de negócio não deveria ser registrado como erro: %s", buf.String())
	}

	rec = httptest.NewRecorder()
	WriteError(rec, req, log, errors.New("falha no banco"))
	if rec.Code != 500 || !strings.Contains(buf.String(), "falha no banco") {
		t.Errorf("erro interno deveria ir para o log: %d %s", rec.Code, buf.String())
	}
	if strings.Contains(rec.Body.String(), "falha no banco") {
		t.Error("detalhe interno vazou na resposta")
	}
}

func TestWriteDataAndPage(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = req.WithContext(logging.WithRequestID(req.Context(), "req-12345678"))

	rec := httptest.NewRecorder()
	WriteData(rec, req, http.StatusCreated, map[string]int{"amount_cents": 2550})
	if rec.Code != 201 || !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("resposta inesperada: %d %s", rec.Code, rec.Header().Get("Content-Type"))
	}
	want := `{"data":{"amount_cents":2550},"meta":{"request_id":"req-12345678"}}`
	if got := strings.TrimSpace(rec.Body.String()); got != want {
		t.Errorf("corpo = %s\nquero  %s", got, want)
	}

	rec = httptest.NewRecorder()
	WritePage(rec, req, []string{"a"}, "cursor-2")
	if got := strings.TrimSpace(rec.Body.String()); !strings.Contains(got, `"next_cursor":"cursor-2","has_more":true`) {
		t.Errorf("página com continuação: %s", got)
	}
	rec = httptest.NewRecorder()
	WritePage(rec, req, []string{"a"}, "")
	got := strings.TrimSpace(rec.Body.String())
	if !strings.Contains(got, `"has_more":false`) || strings.Contains(got, "next_cursor") {
		t.Errorf("última página: %s", got)
	}
}

func TestDecodeJSON(t *testing.T) {
	t.Parallel()
	type input struct {
		Name string `json:"name"`
	}
	newReq := func(contentType, body string) *http.Request {
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		if contentType != "" {
			r.Header.Set("Content-Type", contentType)
		}
		return r
	}

	var in input
	if err := DecodeJSON(newReq("application/json; charset=utf-8", `{"name":"Zé"}`), &in); err != nil || in.Name != "Zé" {
		t.Fatalf("corpo válido: %v, %+v", err, in)
	}

	rejected := map[string]*http.Request{
		"sem content-type":     newReq("", `{"name":"x"}`),
		"content-type errado":  newReq("text/plain", `{"name":"x"}`),
		"json malformado":      newReq("application/json", `{"name":`),
		"campo desconhecido":   newReq("application/json", `{"name":"x","tenant_id":"y"}`),
		"conteúdo após objeto": newReq("application/json", `{"name":"x"}{"name":"y"}`),
		"corpo vazio":          newReq("application/json", ``),
	}
	for name, req := range rejected {
		var dst input
		if err := DecodeJSON(req, &dst); kernel.CodeOf(err) != kernel.CodeValidationFailed {
			t.Errorf("%s: erro = %v, quero VALIDATION_FAILED", name, err)
		}
	}
}

func TestBodyLimit(t *testing.T) {
	t.Parallel()
	type input struct {
		Name string `json:"name"`
	}
	handler := Chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in input
		if err := DecodeJSON(r, &in); err != nil {
			WriteError(w, r, discardLogger(), err)
			return
		}
		WriteData(w, r, http.StatusOK, in)
	}), BodyLimit(32))

	post := func(body string, declareLength bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if !declareLength {
			req.ContentLength = -1
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}

	if rec := post(`{"name":"Zé"}`, true); rec.Code != http.StatusOK {
		t.Errorf("corpo pequeno: %d %s", rec.Code, rec.Body.String())
	}
	big := `{"name":"` + strings.Repeat("a", 100) + `"}`
	for _, declared := range []bool{true, false} {
		rec := post(big, declared)
		if rec.Code != http.StatusRequestEntityTooLarge {
			t.Errorf("corpo grande (tamanho declarado=%v): status %d", declared, rec.Code)
			continue
		}
		if p := decodeProblem(t, rec); p.Code != CodePayloadTooLarge {
			t.Errorf("código = %s", p.Code)
		}
	}
}

func TestRequestID(t *testing.T) {
	t.Parallel()
	var seen string
	handler := Chain(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		seen = logging.RequestID(r.Context())
	}), RequestID())

	call := func(incoming string) string {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		if incoming != "" {
			req.Header.Set(requestIDHeader, incoming)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if got := rec.Header().Get(requestIDHeader); got != seen {
			t.Errorf("cabeçalho %q difere do contexto %q", got, seen)
		}
		return seen
	}

	if id := call(""); len(id) != 32 {
		t.Errorf("identificador gerado = %q", id)
	}
	if a, b := call(""), call(""); a == b {
		t.Error("identificadores gerados deveriam ser distintos")
	}
	if id := call("abc-123_DEF45"); id != "abc-123_DEF45" {
		t.Errorf("identificador válido do cliente deveria ser mantido: %q", id)
	}
	for _, bad := range []string{"curto", strings.Repeat("a", 65), "tem espaço aqui", "quebra\nde-linha-x", `aspas"injetadas`} {
		if id := call(bad); id == bad {
			t.Errorf("identificador inseguro %q foi aceito", bad)
		}
	}
}

func TestRecover(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, nil))
	handler := Chain(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("segredo interno")
	}), Recover(log))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	p := decodeProblem(t, rec)
	if rec.Code != 500 || p.Code != CodeInternalError {
		t.Fatalf("resposta inesperada: %d %+v", rec.Code, p)
	}
	if strings.Contains(rec.Body.String(), "segredo interno") {
		t.Error("mensagem do pânico vazou na resposta")
	}
	if !strings.Contains(buf.String(), "segredo interno") || !strings.Contains(buf.String(), "stack") {
		t.Errorf("pânico deveria ir para o log com a pilha: %s", buf.String())
	}

	abort := Chain(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic(http.ErrAbortHandler)
	}), Recover(log))
	defer func() {
		if rec := recover(); rec != http.ErrAbortHandler {
			t.Errorf("ErrAbortHandler deveria ser repassado, recebi %v", rec)
		}
	}()
	abort.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
}

func TestSecurityHeaders(t *testing.T) {
	t.Parallel()
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })

	rec := httptest.NewRecorder()
	Chain(ok, SecurityHeaders(true)).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	want := map[string]string{
		"X-Content-Type-Options":       "nosniff",
		"X-Frame-Options":              "DENY",
		"Referrer-Policy":              "no-referrer",
		"Content-Security-Policy":      "default-src 'none'; frame-ancestors 'none'",
		"Cross-Origin-Resource-Policy": "same-site",
		"Cache-Control":                "no-store",
		"Strict-Transport-Security":    "max-age=63072000; includeSubDomains",
	}
	for k, v := range want {
		if got := rec.Header().Get(k); got != v {
			t.Errorf("%s = %q, quero %q", k, got, v)
		}
	}

	rec = httptest.NewRecorder()
	Chain(ok, SecurityHeaders(false)).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Header().Get("Strict-Transport-Security") != "" {
		t.Error("HSTS não deveria ser enviado sem TLS")
	}
}

func TestCORS(t *testing.T) {
	t.Parallel()
	called := false
	handler := Chain(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}), CORS([]string{"https://app.exemplo.com.br"}))

	do := func(method, origin string, preflight bool) *httptest.ResponseRecorder {
		called = false
		req := httptest.NewRequest(method, "http://api.exemplo.com.br/v1/customers", nil)
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		if preflight {
			req.Header.Set("Access-Control-Request-Method", "POST")
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}

	t.Run("sem Origin passa direto", func(t *testing.T) {
		rec := do(http.MethodGet, "", false)
		if rec.Code != 200 || !called || rec.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Errorf("status %d, chamado %v", rec.Code, called)
		}
	})
	t.Run("origem permitida", func(t *testing.T) {
		rec := do(http.MethodGet, "https://app.exemplo.com.br", false)
		if rec.Code != 200 || !called {
			t.Fatalf("status %d, chamado %v", rec.Code, called)
		}
		if rec.Header().Get("Access-Control-Allow-Origin") != "https://app.exemplo.com.br" ||
			rec.Header().Get("Access-Control-Allow-Credentials") != "true" ||
			rec.Header().Get("Vary") != "Origin" {
			t.Errorf("cabeçalhos CORS ausentes: %v", rec.Header())
		}
	})
	t.Run("origem não permitida recebe 403 e não 500", func(t *testing.T) {
		rec := do(http.MethodPost, "https://malicioso.exemplo.net", false)
		if rec.Code != http.StatusForbidden || called {
			t.Fatalf("status %d, chamado %v", rec.Code, called)
		}
		if p := decodeProblem(t, rec); p.Code != CodeOriginNotAllowed {
			t.Errorf("código = %s", p.Code)
		}
		if rec.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Error("origem recusada não pode receber Allow-Origin")
		}
	})
	t.Run("preflight de origem não permitida recebe 403", func(t *testing.T) {
		if rec := do(http.MethodOptions, "https://malicioso.exemplo.net", true); rec.Code != http.StatusForbidden {
			t.Errorf("status %d", rec.Code)
		}
	})
	t.Run("preflight de origem permitida", func(t *testing.T) {
		rec := do(http.MethodOptions, "https://app.exemplo.com.br", true)
		if rec.Code != http.StatusNoContent || called {
			t.Fatalf("status %d, chamado %v", rec.Code, called)
		}
		if !strings.Contains(rec.Header().Get("Access-Control-Allow-Headers"), "Idempotency-Key") ||
			!strings.Contains(rec.Header().Get("Access-Control-Allow-Methods"), "PATCH") {
			t.Errorf("cabeçalhos de preflight: %v", rec.Header())
		}
	})
	t.Run("mesma origem do servidor é aceita", func(t *testing.T) {
		if rec := do(http.MethodPost, "http://api.exemplo.com.br", false); rec.Code != 200 || !called {
			t.Errorf("status %d, chamado %v", rec.Code, called)
		}
	})
	t.Run("origem com sufixo parecido é recusada", func(t *testing.T) {
		if rec := do(http.MethodGet, "https://app.exemplo.com.br.malicioso.net", false); rec.Code != http.StatusForbidden {
			t.Errorf("status %d", rec.Code)
		}
	})
}

func TestAccessLog(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	log := logging.New(&buf, slog.LevelInfo, true)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/customers/{customer_id}", func(w http.ResponseWriter, r *http.Request) {
		WriteData(w, r, http.StatusOK, "ok")
	})
	mux.HandleFunc("GET /v1/falha", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	mux.HandleFunc("GET /v1/silencioso", func(http.ResponseWriter, *http.Request) {})
	handler := Chain(mux, RequestID(), AccessLog(log, clock.System{}))

	get := func(target string) map[string]any {
		buf.Reset()
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, target, nil))
		var rec map[string]any
		if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
			t.Fatalf("log inválido: %v\n%s", err, buf.String())
		}
		return rec
	}

	rec := get("/v1/customers/01933b7e-8c1a-7c3e-9f4b-2a6d5e8f1b0c?q=92991234567")
	if rec["route"] != "GET /v1/customers/{customer_id}" || rec["status"] != float64(200) || rec["method"] != "GET" {
		t.Errorf("registro inesperado: %v", rec)
	}
	if rec["request_id"] == nil || rec["bytes"] == float64(0) {
		t.Errorf("request_id ou bytes ausentes: %v", rec)
	}
	if out := buf.String(); strings.Contains(out, "92991234567") || strings.Contains(out, "01933b7e") {
		t.Errorf("query string ou identificador vazou no log de acesso: %s", out)
	}

	if rec := get("/v1/falha"); rec["level"] != "ERROR" || rec["status"] != float64(500) {
		t.Errorf("erro 5xx deveria ser registrado como ERROR: %v", rec)
	}
	if rec := get("/v1/silencioso"); rec["status"] != float64(200) {
		t.Errorf("handler sem escrita deveria contar como 200: %v", rec)
	}
	if rec := get("/nao-existe"); rec["route"] != "sem rota" || rec["status"] != float64(404) {
		t.Errorf("rota inexistente: %v", rec)
	}
}

func TestLivenessAndReadiness(t *testing.T) {
	t.Parallel()
	up := func(context.Context) error { return nil }
	down := func(context.Context) error { return errors.New("conexão recusada") }

	call := func(h http.Handler) (int, HealthResponse) {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
		var resp HealthResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("corpo inválido: %v", err)
		}
		return rec.Code, resp
	}

	if code, resp := call(Liveness()); code != 200 || resp.Status != HealthOK {
		t.Errorf("livez: %d %+v", code, resp)
	}

	cases := []struct {
		name       string
		checks     []HealthCheck
		wantCode   int
		wantStatus string
		wantChecks map[string]string
	}{
		{
			"tudo no ar",
			[]HealthCheck{{"postgres", true, up}, {"mongodb", false, up}},
			200, HealthOK, map[string]string{"postgres": "ok", "mongodb": "ok"},
		},
		{
			"MongoDB fora degrada sem derrubar a prontidão",
			[]HealthCheck{{"postgres", true, up}, {"mongodb", false, down}},
			200, HealthDegraded, map[string]string{"postgres": "ok", "mongodb": "degraded"},
		},
		{
			"PostgreSQL fora derruba a prontidão",
			[]HealthCheck{{"postgres", true, down}, {"mongodb", false, up}},
			503, HealthUnavailable, map[string]string{"postgres": "unavailable", "mongodb": "ok"},
		},
		{
			"os dois fora",
			[]HealthCheck{{"postgres", true, down}, {"mongodb", false, down}},
			503, HealthUnavailable, map[string]string{"postgres": "unavailable", "mongodb": "degraded"},
		},
		{"sem verificações", nil, 200, HealthOK, map[string]string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			code, resp := call(Readiness(discardLogger(), tc.checks...))
			if code != tc.wantCode || resp.Status != tc.wantStatus {
				t.Fatalf("status %d %q; quero %d %q", code, resp.Status, tc.wantCode, tc.wantStatus)
			}
			for name, want := range tc.wantChecks {
				if resp.Checks[name] != want {
					t.Errorf("checks[%s] = %q, quero %q", name, resp.Checks[name], want)
				}
			}
		})
	}
}

func TestReadinessBoundsSlowChecks(t *testing.T) {
	t.Parallel()
	slow := HealthCheck{Name: "postgres", Required: true, Check: func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	}}
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	ctx, cancel := context.WithTimeout(req.Context(), 50*time.Millisecond)
	defer cancel()

	rec := httptest.NewRecorder()
	start := time.Now()
	Readiness(discardLogger(), slow).ServeHTTP(rec, req.WithContext(ctx))
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status %d", rec.Code)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("verificação lenta prendeu a sonda por %s", elapsed)
	}
}

func TestServeShutsDownGracefully(t *testing.T) {
	t.Parallel()
	started := make(chan struct{})
	release := make(chan struct{})
	mux := http.NewServeMux()
	mux.HandleFunc("GET /lento", func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release
		_, _ = w.Write([]byte("terminou"))
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := NewServer("", mux, discardLogger())
	if srv.ReadHeaderTimeout == 0 || srv.ReadTimeout == 0 || srv.WriteTimeout == 0 || srv.IdleTimeout == 0 {
		t.Fatal("o servidor precisa de todos os timeouts definidos")
	}

	ctx, cancel := context.WithCancel(context.Background())
	served := make(chan error, 1)
	go func() { served <- Serve(ctx, srv, ln, 5*time.Second, discardLogger()) }()

	body := make(chan string, 1)
	go func() {
		resp, err := http.Get("http://" + ln.Addr().String() + "/lento")
		if err != nil {
			body <- "erro: " + err.Error()
			return
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		body <- string(b)
	}()

	<-started
	cancel() // pede o encerramento com uma requisição em andamento
	select {
	case err := <-served:
		t.Fatalf("Serve retornou antes de a requisição terminar: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(release)

	if got := <-body; got != "terminou" {
		t.Errorf("a requisição em andamento deveria ser concluída, recebi %q", got)
	}
	if err := <-served; err != nil {
		t.Errorf("Serve = %v", err)
	}
}

func TestServeReportsShutdownTimeout(t *testing.T) {
	t.Parallel()
	started := make(chan struct{})
	release := make(chan struct{})
	defer close(release)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /preso", func(http.ResponseWriter, *http.Request) {
		close(started)
		<-release
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	served := make(chan error, 1)
	go func() {
		served <- Serve(ctx, NewServer("", mux, discardLogger()), ln, 50*time.Millisecond, discardLogger())
	}()
	go func() {
		resp, err := http.Get("http://" + ln.Addr().String() + "/preso")
		if err == nil {
			resp.Body.Close()
		}
	}()

	<-started
	cancel()
	if err := <-served; !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Serve = %v, quero prazo esgotado", err)
	}
}

func TestServeReportsListenerFailure(t *testing.T) {
	t.Parallel()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ln.Close() // listener já fechado: Serve precisa devolver o erro, não travar
	err = Serve(context.Background(), NewServer("", http.NewServeMux(), discardLogger()), ln, time.Second, discardLogger())
	if err == nil {
		t.Fatal("esperava erro do listener")
	}
}
