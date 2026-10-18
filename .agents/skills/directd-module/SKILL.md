---
name: directd-module
description: Change this Arandu package — the client, the meter, the outcomes, the async flow, the catalogue, the generator or a command. Use when the request is to "add a Direct Data API", "update the catalogue", "re-crawl the documentation", "regenerate", "go generate", "change a price", "map a resultadoId", "add an outcome", "change the redaction", "add a field to Result", "support the callback", "fix the breaker", "run the smoke test", or when a change touches client.go, async.go, meter.go, outcome.go, result.go, product.go, redact.go, internal/gen, internal/catalog or cmd/. Covers the no-real-call rule, the order of a call, settle-or-release, how the catalogue is read and regenerated, what is undocumented and must stay configuration, and the tests that hold each rule.
license: MIT
---

# Changing Arandu Direct Data

## Never call the API

Every Direct Data query is billed. Tests use the double in
`tests/Feature/double_test.go`. `cmd/directd-smoke` is the only program that
queries for real, and only the owner runs it, with a spend cap.

## The order of a call

`Client.query` in `client.go`: check product and document → Dedupe.Seen →
breaker → Ledger.Reserve → request → classify → Settle (billable) or Release →
Dedupe.Remember (billable). Anything refused before the request makes no
request and holds nothing; `tests/Feature/client_test.go` counts the requests.

## The catalogue

```bash
go run ./cmd/directd-catalog -out catalog/catalog.json -save /tmp/directd-pages
go generate ./...
```

The crawler reads only `directd.com.br/central-de-ajuda/apis/catalogo` and one
page per API, pauses between requests, refuses the API and portal hosts, and
stops on 401/403/429. `-index-only <dir>` re-parses saved pages offline. A page
that fails leaves its entry `incomplete` with the reason. Never fill a gap by
hand from memory: a parameter or field not in the documentation is not in the
catalogue. Commit the catalogue and the regenerated `zz_catalog_*.go` and
README together; `TestTheGeneratedFilesAreCurrent` holds them equal.

## What is undocumented and stays configuration

- `metaDados.resultadoId` values → `Config.ResultCodes`.
- How Direct Data authenticates a callback → `Config.CallbackVerifier`.
- The parameter of a per-request callback URL → `Config.CallbackURLParam`.
- The value `GERARCOMPROVANTE` expects → passed through as given.

## Outcomes

`Classify` in `outcome.go`. Documented: HTTP 400/401/404/500 (quickstart), the
error envelope (introduction), "Sucesso" and "Sucesso com Ressalvas" (terms
clause 2.5). The word rules are undocumented and say so. A new outcome is a
changelog line, and `Outcome.known` and `Billable` are updated with it.

## Evidence, never decisions

No `Result` field says approved, rejected, regular or trusted;
`tests/Unit/decision_test.go` walks the source and the type. Booleans on
`Result` are facts about the call (`Cached`, `Async`).
