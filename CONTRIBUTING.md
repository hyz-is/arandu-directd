# Contributing

Thank you for considering a contribution.

## Before you open a pull request

Run the four gates. They are what CI runs, and a pull request that fails one of
them fails for everybody who pulls after it.

```bash
export GOWORK=off
gofmt -l $(find . -name '*.go' -not -path '*/testdata/*' -not -name '*.kyse.go')
go build ./...
go vet ./...
go test -race ./...
```

The two filters on `gofmt` are not preference. `gofmt` is the only tool in the
chain that ignores build tags, so it reads files the compiler deliberately
excludes; `testdata/` holds fixtures that are invalid on purpose.

A change to `catalog/catalog.json`, to the generator in `internal/gen` or to the
README's text around the API table is followed by `go generate ./...`, and the
regenerated files go in the same commit. `TestTheGeneratedFilesAreCurrent`
fails otherwise.

## What a change has to keep true

1. **No test, example or session calls Direct Data.** Every query is billed.
   Tests use the `httptest` double; `cmd/directd-smoke` is run by the owner,
   under a spend cap.
2. **Evidence, never decisions.** No field, type or function that says
   approved, rejected, regular or trusted.
3. **Every query is metered.** The Ledger holds the price before the request;
   settle only a billable outcome; an invalid document, an exhausted budget,
   an open breaker and a dedupe hit make no request.
4. **Nothing undocumented is presented as documented.** The catalogue says
   what the public documentation says; what it does not say is configuration
   or an `incomplete` entry with its reason.
5. **The token never prints**, and no personal value reaches a log or an
   error.
6. **`arandu.mod.toml` matches the code.** A new outbound call, file access or
   process is declared there in the same commit.

## Style

Everything in the source is in English: identifiers, doc comments, internal
comments, error messages, log messages, and the names and messages of tests.
`pkg.go.dev` publishes the doc comments, and its readers are users of this
package.

Every exported symbol carries a doc comment saying what it does. The comment
documents the symbol and nothing else: no dates, no issue numbers, no
references to other repositories.

## Commits

One commit per logical change. Formatting is not mixed with behaviour, because
a diff that does both is a diff nobody reviews.

## Reporting a vulnerability

Do not open an issue. See [SECURITY.md](SECURITY.md).

## Licence

By contributing you agree that your contribution is licensed under the MIT
licence, the same as the rest of github.com/hyz-is/arandu-directd.
