package kernel

import "errors"

// Códigos estáveis de erro compartilhados. Cada módulo acrescenta os seus;
// todos correspondem a um valor de ProblemCode no contrato da API.
const (
	CodeValidationFailed = "VALIDATION_FAILED"
	CodeNotFound         = "NOT_FOUND"
	CodeForbidden        = "FORBIDDEN"
	CodeVersionConflict  = "VERSION_CONFLICT"
)

// Error é um erro de negócio com código estável e mensagem em pt-BR própria
// para exibição ao usuário.
type Error struct {
	Code    string
	Message string
	// Field identifica o campo de entrada relacionado, quando houver.
	Field string
}

// NewError cria um erro de negócio.
func NewError(code, message string) *Error {
	return &Error{Code: code, Message: message}
}

// NewFieldError cria um erro de validação ligado a um campo de entrada.
func NewFieldError(field, message string) *Error {
	return &Error{Code: CodeValidationFailed, Message: message, Field: field}
}

func (e *Error) Error() string {
	if e.Field != "" {
		return e.Code + " (" + e.Field + "): " + e.Message
	}
	return e.Code + ": " + e.Message
}

// Is considera dois erros de negócio equivalentes quando têm o mesmo código,
// o que permite errors.Is(err, ErrAlgumaCoisa) independentemente da mensagem.
func (e *Error) Is(target error) bool {
	var t *Error
	if !errors.As(target, &t) {
		return false
	}
	return e.Code == t.Code
}

// CodeOf devolve o código de negócio de err, ou "" se err não for um Error.
func CodeOf(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}
