# Working on Arandu Direct Data

This is an Arandu package: a Go client for the Direct Data API covering every
API of its public catalogue. It returns evidence about each query -- what was
asked, when, at what price, what the source said, and the data, typed -- and
decides nothing. It is a Go module somebody `go get`s and wires by hand in
their own `bootstrap/app.go`; there is no service provider, no container and
no discovery.

Read `.agents/skills/` before writing code. Each skill is a procedure, and the
one you need is named by the situation you are in.

## The rules

A change that breaks one is not merged.

1. **No real call, ever, from a test or a session.** Every Direct Data query
   is billed to the token's owner. Tests talk to an `httptest` double and to
   nothing else. `cmd/directd-smoke` is the one program that queries the real
   API; it is run by the owner, or with the owner's written authorization and
   spend cap, never by an assistant on its own initiative.
2. **The documentation is read, not the API.** `cmd/directd-catalog` reads the
   public documentation (`directd.com.br/central-de-ajuda/apis/...`) and
   refuses `apiv3.directd.com.br` and `app.directd.com.br` by name. A 401, 403
   or 429 stops it; nothing retries around a refusal.
3. **Nothing is invented.** A parameter, a field, a price or a status code
   that the documentation does not state is not written down as if it did. A
   page that cannot be read leaves its catalogue entry `incomplete` with the
   reason. The undocumented parts (resultadoId values, callback
   authentication, the callback URL parameter) are configuration the
   application fills, refused until it does.
4. **Evidence, never decisions.** No result field, function or type says
   approved, rejected, regular, trusted or risky. `tests/Unit/decision_test.go`
   walks the hand-written source and the `Result` type. The generated types
   carry Direct Data's own keys, which are data.
5. **Every query is metered.** A call without a `Ledger` is refused. The
   price is held before the request, settled only for a billable outcome
   (terms clause 2.5) and released otherwise. An invalid document, an
   exhausted budget, an open breaker and a dedupe hit make no request.
6. **The tenant comes from the application.** The package never reads a
   request, a path, a header or a body to find one; the application builds the
   `Meter` from an authorized Grant.
7. **The token never prints.** `Token` redacts itself everywhere; the URL is
   stripped from transport errors; redirects are refused; logs carry the
   masked document only.
8. **Generated code is generated.** `zz_catalog_*.go` and the README's API
   table come from `catalog/catalog.json` through `go generate ./...`;
   `tests/Unit/catalog_test.go` fails when they drift.
9. **Fixtures are synthetic.** Documents with valid check digits that belong
   to nobody known, names that say SINTETICA, `example.invalid` addresses. No
   real person, company, plate or answer in a test, an example or a log line.
10. **Go only.** No Python, no Node, not in a gate, a generator or a crawler.

## The gates

Nothing is finished until all four exit zero.

```sh
export GOWORK=off
test -z "$(gofmt -l $(find . -name '*.go' -not -path '*/testdata/*'))"
go build ./...
go vet ./...
go test -race -count=1 ./...
```

`GOWORK=off` because this checkout may sit beside a Go workspace that does not
list it. On a shared workstation, `GOFLAGS=-p=2` keeps the build from competing
with everything else for memory. `aru doctor` reads applications, not
libraries; `tests/Unit/audit_test.go` compares `arandu.mod.toml` with what the
code calls instead.

## What this repository holds

```
module.go       foundation.Module: Name, Routes (none)
config.go       Config, Validate, WithDefaults
client.go       Client, New, Request, Query, Check
async.go        QueryAsync, Ticket, ParseCallback, CompleteAsync, verifiers
meter.go        Ledger, Dedupe, Meter, MemoryLedger, MemoryDedupe
outcome.go      Outcome, Classify, Billable
result.go       Result, MetaData, As
product.go      Product, Param, roles, DefaultProducts
document.go     CPF/CNPJ checking and masking (arandu-br)
redact.go       Redact
breaker.go      the circuit breaker
token.go        Token
flex.go         Text, Number, Flag
zz_catalog_*.go generated: products, response types, per-API methods
catalog/        catalog.json, read from the public documentation
cmd/directd-catalog  the documentation crawler
cmd/directd-smoke    the capped real run
internal/catalog     the crawler's parser and fetcher
internal/gen         the generator (go generate runs internal/gen/run)
internal/pii         personal-data classes of response keys
internal/smoke       the smoke run's body
```

Tests live under `tests/Unit` (`package unit_test`) and `tests/Feature`
(`package feature_test`), importing the package by its module path.

## What does not exist here, on purpose

| A change reaches for | What is here instead |
| --- | --- |
| an `Approved`, `IsRegular` or `Risk` field on `Result` | the source's data and the `Outcome` of the query; the application decides |
| a test that "just checks" the real API | the `httptest` double, driven by the catalogue's own examples |
| a balance, a table or a cache in the package | `Ledger` and `Dedupe`, implemented by the application |
| a tenant read from the request | the `Meter` the application builds from the Grant |
| a route that queries a document | nothing; the application's controllers, behind its policies |
| a guessed resultadoId or callback scheme | `Config.ResultCodes`, `Config.CallbackVerifier`, `Config.CallbackURLParam` |
| editing `zz_catalog_*.go` by hand | `catalog/catalog.json` and `go generate ./...` |

## Writing code

Everything in the source is in English: identifiers, doc comments, comments,
error and log messages, test names and messages. Every exported symbol carries
a doc comment that documents the symbol and nothing else. Agent instructions
live only in this file and in `.agents/skills/`.
