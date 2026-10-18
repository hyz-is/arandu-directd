// Package smoke is the body of cmd/directd-smoke: a capped run of real
// Direct Data queries, to check the client against the live API.
//
// It is internal so that no application links it. It reads the token from a
// file, holds every call's list price on a MemoryLedger capped by the spend
// limit before the request, counts a released hold as spent too (a refused
// query is not billed under terms clause 2.5, but the run does not bet on
// it), prints one line per call without any personal value, and writes each
// answer redacted to a directory outside every git repository.
package smoke

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	directd "github.com/hyz-is/arandu-directd"
)

// MaxSpendLimitCents is the highest spend limit a run accepts: R$ 50,00.
const MaxSpendLimitCents = 5000

// Call is one planned query.
type Call struct {
	// Product is the API's slug.
	Product string
	// Document is the CPF or CNPJ, or "" for a product queried by other
	// inputs.
	Document string
	// Params are the other parameters, by documented name.
	Params map[string]string
}

// Options configure a run.
type Options struct {
	// TokenFile names the file holding the API token.
	TokenFile string
	// SpendLimitCents caps the run. Required, at most MaxSpendLimitCents.
	SpendLimitCents int64
	// Calls are the planned queries, in order.
	Calls []Call
	// OutDir receives one redacted answer per call. It must be outside every
	// git repository. Not needed for a dry run.
	OutDir string
	// DryRun prints the plan and its cost and calls nothing.
	DryRun bool
	// Stdout and Stderr receive the report and the errors.
	Stdout, Stderr io.Writer
	// BaseURL and HTTPClient replace the API host and the transport. Tests
	// set them to a double; the command never does.
	BaseURL    string
	HTTPClient *http.Client
}

// ParseCalls reads planned queries, one per line: the slug, the document (or
// "-" for none) and any number of name=value parameters, separated by spaces.
// Blank lines and lines starting with # are skipped. A value cannot hold a
// space.
func ParseCalls(r io.Reader) ([]Call, error) {
	var out []Call
	sc := bufio.NewScanner(r)
	n := 0
	for sc.Scan() {
		n++
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		c, err := ParseCall(line)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", n, err)
		}
		out = append(out, c)
	}
	return out, sc.Err()
}

// ParseCall reads one planned query in the ParseCalls format.
func ParseCall(line string) (Call, error) {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return Call{}, errors.New("want: <product> <document or -> [name=value ...]")
	}
	c := Call{Product: fields[0], Params: map[string]string{}}
	if fields[1] != "-" {
		c.Document = fields[1]
	}
	for _, f := range fields[2:] {
		name, value, ok := strings.Cut(f, "=")
		if !ok || name == "" {
			return Call{}, fmt.Errorf("parameter %q is not name=value", f)
		}
		c.Params[name] = value
	}
	return c, nil
}

// Run executes the plan.
func Run(ctx context.Context, o Options) error {
	if o.Stdout == nil {
		o.Stdout = io.Discard
	}
	if o.Stderr == nil {
		o.Stderr = io.Discard
	}
	if o.SpendLimitCents <= 0 {
		return errors.New("-spend-limit-cents is required and must be above zero")
	}
	if o.SpendLimitCents > MaxSpendLimitCents {
		return fmt.Errorf("-spend-limit-cents %d is above the authorized %d", o.SpendLimitCents, MaxSpendLimitCents)
	}
	if len(o.Calls) == 0 {
		return errors.New("no call planned")
	}

	// The plan is checked with a client that holds a placeholder token, so
	// that a dry run never reads the token file and an invalid plan is
	// refused before it is read.
	planner, err := directd.New(directd.Config{Token: "plan-only", BaseURL: o.BaseURL})
	if err != nil {
		return err
	}
	var total int64
	fmt.Fprintf(o.Stdout, "plan: %d calls, spend limit %s\n", len(o.Calls), brl(o.SpendLimitCents))
	for i, c := range o.Calls {
		p, ok := planner.Product(c.Product)
		if !ok {
			return fmt.Errorf("call %d: unknown product %q", i+1, c.Product)
		}
		if p.Disabled || p.Status == directd.StatusInactive {
			return fmt.Errorf("call %d: %s is not available", i+1, c.Product)
		}
		if err := checkCall(c); err != nil {
			return fmt.Errorf("call %d: %w", i+1, err)
		}
		if err := planner.Check(directd.Request{Product: c.Product, Document: c.Document, Params: c.Params}); err != nil {
			return fmt.Errorf("call %d: %w", i+1, err)
		}
		total += p.PriceCents
		fmt.Fprintf(o.Stdout, "  %2d. %-40s %-20s %s\n", i+1, c.Product, masked(c.Document), brl(p.PriceCents))
	}
	fmt.Fprintf(o.Stdout, "plan total %s; the cap stops the run before any call that would pass %s\n", brl(total), brl(o.SpendLimitCents))
	if o.DryRun {
		fmt.Fprintln(o.Stdout, "dry run: nothing called")
		return nil
	}

	if err := checkOutDir(o.OutDir); err != nil {
		return err
	}
	token, err := readToken(o.TokenFile, o.Stderr)
	if err != nil {
		return err
	}
	client, err := directd.New(directd.Config{Token: token, BaseURL: o.BaseURL, HTTPClient: o.HTTPClient})
	if err != nil {
		return err
	}

	ledger := &spendAll{MemoryLedger: directd.NewMemoryLedger(o.SpendLimitCents)}
	meter := directd.Meter{Ledger: ledger, Dedupe: directd.NewMemoryDedupe()}
	for i, c := range o.Calls {
		res, err := client.Query(ctx, meter, directd.Request{Product: c.Product, Document: c.Document, Params: c.Params})
		if errors.Is(err, directd.ErrBudgetExceeded) {
			fmt.Fprintf(o.Stdout, "%2d. %s refused: the next call would pass the cap (spent %s of %s); stopping\n",
				i+1, c.Product, brl(ledger.SpentCents()), brl(o.SpendLimitCents))
			return nil
		}
		if res == nil {
			fmt.Fprintf(o.Stdout, "%2d. %s %s error: %v\n", i+1, c.Product, masked(c.Document), err)
			continue
		}
		fmt.Fprintf(o.Stdout, "%2d. %s %s http=%d outcome=%s resultado=%q mensagem=%q consultaUid=%s elapsed=%dms price=%s cached=%v total=%s\n",
			i+1, res.Product, res.Document, res.HTTPStatus, res.Outcome, clean(res.Status), clean(res.Message),
			res.ConsultaUID, res.ElapsedMs, brl(res.PriceCents), res.Cached, brl(ledger.SpentCents()))
		if err != nil {
			fmt.Fprintf(o.Stdout, "    error: %v\n", err)
		}
		if err := writeAnswer(o.OutDir, i+1, res); err != nil {
			return err
		}
	}
	fmt.Fprintf(o.Stdout, "done: spent at most %s of %s\n", brl(ledger.SpentCents()), brl(o.SpendLimitCents))
	return nil
}

// spendAll is a MemoryLedger on which a released hold is spent too: the run's
// cap counts every request that left, billable or not.
type spendAll struct {
	*directd.MemoryLedger
	prices map[string]int64
}

// Reserve holds the price and remembers it, so Release can spend it.
func (s *spendAll) Reserve(ctx context.Context, c directd.Charge) (string, error) {
	id, err := s.MemoryLedger.Reserve(ctx, c)
	if err == nil {
		if s.prices == nil {
			s.prices = map[string]int64{}
		}
		s.prices[id] = c.PriceCents
	}
	return id, err
}

// Release spends the hold's whole price.
func (s *spendAll) Release(ctx context.Context, holdID string) error {
	return s.MemoryLedger.Settle(ctx, holdID, s.prices[holdID])
}

// checkCall refuses a value with a control character.
func checkCall(c Call) error {
	for name, v := range c.Params {
		if strings.ContainsAny(v, "\r\n\t") {
			return fmt.Errorf("parameter %s holds a control character", name)
		}
	}
	return nil
}

// readToken reads and trims the token file. Its content is never printed.
func readToken(path string, stderr io.Writer) (directd.Token, error) {
	if path == "" {
		return "", errors.New("DIRECTD_TOKEN_FILE is not set: it names the file holding the token")
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("the token file cannot be read: %w", errors.Unwrap(err))
	}
	if info.Mode().Perm()&0o077 != 0 {
		fmt.Fprintf(stderr, "warning: the token file is readable by others (mode %v); chmod 600 it\n", info.Mode().Perm())
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", errors.New("the token file cannot be read")
	}
	token := strings.TrimSpace(string(raw))
	if token == "" || strings.ContainsAny(token, " \t\r\n") {
		return "", errors.New("the token file must hold the token alone, on one line")
	}
	return directd.Token(token), nil
}

// checkOutDir refuses an output directory inside a git repository and
// creates it, private to the user.
func checkOutDir(dir string) error {
	if dir == "" {
		return errors.New("-out is required: the directory the redacted answers go to")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	for d := abs; ; d = filepath.Dir(d) {
		if _, err := os.Lstat(filepath.Join(d, ".git")); err == nil {
			return fmt.Errorf("-out %s is inside the git repository at %s; answers stay outside every repository", abs, d)
		}
		if filepath.Dir(d) == d {
			break
		}
	}
	return os.MkdirAll(abs, 0o700)
}

// writeAnswer writes one answer, redacted.
func writeAnswer(dir string, n int, r *directd.Result) error {
	var body []byte
	if len(r.Body) > 0 {
		red, ok := directd.Redact(r.Body)
		if !ok {
			red = []byte(`"<unreadable body>"`)
		}
		body = red
	} else {
		body = []byte(`null`)
	}
	doc := fmt.Sprintf("{\n  \"product\": %q,\n  \"document\": %q,\n  \"http_status\": %d,\n  \"outcome\": %q,\n  \"elapsed_ms\": %d,\n  \"price_cents\": %d,\n  \"requested_at\": %q,\n  \"body\": %s\n}\n",
		r.Product, r.Document, r.HTTPStatus, r.Outcome, r.ElapsedMs, r.PriceCents, r.RequestedAt.Format(time.RFC3339), body)
	name := filepath.Join(dir, fmt.Sprintf("%02d-%s.json", n, r.Product))
	return os.WriteFile(name, []byte(doc), 0o600)
}

// masked writes a document masked, or "-" for none.
func masked(doc string) string {
	if doc == "" {
		return "-"
	}
	if m := directd.MaskDocument(doc); m != "" {
		return m
	}
	return "<invalid>"
}

// clean redacts a status or message before it is printed.
func clean(s string) string {
	quoted, _ := json.Marshal(s)
	red, ok := directd.Redact([]byte(`{"mensagem":` + string(quoted) + `}`))
	if !ok {
		return "<unreadable>"
	}
	out := strings.TrimPrefix(string(red), `{"mensagem":`)
	out = strings.TrimSuffix(out, `}`)
	return strings.Trim(out, `"`)
}

// brl writes cents as Brazilian reais.
func brl(cents int64) string { return fmt.Sprintf("R$ %d,%02d", cents/100, cents%100) }
