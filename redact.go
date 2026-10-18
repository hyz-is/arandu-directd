package directd

import (
	"bytes"
	"encoding/json"
	"regexp"
	"sort"

	"github.com/hyz-is/arandu-directd/internal/pii"
)

// Redact rewrites a Direct Data answer so that it can be logged, shown in a
// support ticket or kept as a fixture without the personal data in it.
//
// The structure is kept: every key, every array and every boolean. A string
// under a key that holds personal data becomes a placeholder naming its kind
// ("<name>", "<address>", "<phone>", "<email>", "<birthdate>", "<vehicle>",
// "<network>", "<account>"), a CPF or CNPJ becomes its masked form (or
// "<document>"), and a number under such a key becomes 0. A string under a
// key that describes a status, a category or a count (situacaoCadastral,
// resultado, cnaeDescricao, ...) is kept, unless it holds a run of nine or
// more digits or an "@", which become "<redacted>". Every other string
// becomes "<string>".
//
// When the input is not JSON it returns nil and false: a body that cannot be
// read cannot be shown to be clean, so none of it is returned.
func Redact(body []byte) ([]byte, bool) {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, false
	}
	v = redactValue("", v)
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, false
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), true
}

// digitRun is a run of nine or more digits, punctuation allowed between them,
// which is what a document or a phone number looks like inside free text.
var digitRun = regexp.MustCompile(`\d[\d.\-/ ]{7,}\d`)

// emailLike is anything with an at sign between non-spaces.
var emailLike = regexp.MustCompile(`\S+@\S+`)

func redactValue(key string, v any) any {
	switch v := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			v[k] = redactValue(k, v[k])
		}
		return v
	case []any:
		for i := range v {
			v[i] = redactValue(key, v[i])
		}
		return v
	case string:
		return redactString(key, v)
	case json.Number:
		if pii.Of(key) != pii.ClassNone {
			return json.Number("0")
		}
		return v
	}
	return v
}

func redactString(key, s string) string {
	if s == "" {
		return s
	}
	switch c := pii.Of(key); c {
	case pii.ClassNone:
	case pii.ClassDocument:
		if m := MaskDocument(s); m != "" {
			return m
		}
		return "<document>"
	default:
		return "<" + string(c) + ">"
	}
	if !pii.Keep(key) {
		return "<string>"
	}
	if emailLike.MatchString(s) || digitRun.MatchString(s) {
		return "<redacted>"
	}
	return s
}
