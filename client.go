package directd

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// The errors a call is refused with before anything leaves the process. Each
// is wrapped with the detail of the case, so callers test for them with
// errors.Is.
var (
	// ErrUnknownProduct is a slug that is neither in the catalogue nor in
	// Config.Products.
	ErrUnknownProduct = errors.New("directd: unknown product")
	// ErrProductDisabled is a product disabled in Config.Products, or one the
	// documentation lists as inactive.
	ErrProductDisabled = errors.New("directd: product disabled")
	// ErrInvalidParams is a parameter the product does not document, a
	// reserved one (TOKEN, ASync), a document passed in Params, or a query
	// with nothing to ask.
	ErrInvalidParams = errors.New("directd: invalid parameters")
)

// asyncParam and asyncValue switch a query to asynchronous mode, as every
// product page documents ("ASync=habilitar").
const (
	asyncParam = "ASync"
	asyncValue = "habilitar"
	tokenParam = "TOKEN"
)

// Request is one query.
type Request struct {
	// Product is the API's slug, such as CadastroPessoaFisicaPlus.
	Product string
	// Document is the CPF or CNPJ queried, bare or punctuated. The client
	// checks its check digits and sends it, bare, in the product's CPF, CNPJ
	// or document parameter. Leave it empty for a product queried by other
	// inputs (a plate, a name).
	Document string
	// Params are the product's other parameters, by documented name (case is
	// ignored), such as DataNascimento or Placa.
	Params map[string]string
}

// Client queries Direct Data. It is safe for concurrent use.
type Client struct {
	cfg      Config
	products map[string]Product
	http     *http.Client
	breaker  *breaker
	log      *slog.Logger
}

// New returns the client, or the reason it cannot be built.
func New(cfg Config) (*Client, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	cfg = cfg.WithDefaults()

	products := DefaultProducts()
	for slug, p := range cfg.Products {
		if p.Slug == "" {
			p.Slug = slug
		}
		if base, ok := products[slug]; ok && p.Path == "" && p.Disabled {
			base.Disabled = true
			p = base
		}
		for i := range p.Params {
			if p.Params[i].Role == "" {
				p.Params[i].Role = roleOf(p.Params[i].Name)
			}
		}
		products[slug] = p.clone()
	}

	hc := &http.Client{}
	if cfg.HTTPClient != nil {
		copied := *cfg.HTTPClient
		hc = &copied
	}
	hc.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

	log := cfg.Logger
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Client{
		cfg:      cfg,
		products: products,
		http:     hc,
		breaker:  newBreaker(cfg.Breaker, cfg.Now),
		log:      log,
	}, nil
}

// Product returns the product with the slug, as the client uses it.
func (c *Client) Product(slug string) (Product, bool) {
	p, ok := c.products[slug]
	return p.clone(), ok
}

// Products returns every product the client knows, by slug. The map is a copy.
func (c *Client) Products() map[string]Product {
	out := make(map[string]Product, len(c.products))
	for k, p := range c.products {
		out[k] = p.clone()
	}
	return out
}

// Check reports why a request would be refused before any request,
// reservation or lookup -- an unknown or disabled product, an invalid
// document, an undocumented parameter -- or nil. It costs nothing.
func (c *Client) Check(req Request) error {
	_, err := c.prepare(req)
	return err
}

// BreakerState reports where the circuit breaker is.
func (c *Client) BreakerState() BreakerState { return c.breaker.snapshot() }

// prepared is a request checked and ready to send.
type prepared struct {
	product  Product
	values   url.Values
	document string
	masked   string
	key      DedupeKey
}

// prepare checks a request against its product: the product exists and is
// enabled, the document is valid and accepted, every parameter is
// documented. It makes no request and touches no meter.
func (c *Client) prepare(req Request) (prepared, error) {
	p, ok := c.products[req.Product]
	if !ok {
		return prepared{}, fmt.Errorf("%w: %q", ErrUnknownProduct, req.Product)
	}
	if p.Disabled || p.Status == StatusInactive {
		return prepared{}, fmt.Errorf("%w: %s", ErrProductDisabled, p.Slug)
	}
	values := url.Values{}
	out := prepared{product: p}

	if strings.TrimSpace(req.Document) != "" {
		doc, kind, err := parseDocument(req.Document)
		if err != nil {
			return prepared{}, fmt.Errorf("%s: %w", p.Slug, err)
		}
		var param Param
		switch {
		case kind == KindCPF:
			param, ok = p.byRole(RoleCPF)
		case kind == KindCNPJ:
			param, ok = p.byRole(RoleCNPJ)
		}
		if !ok {
			param, ok = p.byRole(RoleDocument)
		}
		if !ok {
			return prepared{}, fmt.Errorf("%w: %s does not take a %s", ErrInvalidDocument, p.Slug, strings.ToUpper(string(kind)))
		}
		values.Set(param.Name, doc)
		out.document = doc
		out.masked = MaskDocument(doc)
	}

	names := make([]string, 0, len(req.Params))
	for name := range req.Params {
		names = append(names, name)
	}
	sort.Strings(names)
	var digest strings.Builder
	for _, name := range names {
		value := req.Params[name]
		if strings.EqualFold(name, tokenParam) || strings.EqualFold(name, asyncParam) {
			return prepared{}, fmt.Errorf("%w: %s is set by the client", ErrInvalidParams, name)
		}
		param, ok := p.param(name)
		if !ok {
			return prepared{}, fmt.Errorf("%w: %s does not document %q", ErrInvalidParams, p.Slug, name)
		}
		switch param.Role {
		case RoleCPF, RoleCNPJ, RoleDocument:
			return prepared{}, fmt.Errorf("%w: pass the document in Request.Document, where it is checked, not in Params[%q]", ErrInvalidParams, name)
		}
		if value == "" {
			continue
		}
		values.Set(param.Name, value)
		fmt.Fprintf(&digest, "%s=%s\n", strings.ToLower(param.Name), value)
	}
	if len(values) == 0 {
		return prepared{}, fmt.Errorf("%w: %s needs a document or a parameter", ErrInvalidParams, p.Slug)
	}
	sum := sha256.Sum256([]byte(digest.String()))
	out.values = values
	out.key = DedupeKey{Product: p.Slug, Document: out.document, ParamsDigest: hex.EncodeToString(sum[:])}
	return out, nil
}

// Query makes one synchronous query and returns its result as evidence.
//
// In order: the product and the document are checked (an invalid one is
// refused with ErrInvalidDocument before anything else); m.Dedupe may answer
// with a result remembered for the day, marked Cached, and no request is
// made; the circuit breaker may refuse with ErrUnavailable; m.Ledger holds
// the price, and a refusal (ErrBudgetExceeded) means no request; the request
// is made; the hold is settled when the outcome is billable and released
// otherwise; a billable result is remembered in m.Dedupe.
//
// The error is non-nil when no answer could be read (transport, timeout) or
// the Ledger failed. A query Direct Data answered comes back with a nil error
// whatever its Outcome: not_found is an answer.
func (c *Client) Query(ctx context.Context, m Meter, req Request) (*Result, error) {
	return c.query(ctx, m, req, nil)
}

func (c *Client) query(ctx context.Context, m Meter, req Request, opts *AsyncOptions) (*Result, error) {
	async := opts != nil
	if m.Ledger == nil {
		return nil, ErrNoLedger
	}
	pr, err := c.prepare(req)
	if err != nil {
		return nil, err
	}
	now := c.cfg.Now().In(c.cfg.Location)
	pr.key.Day = now.Format(time.DateOnly)

	if m.Dedupe != nil && !async {
		cached, ok, err := m.Dedupe.Seen(ctx, pr.key)
		if err != nil {
			return nil, fmt.Errorf("directd: dedupe lookup: %w", err)
		}
		if ok && cached != nil {
			out := *cached
			out.Cached = true
			out.ChargedCents = 0
			c.logResult(ctx, &out)
			return &out, nil
		}
	}

	if !c.breaker.allow() {
		return nil, fmt.Errorf("%w: %s", ErrUnavailable, pr.product.Slug)
	}
	hold, err := m.Ledger.Reserve(ctx, Charge{Product: pr.product.Slug, PriceCents: pr.product.PriceCents, Document: pr.masked})
	if err != nil {
		c.breaker.cancel()
		return nil, fmt.Errorf("directd: %s: reserving %d cents: %w", pr.product.Slug, pr.product.PriceCents, err)
	}
	if async {
		pr.values.Set(asyncParam, asyncValue)
		if opts.CallbackURL != "" {
			pr.values.Set(c.cfg.CallbackURLParam, opts.CallbackURL)
		}
	}

	res, sendErr := c.send(ctx, pr, now)
	res.holdID = hold
	if sendErr != nil {
		c.breaker.record(true)
		relErr := m.Ledger.Release(ctx, hold)
		c.logResult(ctx, res)
		return res, errors.Join(sendErr, wrapLedger("release", relErr))
	}
	c.breaker.record(res.HTTPStatus >= 500)

	if async && accepted(res) {
		// The query was accepted to run in the background: the hold stays
		// open until CompleteAsync.
		c.logResult(ctx, res)
		return res, nil
	}
	if err := c.settle(ctx, m.Ledger, hold, res); err != nil {
		c.logResult(ctx, res)
		return res, err
	}
	if m.Dedupe != nil && res.Outcome.Billable() && !async {
		if err := m.Dedupe.Remember(ctx, pr.key, res); err != nil {
			c.logResult(ctx, res)
			return res, fmt.Errorf("directd: dedupe remember: %w", err)
		}
	}
	c.logResult(ctx, res)
	return res, nil
}

// settle settles a billable result's hold at the list price and releases any
// other.
func (c *Client) settle(ctx context.Context, l Ledger, hold string, res *Result) error {
	if res.Outcome.Billable() {
		if err := l.Settle(ctx, hold, res.PriceCents); err != nil {
			return wrapLedger("settle", err)
		}
		res.ChargedCents = res.PriceCents
		return nil
	}
	return wrapLedger("release", l.Release(ctx, hold))
}

// wrapLedger names the ledger step that failed.
func wrapLedger(step string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("directd: ledger %s: %w", step, err)
}

// send makes the request and reads the answer. The returned Result is never
// nil; the error is a transport failure, with the URL stripped from it.
func (c *Client) send(ctx context.Context, pr prepared, now time.Time) (*Result, error) {
	res := &Result{
		Product:     pr.product.Slug,
		Document:    pr.masked,
		RequestedAt: now,
		PriceCents:  pr.product.PriceCents,
		Outcome:     OutcomeUnavailable,
	}

	values := url.Values{}
	for k, v := range pr.values {
		values[k] = v
	}
	values.Set(tokenParam, c.cfg.Token.reveal())
	target := c.cfg.BaseURL + pr.product.Path + "?" + values.Encode()

	ctx, cancel := context.WithTimeout(ctx, c.cfg.Timeout)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return res, c.scrub(pr.product, err)
	}
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("User-Agent", "arandu-directd")

	start := time.Now()
	resp, err := c.http.Do(httpReq)
	if err != nil {
		res.ElapsedMs = time.Since(start).Milliseconds()
		return res, c.scrub(pr.product, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, c.cfg.MaxBodyBytes+1))
	res.ElapsedMs = time.Since(start).Milliseconds()
	if err != nil {
		return res, c.scrub(pr.product, err)
	}
	if int64(len(body)) > c.cfg.MaxBodyBytes {
		return res, fmt.Errorf("directd: %s: the answer is larger than %d bytes", pr.product.Slug, c.cfg.MaxBodyBytes)
	}
	c.read(res, resp.StatusCode, body)
	return res, nil
}

// read fills a result from an answer's status and body.
func (c *Client) read(res *Result, status int, body []byte) {
	res.HTTPStatus = status
	var env envelope
	var meta *MetaData
	if json.Valid(body) {
		res.Body = append(json.RawMessage(nil), bytes.TrimSpace(body)...)
		if err := json.Unmarshal(body, &env); err == nil {
			if env.MetaDados != nil {
				m := env.MetaDados.meta()
				meta = &m
				res.Meta = m
				res.ConsultaUID = m.ConsultaUID
				res.Status = m.Resultado
				res.StatusID = m.ResultadoID
				res.Message = m.Mensagem
				res.ReceiptURL = m.URLComprovante
			}
			if code, ok := env.HTTPStatusCode.Int64(); ok && code != 0 {
				res.HTTPStatus = int(code)
			}
			if res.Message == "" {
				res.Message = string(env.Message)
			}
			res.Errors = errorTexts(env.Errors)
			if len(env.Retorno) > 0 && string(env.Retorno) != "null" {
				res.Retorno = env.Retorno
			}
		}
	}
	res.Outcome = Classify(res.HTTPStatus, meta, c.cfg.ResultCodes)
	c.decode(res)
}

// decode fills Data from Retorno with the product's generated type.
func (c *Client) decode(res *Result) {
	if len(res.Retorno) == 0 {
		return
	}
	dec, ok := decoders[res.Product]
	if !ok {
		return
	}
	v, err := dec(res.Retorno)
	if err != nil {
		res.DataError = err.Error()
		return
	}
	res.Data = v
}

// scrub turns a transport error into one that carries neither the URL nor
// the token: *url.Error prints the whole URL, and the URL holds the token.
func (c *Client) scrub(p Product, err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		err = ue.Err
	}
	return &transportError{
		msg: fmt.Sprintf("directd: %s %s: %s", http.MethodGet, p.Path,
			strings.NewReplacer(c.cfg.Token.reveal(), redacted, url.QueryEscape(c.cfg.Token.reveal()), redacted).Replace(err.Error())),
		err: err,
	}
}

// transportError is a failure to reach Direct Data, with a message that holds
// no secret. It unwraps to the cause, so errors.Is(err,
// context.DeadlineExceeded) still answers.
type transportError struct {
	msg string
	err error
}

func (e *transportError) Error() string { return e.msg }
func (e *transportError) Unwrap() error { return e.err }

// logResult writes one record about a call. It never writes the token, the
// URL, the document unmasked or any response value.
func (c *Client) logResult(ctx context.Context, r *Result) {
	if r == nil {
		return
	}
	c.log.LogAttrs(ctx, slog.LevelInfo, "directd query",
		slog.String("product", r.Product),
		slog.String("document", r.Document),
		slog.String("outcome", string(r.Outcome)),
		slog.Int("http_status", r.HTTPStatus),
		slog.Int64("elapsed_ms", r.ElapsedMs),
		slog.String("consulta_uid", r.ConsultaUID),
		slog.Int64("price_cents", r.PriceCents),
		slog.Int64("charged_cents", r.ChargedCents),
		slog.Bool("cached", r.Cached),
	)
}
