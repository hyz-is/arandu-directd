package directd

import "log/slog"

// redacted is what a Token prints as, whatever the verb.
const redacted = "[redacted]"

// Token is the Direct Data API token. Direct Data authenticates every request
// with it as the TOKEN query parameter, and every query it authorizes is
// billed to its owner, so it is a credential.
//
// It never prints: String, GoString, LogValue, MarshalJSON and MarshalText
// all write "[redacted]". The client puts it into the request URL and nowhere
// else, and strips the URL from transport errors so it cannot leak through
// one.
type Token string

// String writes "[redacted]".
func (Token) String() string { return redacted }

// GoString writes "[redacted]", for %#v.
func (Token) GoString() string { return redacted }

// LogValue writes "[redacted]" in structured logs.
func (Token) LogValue() slog.Value { return slog.StringValue(redacted) }

// MarshalJSON writes "[redacted]".
func (Token) MarshalJSON() ([]byte, error) { return []byte(`"` + redacted + `"`), nil }

// MarshalText writes "[redacted]".
func (Token) MarshalText() ([]byte, error) { return []byte(redacted), nil }

// Empty reports whether no token was given.
func (t Token) Empty() bool { return t == "" }

// reveal is the token's value, for the one place that sends it.
func (t Token) reveal() string { return string(t) }
