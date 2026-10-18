package directd

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// The errors of asynchronous queries.
var (
	// ErrCallbackUnverified is a callback ParseCallback would not read: no
	// CallbackVerifier is configured, or the verifier refused it. The
	// documentation does not say how Direct Data authenticates a callback,
	// so an unauthenticated one is never read.
	ErrCallbackUnverified = errors.New("directd: callback not verified")
	// ErrCallbackMismatch is a callback whose consultaUid is not the
	// ticket's.
	ErrCallbackMismatch = errors.New("directd: callback does not belong to the ticket")
	// ErrCallbackParamUnknown is a per-request callback URL asked for while
	// Config.CallbackURLParam is empty: the documentation names no parameter
	// for it.
	ErrCallbackParamUnknown = errors.New("directd: no documented parameter carries a callback URL; set Config.CallbackURLParam or configure the callback in the portal")
	// ErrTicketClosed is a ticket already completed.
	ErrTicketClosed = errors.New("directd: ticket already completed")
)

// AsyncOptions are the options of an asynchronous query.
type AsyncOptions struct {
	// CallbackURL is where Direct Data should post the result. It is sent in
	// Config.CallbackURLParam, which the documentation does not name; leave
	// it empty to use the callback configured in the portal.
	CallbackURL string
}

// Ticket is an asynchronous query that Direct Data accepted, or the result
// it answered with instead. The application stores the ticket, finds it again
// by ConsultaUID when the callback arrives, and passes both to CompleteAsync.
type Ticket struct {
	// Product is the API's slug.
	Product string
	// Document is the queried document, masked.
	Document string
	// ConsultaUID is Direct Data's identifier of the query; the callback
	// carries the same one.
	ConsultaUID string
	// HoldID is the Ledger hold kept open until CompleteAsync, or "" when
	// none is open.
	HoldID string
	// PriceCents is the product's configured list price.
	PriceCents int64
	// RequestedAt is when the request left.
	RequestedAt time.Time
	// HTTPStatus is the HTTP status of the submission.
	HTTPStatus int
	// Status is the submission's metaDados.resultado.
	Status string
	// Message is the submission's metaDados.mensagem.
	Message string
	// Result is set when the submission was not left pending: Direct Data
	// answered at once, or refused it. Its hold is then already settled or
	// released, and HoldID is "".
	Result *Result
}

// QueryAsync submits a query in asynchronous mode (ASync=habilitar, which the
// documentation offers at no extra cost) and returns its ticket.
//
// The checks, the Dedupe lookup aside, and the Ledger hold are those of
// Query. When Direct Data accepts the query to run in the background, the
// hold stays open and the ticket carries it; CompleteAsync settles or
// releases it when the callback arrives. When Direct Data answers or refuses
// at once, the hold is settled or released here and the ticket carries the
// Result.
func (c *Client) QueryAsync(ctx context.Context, m Meter, req Request, opts AsyncOptions) (*Ticket, error) {
	if opts.CallbackURL != "" {
		if c.cfg.CallbackURLParam == "" {
			return nil, ErrCallbackParamUnknown
		}
		u, err := url.Parse(opts.CallbackURL)
		if err != nil || u.Scheme != "https" || u.Host == "" {
			return nil, fmt.Errorf("%w: the callback URL must be an absolute https URL", ErrInvalidParams)
		}
	}
	res, err := c.query(ctx, m, req, &opts)
	if res == nil {
		return nil, err
	}
	t := &Ticket{
		Product: res.Product, Document: res.Document, ConsultaUID: res.ConsultaUID,
		PriceCents: res.PriceCents, RequestedAt: res.RequestedAt, HTTPStatus: res.HTTPStatus,
		Status: res.Status, Message: res.Message,
	}
	if err == nil && accepted(res) {
		t.HoldID = res.holdID
	} else {
		t.Result = res
	}
	return t, err
}

// accepted reports whether an asynchronous submission was left to run in the
// background: a 2xx answer with a consultaUid that is neither billable nor a
// refusal.
func accepted(r *Result) bool {
	if r.HTTPStatus/100 != 2 || r.ConsultaUID == "" || r.Outcome.Billable() {
		return false
	}
	return r.Outcome == OutcomeError || r.Meta.Assincrono
}

// CallbackRequest is what the application's callback endpoint received.
type CallbackRequest struct {
	// Product is the API's slug, when the application already knows it; it
	// selects the type Data is decoded into. CompleteAsync fills it from the
	// ticket otherwise.
	Product string
	// Header is the request's headers.
	Header http.Header
	// Query is the request's query parameters.
	Query url.Values
	// Body is the request's body.
	Body []byte
}

// CallbackVerifier authenticates a callback before it is read.
type CallbackVerifier interface {
	// VerifyCallback returns nil for a callback that came from Direct Data.
	VerifyCallback(r CallbackRequest) error
}

// SecretQueryVerifier accepts a callback whose query parameter Param holds
// Secret, compared in constant time. It fits a callback URL registered in the
// portal with a secret of the application's own in it, such as
// https://app.example/directd/callback?k=<secret>. The secret travels in the
// URL, so it is only as private as the logs of every hop that sees the URL.
type SecretQueryVerifier struct {
	// Param is the query parameter holding the secret.
	Param string
	// Secret is the expected value.
	Secret Token
}

// VerifyCallback compares the parameter with the secret.
func (v SecretQueryVerifier) VerifyCallback(r CallbackRequest) error {
	return equalSecret(r.Query.Get(v.Param), v.Secret)
}

// SecretHeaderVerifier accepts a callback whose header Header holds Secret,
// compared in constant time, for when the portal lets the callback carry a
// header.
type SecretHeaderVerifier struct {
	// Header is the header holding the secret.
	Header string
	// Secret is the expected value.
	Secret Token
}

// VerifyCallback compares the header with the secret.
func (v SecretHeaderVerifier) VerifyCallback(r CallbackRequest) error {
	return equalSecret(r.Header.Get(v.Header), v.Secret)
}

// equalSecret compares in constant time and refuses an empty secret.
func equalSecret(got string, want Token) error {
	if want.Empty() {
		return fmt.Errorf("%w: the verifier has no secret", ErrCallbackUnverified)
	}
	if subtle.ConstantTimeCompare([]byte(got), []byte(want.reveal())) != 1 {
		return fmt.Errorf("%w: the secret does not match", ErrCallbackUnverified)
	}
	return nil
}

// ParseCallback verifies a callback with Config.CallbackVerifier and reads
// it into a Result marked Async. It refuses every callback when no verifier
// is configured. It settles nothing: CompleteAsync does, with the ticket.
func (c *Client) ParseCallback(r CallbackRequest) (*Result, error) {
	if c.cfg.CallbackVerifier == nil {
		return nil, fmt.Errorf("%w: Config.CallbackVerifier is not set", ErrCallbackUnverified)
	}
	if err := c.cfg.CallbackVerifier.VerifyCallback(r); err != nil {
		if errors.Is(err, ErrCallbackUnverified) {
			return nil, err
		}
		return nil, fmt.Errorf("%w: %w", ErrCallbackUnverified, err)
	}
	res := &Result{Product: r.Product, RequestedAt: c.cfg.Now().In(c.cfg.Location), Async: true}
	if p, ok := c.products[r.Product]; ok {
		res.PriceCents = p.PriceCents
	}
	c.read(res, http.StatusOK, r.Body)
	res.Document = MaskDocument(res.Meta.Chave)
	return res, nil
}

// CompleteAsync settles or releases the ticket's hold from the callback's
// result -- settled at the list price when the outcome is billable, released
// otherwise -- and closes the ticket. The result must carry the ticket's
// consultaUid.
func (c *Client) CompleteAsync(ctx context.Context, l Ledger, t *Ticket, r *Result) error {
	if l == nil {
		return ErrNoLedger
	}
	if t == nil || r == nil {
		return fmt.Errorf("%w: a ticket and a result are both required", ErrCallbackMismatch)
	}
	if t.HoldID == "" {
		return ErrTicketClosed
	}
	if t.ConsultaUID == "" || subtle.ConstantTimeCompare([]byte(t.ConsultaUID), []byte(r.ConsultaUID)) != 1 {
		return ErrCallbackMismatch
	}
	if r.Product == "" {
		r.Product = t.Product
		c.decode(r)
	}
	if r.Document == "" {
		r.Document = t.Document
	}
	r.PriceCents = t.PriceCents
	r.Async = true
	if err := c.settle(ctx, l, t.HoldID, r); err != nil {
		return err
	}
	t.HoldID = ""
	c.logResult(ctx, r)
	return nil
}
