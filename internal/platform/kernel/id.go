package kernel

import (
	"encoding/hex"
	"time"
)

// ErrInvalidID indica texto que não é um UUID.
var ErrInvalidID = NewError(CodeValidationFailed, "Identificador inválido.")

// ID é um UUID de 128 bits. As entidades do sistema usam a versão 7
// (RFC 9562), que embute o instante de criação e ordena por tempo.
type ID [16]byte

// ParseID lê um UUID no formato canônico de 36 caracteres.
func ParseID(s string) (ID, error) {
	var id ID
	if len(s) != 36 || s[8] != '-' || s[13] != '-' || s[18] != '-' || s[23] != '-' {
		return ID{}, ErrInvalidID
	}
	compact := s[0:8] + s[9:13] + s[14:18] + s[19:23] + s[24:36]
	if _, err := hex.Decode(id[:], []byte(compact)); err != nil {
		return ID{}, ErrInvalidID
	}
	return id, nil
}

// String devolve o UUID no formato canônico, em minúsculas.
func (id ID) String() string {
	var buf [36]byte
	hex.Encode(buf[0:8], id[0:4])
	buf[8] = '-'
	hex.Encode(buf[9:13], id[4:6])
	buf[13] = '-'
	hex.Encode(buf[14:18], id[6:8])
	buf[18] = '-'
	hex.Encode(buf[19:23], id[8:10])
	buf[23] = '-'
	hex.Encode(buf[24:36], id[10:16])
	return string(buf[:])
}

// IsZero informa se o identificador é o UUID nulo.
func (id ID) IsZero() bool { return id == ID{} }

// Version devolve a versão do UUID.
func (id ID) Version() int { return int(id[6] >> 4) }

// IsV7 informa se o identificador é um UUID versão 7 com a variante da RFC.
func (id ID) IsV7() bool { return id.Version() == 7 && id[8]&0xC0 == 0x80 }

// Time devolve o instante embutido em um UUIDv7. Para outras versões o
// resultado não tem significado.
func (id ID) Time() time.Time {
	ms := int64(id[0])<<40 | int64(id[1])<<32 | int64(id[2])<<24 |
		int64(id[3])<<16 | int64(id[4])<<8 | int64(id[5])
	return time.UnixMilli(ms).UTC()
}

// Compare ordena identificadores byte a byte; para UUIDv7 isso equivale à
// ordem de criação.
func (id ID) Compare(o ID) int {
	for i := range id {
		if id[i] != o[i] {
			return cmpInt(int(id[i]), int(o[i]))
		}
	}
	return 0
}

// MarshalText implementa encoding.TextMarshaler.
func (id ID) MarshalText() ([]byte, error) { return []byte(id.String()), nil }

// UnmarshalText implementa encoding.TextUnmarshaler.
func (id *ID) UnmarshalText(b []byte) error {
	parsed, err := ParseID(string(b))
	if err != nil {
		return err
	}
	*id = parsed
	return nil
}

// TenantID identifica um comércio. É um tipo distinto de ID para que o
// compilador impeça passar o identificador de outra entidade no lugar dele.
type TenantID ID

// ParseTenantID lê o identificador de um comércio.
func ParseTenantID(s string) (TenantID, error) {
	id, err := ParseID(s)
	return TenantID(id), err
}

// String devolve o identificador no formato canônico.
func (t TenantID) String() string { return ID(t).String() }

// IsZero informa se o identificador é nulo.
func (t TenantID) IsZero() bool { return ID(t).IsZero() }
