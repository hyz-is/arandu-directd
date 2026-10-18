// Command directd-smoke makes a capped run of real Direct Data queries, to
// check the client against the live API. Every query it makes is billed to
// the token's owner.
//
//	DIRECTD_TOKEN_FILE=~/.config/directd/token \
//	go run ./cmd/directd-smoke -spend-limit-cents 500 -out /tmp/directd-smoke \
//	  -call 'CadastroPessoaFisica 00000000000' \
//	  -call 'ReceitaFederalPessoaFisica 00000000000 DataNascimento=01/01/1990'
//
// The token is read from the file DIRECTD_TOKEN_FILE names, never from an
// argument, and never printed. -spend-limit-cents is required and refused
// above 5000 (R$ 50,00). Before each request the call's list price is held
// on a ledger capped by the limit, and the run stops at the first call that
// would pass it; a refused query counts as spent too. -calls-file takes one
// call per line in the -call format ("<product> <document or -> [name=value
// ...]", # for comments). -dry-run prints the plan and its cost, reads no
// token and calls nothing.
//
// Each answer is written redacted -- documents masked, names, addresses,
// phones and e-mails replaced by placeholders -- to -out, which must be
// outside every git repository.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"

	"github.com/hyz-is/arandu-directd/internal/smoke"
)

// callFlags collects repeated -call flags.
type callFlags []string

func (c *callFlags) String() string     { return strings.Join(*c, "; ") }
func (c *callFlags) Set(v string) error { *c = append(*c, v); return nil }

func main() {
	var calls callFlags
	limit := flag.Int64("spend-limit-cents", 0, "spend cap for the run, in BRL cents (required, at most 5000)")
	file := flag.String("calls-file", "", "file with one call per line")
	out := flag.String("out", "", "directory for the redacted answers, outside every git repository")
	dry := flag.Bool("dry-run", false, "print the plan and its cost; read no token and call nothing")
	flag.Var(&calls, "call", "one call: '<product> <document or -> [name=value ...]' (repeatable)")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := run(ctx, calls, *file, *limit, *out, *dry); err != nil {
		fmt.Fprintln(os.Stderr, "directd-smoke:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, flags []string, file string, limit int64, out string, dry bool) error {
	var plan []smoke.Call
	for _, f := range flags {
		c, err := smoke.ParseCall(f)
		if err != nil {
			return fmt.Errorf("-call: %w", err)
		}
		plan = append(plan, c)
	}
	if file != "" {
		fh, err := os.Open(file)
		if err != nil {
			return err
		}
		defer fh.Close()
		more, err := smoke.ParseCalls(fh)
		if err != nil {
			return fmt.Errorf("-calls-file: %w", err)
		}
		plan = append(plan, more...)
	}
	return smoke.Run(ctx, smoke.Options{
		TokenFile:       os.Getenv("DIRECTD_TOKEN_FILE"),
		SpendLimitCents: limit,
		Calls:           plan,
		OutDir:          out,
		DryRun:          dry,
		Stdout:          os.Stdout,
		Stderr:          os.Stderr,
	})
}
