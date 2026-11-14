package httpx

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"

	"github.com/JotaMarcos/click-money/internal/platform/kernel"
	"github.com/JotaMarcos/click-money/internal/platform/logging"
)

// Envelope é o formato de toda resposta de sucesso: { "data": ..., "meta": ... }.
type Envelope struct {
	Data any  `json:"data"`
	Meta Meta `json:"meta"`
}

// Meta carrega metadados da resposta. Os campos de paginação só aparecem em
// listas.
type Meta struct {
	RequestID  string `json:"request_id,omitempty"`
	NextCursor string `json:"next_cursor,omitempty"`
	HasMore    *bool  `json:"has_more,omitempty"`
}

// WriteData escreve data no envelope padrão com o status informado.
func WriteData(w http.ResponseWriter, r *http.Request, status int, data any) {
	writeEnvelope(w, r, status, Envelope{Data: data})
}

// WritePage escreve uma página de lista. nextCursor vazio indica a última.
func WritePage(w http.ResponseWriter, r *http.Request, items any, nextCursor string) {
	hasMore := nextCursor != ""
	writeEnvelope(w, r, http.StatusOK, Envelope{
		Data: items,
		Meta: Meta{NextCursor: nextCursor, HasMore: &hasMore},
	})
}

func writeEnvelope(w http.ResponseWriter, r *http.Request, status int, env Envelope) {
	env.Meta.RequestID = logging.RequestID(r.Context())
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(env); err != nil {
		slog.WarnContext(r.Context(), "falha ao escrever resposta", "error", err)
	}
}

// Erros de leitura do corpo.
var (
	errUnsupportedMediaType = kernel.NewError(kernel.CodeValidationFailed, "Envie o corpo como application/json.")
	errMalformedJSON        = kernel.NewError(kernel.CodeValidationFailed, "O corpo da requisição não é um JSON válido.")
)

// DecodeJSON lê o corpo JSON da requisição em dst.
//
// Recusa outro Content-Type, campos desconhecidos e conteúdo depois do
// objeto. Um corpo acima do limite do middleware BodyLimit resulta em
// *http.MaxBytesError, que ProblemFromError traduz para 413.
func DecodeJSON(r *http.Request, dst any) error {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return errUnsupportedMediaType
	}

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return err
		}
		return errMalformedJSON
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return errMalformedJSON
	}
	return nil
}
