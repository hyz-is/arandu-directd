package directd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Text is a string field of a response. Direct Data documents its fields with
// example values only, so a field documented as a string is read whether it
// arrives as a string, a number or a boolean; null reads as "".
type Text string

// String returns the text.
func (t Text) String() string { return string(t) }

// UnmarshalJSON reads a string, a number, a boolean or null.
func (t *Text) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	switch {
	case len(b) == 0 || bytes.Equal(b, []byte("null")):
		*t = ""
	case b[0] == '"':
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*t = Text(s)
	case b[0] == '{' || b[0] == '[':
		return fmt.Errorf("directd: a text field holds %s", firstToken(b))
	default:
		*t = Text(b)
	}
	return nil
}

// Number is a numeric field of a response, kept as the text it arrived as so
// that no digit is lost: a number, or a string holding one, reads; null reads
// as "".
type Number string

// String returns the number as it arrived.
func (n Number) String() string { return string(n) }

// Int64 returns the number as an integer, and false when it is empty or not
// an integer.
func (n Number) Int64() (int64, bool) {
	v, err := strconv.ParseInt(strings.TrimSpace(string(n)), 10, 64)
	return v, err == nil
}

// Float64 returns the number, and false when it is empty or not a number. A
// decimal comma is read as a point.
func (n Number) Float64() (float64, bool) {
	v, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(string(n)), ",", "."), 64)
	return v, err == nil
}

// UnmarshalJSON reads a number, a string or null.
func (n *Number) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	switch {
	case len(b) == 0 || bytes.Equal(b, []byte("null")):
		*n = ""
	case b[0] == '"':
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*n = Number(strings.TrimSpace(s))
	case b[0] == '{' || b[0] == '[' || b[0] == 't' || b[0] == 'f':
		return fmt.Errorf("directd: a number field holds %s", firstToken(b))
	default:
		*n = Number(b)
	}
	return nil
}

// Flag is a boolean field of a response: true, false, null (false), or the
// strings "true" and "false".
type Flag bool

// UnmarshalJSON reads a boolean, null, or the strings "true" and "false".
func (f *Flag) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	switch string(b) {
	case "true", `"true"`, `"True"`, `"TRUE"`:
		*f = true
	case "false", `"false"`, `"False"`, `"FALSE"`, "null", `""`:
		*f = false
	default:
		return fmt.Errorf("directd: a boolean field holds %s", firstToken(b))
	}
	return nil
}

// firstToken names the kind of a JSON value, for an error message that must
// not carry a personal value.
func firstToken(b []byte) string {
	switch {
	case len(b) == 0:
		return "nothing"
	case b[0] == '"':
		return "a string"
	case b[0] == '{':
		return "an object"
	case b[0] == '[':
		return "an array"
	case b[0] == 't' || b[0] == 'f':
		return "a boolean"
	}
	return "a number"
}
