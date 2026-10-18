# Changelog

Everything worth knowing about a release of Arandu Direct Data is recorded
here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and
the versions follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

A published module version is immutable: Go serves it from the proxy forever, so
a release is corrected by another release and never by moving a tag.

## [Unreleased]

The first release, proposed as v0.1.0. It requires
`github.com/arandu-io/framework` v0.57.1 and `github.com/hyz-is/arandu-br`
v0.3.0. No test calls Direct Data; the client has not yet been run against the
live API.

### Added

- `Client` (`New(Config)`), a `foundation.Module` with no route, querying the
  Direct Data API with the token as the `TOKEN` query parameter, redirects
  refused and transport errors stripped of the URL. `Query`, `Check`,
  `Product`, `Products`, `BreakerState`.
- Every API of the public catalogue, read on 2026-10-18 into
  `catalog/catalog.json` (154 APIs in 13 categories): `DefaultProducts` with
  path, parameters and roles, list price in cents, supplier flag, status and
  documentation URL; a generated method, parameter struct and response type
  per API; `As` for the typed data. Response fields use `Text`, `Number` and
  `Flag`, and carry `pii` tags.
- `Meter`, `Ledger` and `Dedupe`: the price is held before the request,
  settled for a billable outcome (terms clause 2.5) and released otherwise; a
  same-day repeat may be answered from `Dedupe`. `MemoryLedger` and
  `MemoryDedupe` for tests and commands. `ErrBudgetExceeded`, `ErrNoLedger`.
- `Outcome` and `Classify`: found, found_with_caveats, not_found,
  invalid_document, unavailable, insufficient_balance, unauthorized,
  rate_limited and error, from the HTTP status, the error envelope's
  `httpStatusCode`, `metaDados.resultado`, and `Config.ResultCodes`.
- `Result` as evidence: product, masked document, request time, elapsed time,
  consultaUid, list price, charged amount, cached, outcome, status, message,
  receipt URL, metadata, raw retorno and body, typed data.
- CPF and alphanumeric CNPJ check digits, with `arandu-br`, before any
  request; `MaskDocument`.
- `QueryAsync`, `Ticket`, `ParseCallback` (refused without a
  `CallbackVerifier`), `CompleteAsync`, `SecretQueryVerifier`,
  `SecretHeaderVerifier`.
- A circuit breaker (`BreakerConfig`, `ErrUnavailable`).
- `Redact`, for logs and fixtures without personal data.
- `Token`, which never prints.
- `cmd/directd-catalog`, which regenerates the catalogue from the public
  documentation only, and `go generate` (`internal/gen`) for the products,
  types, methods and the README's API table.
- `cmd/directd-smoke`, a capped run of real queries for the owner: token from
  `DIRECTD_TOKEN_FILE`, `-spend-limit-cents` at most 5000, the cap held before
  each request, `-dry-run`, redacted answers outside every git repository.
