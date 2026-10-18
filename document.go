package directd

import (
	"errors"
	"fmt"

	br "github.com/hyz-is/arandu-br"
)

// ErrInvalidDocument is a document that is not a valid CPF or CNPJ, or not
// one the product takes. It is refused before any request, reservation or
// lookup.
var ErrInvalidDocument = errors.New("directd: invalid document")

// DocumentKind is what a document is.
type DocumentKind string

// The document kinds.
const (
	// KindCPF is a CPF.
	KindCPF DocumentKind = "cpf"
	// KindCNPJ is a CNPJ, numeric or alphanumeric.
	KindCNPJ DocumentKind = "cnpj"
)

// parseDocument reads a CPF or a CNPJ, bare or punctuated, checking the check
// digits with arandu-br, and returns it bare.
func parseDocument(s string) (string, DocumentKind, error) {
	if d, err := br.ParseCPF(s); err == nil {
		return d, KindCPF, nil
	}
	if d, err := br.ParseCNPJ(s); err == nil {
		return d, KindCNPJ, nil
	}
	return "", "", fmt.Errorf("%w: not a CPF or a CNPJ with valid check digits", ErrInvalidDocument)
}

// MaskDocument writes a CPF with only its check digits shown
// (***.***.***-09) and a CNPJ with its first two and last two characters
// shown (11.***.***/****-81). Anything else comes back as "".
func MaskDocument(s string) string {
	if d, err := br.ParseCPF(s); err == nil {
		return br.RedactCPF(d, br.KeepCPFCheckDigits)
	}
	if d, err := br.ParseCNPJ(s); err == nil {
		return br.RedactCNPJ(d, br.KeepCNPJEnds)
	}
	if v := br.RedactCPF(s, br.KeepCPFCheckDigits); v != "" {
		return v
	}
	return br.RedactCNPJ(s, br.KeepCNPJEnds)
}
