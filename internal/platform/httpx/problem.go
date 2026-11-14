// Package httpx reúne a infraestrutura HTTP compartilhada: servidor com
// timeouts, middlewares, respostas no envelope padrão, erros no formato
// Problem Details (RFC 9457) e verificações de saúde.
package httpx

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/JotaMarcos/click-money/internal/platform/kernel"
	"github.com/JotaMarcos/click-money/internal/platform/logging"
)

// Códigos de erro produzidos pela própria camada HTTP.
const (
	CodeUnauthenticated  = "UNAUTHENTICATED"
	CodeOriginNotAllowed = "ORIGIN_NOT_ALLOWED"
	CodeRateLimited      = "RATE_LIMITED"
	CodePayloadTooLarge  = "PAYLOAD_TOO_LARGE"
	CodeInternalError    = "INTERNAL_ERROR"
)

const problemContentType = "application/problem+json"

// Problem é o corpo de erro da API (RFC 9457) com as extensões do
// Click-Money: code, request_id e errors.
type Problem struct {
	Type      string       `json:"type"`
	Title     string       `json:"title"`
	Status    int          `json:"status"`
	Detail    string       `json:"detail,omitempty"`
	Instance  string       `json:"instance,omitempty"`
	Code      string       `json:"code"`
	RequestID string       `json:"request_id,omitempty"`
	Errors    []FieldError `json:"errors,omitempty"`
}

// FieldError descreve um problema em um campo de entrada.
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// statusByCode define o status HTTP dos códigos que não são regra de negócio
// comum. Qualquer outro código de negócio responde 422.
var statusByCode = map[string]int{
	kernel.CodeValidationFailed:    http.StatusBadRequest,
	CodeUnauthenticated:            http.StatusUnauthorized,
	kernel.CodeForbidden:           http.StatusForbidden,
	CodeOriginNotAllowed:           http.StatusForbidden,
	"DEVICE_REVOKED":               http.StatusForbidden,
	kernel.CodeNotFound:            http.StatusNotFound,
	kernel.CodeVersionConflict:     http.StatusConflict,
	"IDEMPOTENCY_KEY_REUSED":       http.StatusConflict,
	"CUSTOMER_PHONE_TAKEN":         http.StatusConflict,
	"REVIEW_ITEM_ALREADY_RESOLVED": http.StatusConflict,
	CodePayloadTooLarge:            http.StatusRequestEntityTooLarge,
	CodeRateLimited:                http.StatusTooManyRequests,
	CodeInternalError:              http.StatusInternalServerError,
}

var titleByStatus = map[int]string{
	http.StatusBadRequest:            "Requisição inválida",
	http.StatusUnauthorized:          "Não autenticado",
	http.StatusForbidden:             "Acesso negado",
	http.StatusNotFound:              "Não encontrado",
	http.StatusMethodNotAllowed:      "Método não permitido",
	http.StatusConflict:              "Conflito",
	http.StatusRequestEntityTooLarge: "Requisição grande demais",
	http.StatusUnprocessableEntity:   "Operação não permitida",
	http.StatusTooManyRequests:       "Muitas requisições",
	http.StatusInternalServerError:   "Erro interno",
	http.StatusServiceUnavailable:    "Serviço indisponível",
}

// NewProblem monta um Problem para o código informado, com o status e o
// título correspondentes.
func NewProblem(code, detail string) Problem {
	status, ok := statusByCode[code]
	if !ok {
		status = http.StatusUnprocessableEntity
	}
	return Problem{
		Type:   "/problems/" + strings.ReplaceAll(strings.ToLower(code), "_", "-"),
		Title:  titleByStatus[status],
		Status: status,
		Detail: detail,
		Code:   code,
	}
}

// ProblemFromError converte um erro em Problem.
//
// Erros de negócio (kernel.Error) viram a resposta correspondente ao código.
// Qualquer outro erro vira 500 com mensagem genérica: o detalhe interno vai
// para o log, nunca para o cliente.
func ProblemFromError(err error) Problem {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		return NewProblem(CodePayloadTooLarge, "O corpo da requisição passa do limite permitido.")
	}

	var bizErr *kernel.Error
	if !errors.As(err, &bizErr) {
		return NewProblem(CodeInternalError, "Não foi possível concluir a operação. Tente novamente.")
	}
	p := NewProblem(bizErr.Code, bizErr.Message)
	if bizErr.Field != "" {
		p.Errors = []FieldError{{Field: bizErr.Field, Message: bizErr.Message}}
	}
	return p
}

// WriteProblem escreve p como application/problem+json, preenchendo o
// identificador e o caminho da requisição.
func WriteProblem(w http.ResponseWriter, r *http.Request, p Problem) {
	p.RequestID = logging.RequestID(r.Context())
	p.Instance = r.URL.Path

	h := w.Header()
	h.Set("Content-Type", problemContentType)
	h.Set("Cache-Control", "no-store")
	w.WriteHeader(p.Status)
	if err := json.NewEncoder(w).Encode(p); err != nil {
		slog.WarnContext(r.Context(), "falha ao escrever resposta de erro", "error", err)
	}
}

// WriteError converte err em Problem e o escreve. Erros internos são
// registrados em log com o detalhe completo.
func WriteError(w http.ResponseWriter, r *http.Request, log *slog.Logger, err error) {
	p := ProblemFromError(err)
	if p.Status >= http.StatusInternalServerError {
		log.ErrorContext(r.Context(), "erro interno ao atender requisição", "error", err)
	}
	WriteProblem(w, r, p)
}
