package directd

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
)

// ErrBudgetExceeded is what a Ledger returns from Reserve when the price does
// not fit the budget. The client then makes no request.
var ErrBudgetExceeded = errors.New("directd: budget exceeded")

// ErrNoLedger is a query made without a Ledger. Every query costs money, so
// the client refuses to make one nobody accounts for.
var ErrNoLedger = errors.New("directd: no ledger: every query is billed, and Meter.Ledger accounts for it")

// Charge is what a query is about to cost, as the client asks a Ledger to
// hold it.
type Charge struct {
	// Product is the API's slug.
	Product string
	// PriceCents is the product's configured list price.
	PriceCents int64
	// Document is the queried document, masked.
	Document string
}

// Ledger holds, settles and releases the cost of queries. The application
// implements it -- normally over its wallet (arandu-wallet), bound to the
// tenant of the Grant the query is made under -- and passes it in the Meter of
// each call. This package keeps no balance.
//
// The client calls Reserve before the request and makes none when Reserve
// fails; then exactly one of Settle or Release for the hold: Settle with the
// price when the outcome is billable under terms clause 2.5, Release
// otherwise. An asynchronous query keeps its hold open until CompleteAsync.
type Ledger interface {
	// Reserve holds the charge and returns the hold's identifier, or an error
	// wrapping ErrBudgetExceeded when it does not fit.
	Reserve(ctx context.Context, c Charge) (holdID string, err error)
	// Settle turns the hold into a spend of cents.
	Settle(ctx context.Context, holdID string, cents int64) error
	// Release gives the hold back.
	Release(ctx context.Context, holdID string) error
}

// DedupeKey identifies a query for Dedupe. Direct Data bills the same
// document on the same API on the same day again (terms clause 2.5), so a
// result remembered under this key saves a charge.
type DedupeKey struct {
	// Product is the API's slug.
	Product string
	// Document is the document, bare (digits, or the CNPJ's characters), or
	// "" when the query had none.
	Document string
	// ParamsDigest is the SHA-256, in hex, of every other parameter, so a
	// query with other inputs is a different key.
	ParamsDigest string
	// Day is the query's date in the configured location, YYYY-MM-DD.
	Day string
}

// Dedupe remembers billable results for the day. The application implements
// it over its own storage, bound to the tenant like the Ledger, and keeps
// what it stores encrypted: a result is personal data. Optional: a Meter
// without one queries every time.
type Dedupe interface {
	// Seen returns the result remembered under the key, and false on a miss.
	Seen(ctx context.Context, key DedupeKey) (*Result, bool, error)
	// Remember stores a billable result under the key.
	Remember(ctx context.Context, key DedupeKey, r *Result) error
}

// Meter is the accounting a call is made under: the Ledger that pays for it
// and, optionally, the Dedupe that may answer it for free. The application
// builds one per tenant from the authorized Grant; the client never reads a
// tenant itself.
type Meter struct {
	// Ledger accounts for the call. Required.
	Ledger Ledger
	// Dedupe may answer the call from a result remembered for the day.
	Dedupe Dedupe
}

// MemoryLedger is a Ledger in memory with a cap, for tests and commands. It is
// not storage: what it holds is gone with the process.
type MemoryLedger struct {
	mu       sync.Mutex
	capCents int64
	spent    int64
	held     map[string]int64
}

// NewMemoryLedger returns a MemoryLedger whose holds and spends together may
// not pass capCents.
func NewMemoryLedger(capCents int64) *MemoryLedger {
	return &MemoryLedger{capCents: capCents, held: map[string]int64{}}
}

// Reserve holds the price, or returns ErrBudgetExceeded when it does not fit
// in what the cap leaves.
func (l *MemoryLedger) Reserve(_ context.Context, c Charge) (string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if c.PriceCents < 0 {
		return "", fmt.Errorf("directd: negative price %d", c.PriceCents)
	}
	if left := l.capCents - l.spent - l.heldLocked(); c.PriceCents > left {
		return "", fmt.Errorf("%w: %s costs %d cents and %d remain", ErrBudgetExceeded, c.Product, c.PriceCents, left)
	}
	id := newID()
	l.held[id] = c.PriceCents
	return id, nil
}

// Settle spends cents from the hold and frees the rest.
func (l *MemoryLedger) Settle(_ context.Context, holdID string, cents int64) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, ok := l.held[holdID]; !ok {
		return fmt.Errorf("directd: no hold %q", holdID)
	}
	delete(l.held, holdID)
	l.spent += cents
	return nil
}

// Release frees the hold.
func (l *MemoryLedger) Release(_ context.Context, holdID string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, ok := l.held[holdID]; !ok {
		return fmt.Errorf("directd: no hold %q", holdID)
	}
	delete(l.held, holdID)
	return nil
}

// SpentCents is what was settled.
func (l *MemoryLedger) SpentCents() int64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.spent
}

// HeldCents is what is held and not yet settled or released.
func (l *MemoryLedger) HeldCents() int64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.heldLocked()
}

// RemainingCents is what the cap still allows.
func (l *MemoryLedger) RemainingCents() int64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.capCents - l.spent - l.heldLocked()
}

func (l *MemoryLedger) heldLocked() int64 {
	var n int64
	for _, c := range l.held {
		n += c
	}
	return n
}

// MemoryDedupe is a Dedupe in memory, for tests and commands. It is not
// storage.
type MemoryDedupe struct {
	mu   sync.Mutex
	seen map[DedupeKey]*Result
}

// NewMemoryDedupe returns an empty MemoryDedupe.
func NewMemoryDedupe() *MemoryDedupe { return &MemoryDedupe{seen: map[DedupeKey]*Result{}} }

// Seen returns the result remembered under the key.
func (d *MemoryDedupe) Seen(_ context.Context, key DedupeKey) (*Result, bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	r, ok := d.seen[key]
	if !ok {
		return nil, false, nil
	}
	c := *r
	return &c, true, nil
}

// Remember stores the result under the key.
func (d *MemoryDedupe) Remember(_ context.Context, key DedupeKey, r *Result) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	c := *r
	d.seen[key] = &c
	return nil
}

// newID draws a 128-bit hold identifier.
func newID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
