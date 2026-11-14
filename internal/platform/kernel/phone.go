package kernel

import "strings"

// ErrInvalidPhone indica texto que não é um telefone reconhecível.
var ErrInvalidPhone = NewError(CodeValidationFailed, "Telefone inválido. Informe o DDD e o número.")

const brazilCountryCode = "55"

// Phone é um telefone normalizado em E.164, como "+5592991234567".
// O valor zero representa "sem telefone".
type Phone struct {
	e164 string
}

// ParsePhone normaliza um telefone digitado em formato livre.
//
// Sem código de país, o número é tratado como brasileiro: DDD válido seguido
// de 8 dígitos (fixo) ou 9 dígitos começando por 9 (celular). Com "+", aceita
// qualquer país; se for o Brasil, aplica as mesmas regras.
func ParsePhone(input string) (Phone, error) {
	s := strings.TrimSpace(input)
	international := strings.HasPrefix(s, "+")
	digits, ok := phoneDigits(strings.TrimPrefix(s, "+"))
	if !ok {
		return Phone{}, ErrInvalidPhone
	}

	if international {
		if strings.HasPrefix(digits, brazilCountryCode) {
			return brazilianPhone(digits[len(brazilCountryCode):])
		}
		if len(digits) < 8 || len(digits) > 15 || digits[0] == '0' {
			return Phone{}, ErrInvalidPhone
		}
		return Phone{e164: "+" + digits}, nil
	}

	// "092 99123-4567": o zero inicial é o prefixo de longa distância.
	digits = strings.TrimPrefix(digits, "0")
	if (len(digits) == 12 || len(digits) == 13) && strings.HasPrefix(digits, brazilCountryCode) {
		digits = digits[len(brazilCountryCode):]
	}
	return brazilianPhone(digits)
}

// String devolve o telefone em E.164, ou "" se não houver telefone.
func (p Phone) String() string { return p.e164 }

// IsZero informa se não há telefone.
func (p Phone) IsZero() bool { return p.e164 == "" }

// IsBrazilian informa se o telefone é do Brasil.
func (p Phone) IsBrazilian() bool { return strings.HasPrefix(p.e164, "+"+brazilCountryCode) }

// AreaCode devolve o DDD de um telefone brasileiro, ou "" para os demais.
func (p Phone) AreaCode() string {
	if !p.IsBrazilian() {
		return ""
	}
	return p.e164[3:5]
}

// IsNorthRegion informa se o DDD pertence à Região Norte. Serve para alertar
// sobre um provável erro de digitação, nunca para impedir o cadastro.
func (p Phone) IsNorthRegion() bool {
	_, ok := northAreaCodes[p.AreaCode()]
	return ok
}

// WhatsAppNumber devolve o número no formato exigido por links wa.me: só
// dígitos, com código do país e sem "+".
func (p Phone) WhatsAppNumber() string { return strings.TrimPrefix(p.e164, "+") }

// Masked devolve o telefone com os dígitos do meio ocultos, para uso em logs
// e telas em que o número completo não é necessário.
func (p Phone) Masked() string {
	if len(p.e164) < 8 {
		return ""
	}
	return p.e164[:5] + strings.Repeat("*", len(p.e164)-7) + p.e164[len(p.e164)-2:]
}

func brazilianPhone(national string) (Phone, error) {
	if len(national) != 10 && len(national) != 11 {
		return Phone{}, ErrInvalidPhone
	}
	if _, ok := brazilAreaCodes[national[:2]]; !ok {
		return Phone{}, ErrInvalidPhone
	}
	subscriber := national[2:]
	switch len(subscriber) {
	case 9:
		if subscriber[0] != '9' {
			return Phone{}, ErrInvalidPhone
		}
	case 8:
		if subscriber[0] < '2' || subscriber[0] > '5' {
			return Phone{}, ErrInvalidPhone
		}
	}
	return Phone{e164: "+" + brazilCountryCode + national}, nil
}

// phoneDigits remove a pontuação usual de telefones e recusa qualquer outro
// caractere.
func phoneDigits(s string) (string, bool) {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '(' || r == ')' || r == '.':
		default:
			return "", false
		}
	}
	return b.String(), b.Len() > 0
}

var northAreaCodes = map[string]struct{}{
	"63": {},                     // Tocantins
	"68": {},                     // Acre
	"69": {},                     // Rondônia
	"91": {}, "93": {}, "94": {}, // Pará
	"92": {}, "97": {}, // Amazonas
	"95": {}, // Roraima
	"96": {}, // Amapá
}

var brazilAreaCodes = func() map[string]struct{} {
	codes := []string{
		"11", "12", "13", "14", "15", "16", "17", "18", "19",
		"21", "22", "24", "27", "28",
		"31", "32", "33", "34", "35", "37", "38",
		"41", "42", "43", "44", "45", "46", "47", "48", "49",
		"51", "53", "54", "55",
		"61", "62", "63", "64", "65", "66", "67", "68", "69",
		"71", "73", "74", "75", "77", "79",
		"81", "82", "83", "84", "85", "86", "87", "88", "89",
		"91", "92", "93", "94", "95", "96", "97", "98", "99",
	}
	m := make(map[string]struct{}, len(codes))
	for _, c := range codes {
		m[c] = struct{}{}
	}
	return m
}()
