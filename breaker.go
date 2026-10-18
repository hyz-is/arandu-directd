package directd

import (
	"errors"
	"sync"
	"time"
)

// ErrUnavailable is a call the circuit breaker refused, after Threshold
// consecutive transport errors or 5xx answers, until its cooldown ends. No
// request is made and nothing is reserved.
var ErrUnavailable = errors.New("directd: unavailable: the circuit breaker is open")

// BreakerConfig configures the circuit breaker.
type BreakerConfig struct {
	// Threshold is how many consecutive transport errors or 5xx answers open
	// the breaker. Default 5.
	Threshold int
	// Cooldown is how long an open breaker refuses calls before letting one
	// through to probe. Default 30 seconds.
	Cooldown time.Duration
}

// BreakerState is where the breaker is.
type BreakerState string

// The breaker states.
const (
	// BreakerClosed lets every call through.
	BreakerClosed BreakerState = "closed"
	// BreakerOpen refuses every call until the cooldown ends.
	BreakerOpen BreakerState = "open"
	// BreakerHalfOpen lets one probe through; its outcome closes or reopens
	// the breaker.
	BreakerHalfOpen BreakerState = "half_open"
)

// breaker counts consecutive failures and refuses calls while open.
type breaker struct {
	cfg BreakerConfig
	now func() time.Time

	mu       sync.Mutex
	failures int
	state    BreakerState
	until    time.Time
	probing  bool
}

func newBreaker(cfg BreakerConfig, now func() time.Time) *breaker {
	return &breaker{cfg: cfg, now: now, state: BreakerClosed}
}

// allow reports whether a call may go out.
func (b *breaker) allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	switch b.state {
	case BreakerOpen:
		if b.now().Before(b.until) {
			return false
		}
		b.state = BreakerHalfOpen
		b.probing = true
		return true
	case BreakerHalfOpen:
		if b.probing {
			return false
		}
		b.probing = true
		return true
	}
	return true
}

// record counts a call's result: failed is a transport error or a 5xx.
func (b *breaker) record(failed bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.probing = false
	if !failed {
		b.failures = 0
		b.state = BreakerClosed
		return
	}
	b.failures++
	if b.state == BreakerHalfOpen || b.failures >= b.cfg.Threshold {
		b.state = BreakerOpen
		b.until = b.now().Add(b.cfg.Cooldown)
	}
}

// cancel gives back a probe slot that made no request.
func (b *breaker) cancel() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.probing = false
}

// snapshot is the current state.
func (b *breaker) snapshot() BreakerState {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.state == BreakerOpen && !b.now().Before(b.until) {
		return BreakerHalfOpen
	}
	return b.state
}
