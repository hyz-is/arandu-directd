package directd

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// The defaults for the optional settings. They are constants rather than
// literals inside Config.WithDefaults, so the value a reader finds here is the
// value the package uses.
const (
	// DefaultBaseURL is Direct Data's API host.
	DefaultBaseURL = "https://apiv3.directd.com.br"
	// DefaultTimeout bounds one request when Config leaves Timeout at zero.
	DefaultTimeout = 30 * time.Second
	// DefaultBreakerThreshold is how many consecutive transport errors or 5xx
	// answers open the circuit breaker.
	DefaultBreakerThreshold = 5
	// DefaultBreakerCooldown is how long an open breaker refuses calls.
	DefaultBreakerCooldown = 30 * time.Second
	// DefaultMaxBodyBytes bounds the answer the client reads.
	DefaultMaxBodyBytes = 8 << 20
)

// brasilia is the default day boundary for Dedupe. Brazil has kept no
// daylight saving time since 2019, so a fixed offset is exact and needs no
// time zone database.
var brasilia = time.FixedZone("BRT", -3*60*60)

// Config is what the application passes when it wires this package.
//
// Every value comes from the application's own typed configuration. This
// package reads no environment variable and no file. A field left at its zero
// value takes the default named beside it.
type Config struct {
	// BaseURL is the API host. Default DefaultBaseURL. It must be https,
	// except on a loopback address, which is what a test double listens on.
	BaseURL string
	// Token is the API token, sent as the TOKEN query parameter, as the
	// documentation's introduction says. Required.
	Token Token
	// HTTPClient makes the requests. Default a new http.Client. The client
	// copies it and refuses every redirect, so the token in the URL never
	// travels to another address.
	HTTPClient *http.Client
	// Timeout bounds one request, on top of the context's own deadline.
	// Default DefaultTimeout.
	Timeout time.Duration
	// Products overrides or adds products by slug over DefaultProducts: a
	// contract price, a disabled API, an API published after the catalogue.
	Products map[string]Product
	// Breaker configures the circuit breaker.
	Breaker BreakerConfig
	// Location is where a day begins and ends for Dedupe, since Direct Data
	// bills a same-day repeat. Default UTC-3 (Brasília).
	Location *time.Location
	// Logger receives one record per call, with the product, the masked
	// document, the outcome, the HTTP status, the elapsed time and the
	// consultaUid. Never the token, the URL or a response value. Default: no
	// logging.
	Logger *slog.Logger
	// ResultCodes maps metaDados.resultadoId values to outcomes. The public
	// documentation lists none, so it is empty by default; the application
	// fills it from what it observes.
	ResultCodes map[int]Outcome
	// CallbackVerifier authenticates an asynchronous callback before
	// ParseCallback reads it. The documentation does not say how Direct Data
	// authenticates callbacks, so without one ParseCallback refuses every
	// callback.
	CallbackVerifier CallbackVerifier
	// CallbackURLParam is the query parameter that carries a per-request
	// callback URL. The documentation names none -- the callback appears to be
	// configured in the portal -- so it is empty by default and
	// AsyncOptions.CallbackURL is refused while it is.
	CallbackURLParam string
	// MaxBodyBytes bounds the answer the client reads. Default
	// DefaultMaxBodyBytes.
	MaxBodyBytes int64
	// Now is the clock. Default time.Now.
	Now func() time.Time
}

// LogValue writes the configuration without the token.
func (c Config) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("base_url", c.BaseURL),
		slog.Bool("token_set", !c.Token.Empty()),
		slog.Duration("timeout", c.Timeout),
		slog.Int("products_overridden", len(c.Products)),
	)
}

// Validate reports what the configuration cannot be used with. New calls it,
// so a setting that cannot work fails where it is wired.
func (c Config) Validate() error {
	if c.Token.Empty() {
		return errors.New("directd: Config.Token is required: Direct Data authenticates every request with it")
	}
	if strings.ContainsAny(c.Token.reveal(), " \t\r\n") {
		return errors.New("directd: Config.Token holds whitespace; read it trimmed")
	}
	if c.BaseURL != "" {
		u, err := url.Parse(c.BaseURL)
		if err != nil || u.Host == "" {
			return fmt.Errorf("directd: Config.BaseURL %q is not a URL", c.BaseURL)
		}
		if u.Scheme != "https" && !(u.Scheme == "http" && loopback(u.Hostname())) {
			return fmt.Errorf("directd: Config.BaseURL %q must be https: the token travels in the URL", c.BaseURL)
		}
		if u.RawQuery != "" || u.User != nil {
			return fmt.Errorf("directd: Config.BaseURL %q must carry no query and no user", c.BaseURL)
		}
	}
	if c.Timeout < 0 || c.Breaker.Threshold < 0 || c.Breaker.Cooldown < 0 || c.MaxBodyBytes < 0 {
		return errors.New("directd: Config durations, thresholds and sizes cannot be negative")
	}
	for slug, p := range c.Products {
		if p.Slug != "" && p.Slug != slug {
			return fmt.Errorf("directd: Config.Products[%q] holds slug %q", slug, p.Slug)
		}
		if p.Disabled {
			continue
		}
		if !strings.HasPrefix(p.Path, "/api/") {
			return fmt.Errorf("directd: Config.Products[%q].Path %q must start with /api/", slug, p.Path)
		}
		if p.PriceCents <= 0 {
			return fmt.Errorf("directd: Config.Products[%q].PriceCents must be above zero: the Ledger holds it before every request", slug)
		}
	}
	for id, o := range c.ResultCodes {
		if !o.known() {
			return fmt.Errorf("directd: Config.ResultCodes[%d] is %q, which is not an Outcome", id, o)
		}
	}
	return nil
}

// WithDefaults returns the configuration with every zero field set to its
// default. It does not validate.
func (c Config) WithDefaults() Config {
	if c.BaseURL == "" {
		c.BaseURL = DefaultBaseURL
	}
	c.BaseURL = strings.TrimRight(c.BaseURL, "/")
	if c.Timeout == 0 {
		c.Timeout = DefaultTimeout
	}
	if c.Breaker.Threshold == 0 {
		c.Breaker.Threshold = DefaultBreakerThreshold
	}
	if c.Breaker.Cooldown == 0 {
		c.Breaker.Cooldown = DefaultBreakerCooldown
	}
	if c.Location == nil {
		c.Location = brasilia
	}
	if c.MaxBodyBytes == 0 {
		c.MaxBodyBytes = DefaultMaxBodyBytes
	}
	if c.Now == nil {
		c.Now = time.Now
	}
	codes := make(map[int]Outcome, len(c.ResultCodes))
	for k, v := range c.ResultCodes {
		codes[k] = v
	}
	c.ResultCodes = codes
	return c
}

// known reports whether o is one of the declared outcomes.
func (o Outcome) known() bool {
	switch o {
	case OutcomeFound, OutcomeFoundWithCaveats, OutcomeNotFound, OutcomeInvalidDocument, OutcomeUnavailable,
		OutcomeInsufficientBalance, OutcomeUnauthorized, OutcomeRateLimited, OutcomeError:
		return true
	}
	return false
}

// loopback reports whether host is a loopback address or localhost.
func loopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
