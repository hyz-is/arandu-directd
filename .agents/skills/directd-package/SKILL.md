---
name: directd-package
description: Install, wire and use the Arandu Direct Data package (Go, Arandu) in an application. Use when the request is to "install arandu-directd", "query Direct Data", "consultar CPF na Direct Data", "consultar CNPJ", "Cadastro PF Plus", "Receita Federal PF", "Similarity", "AML vínculos", "PEP", "go get github.com/hyz-is/arandu-directd", "wire it into bootstrap/app.go", "budget exceeded", "no ledger", "invalid document", "callback not verified", "circuit breaker open", or when a project's go.mod already requires github.com/hyz-is/arandu-directd. Covers the wiring and where it goes, the Meter the application builds from the Grant, the Ledger and Dedupe it implements, the generated per-API methods, the outcomes, async callbacks, and what each error means.
license: MIT
metadata:
  audience: app
---

# Using Arandu Direct Data

A client for every API of the Direct Data catalogue. It returns evidence --
the query, its price, Direct Data's answer and the data, typed -- and decides
nothing. **Every query is billed to the token's owner.** Never run one from a
test or to "check" something; tests use an `httptest` double.

## The wiring, and where it goes

All of it is in `bootstrap/app.go`.

```go
import directd "github.com/hyz-is/arandu-directd"
```

In `Build`, before `k.Register`, with every value from the application's
typed configuration (the package reads no environment variable and no file):

```go
	directdClient, err := directd.New(directd.Config{
		Token:  directd.Token(cfg.DirectData.Token),
		Logger: logger,
	})
	if err != nil {
		return App{}, err
	}
```

Inside the `k.Register(...)` call: `directdClient,` (it adds no route). Pass
`directdClient` to the services that query.

## Each query

1. The controller authorizes the action (`Policy` → `security.Authorize` →
   `Grant`).
2. The service builds `directd.Meter{Ledger: ..., Dedupe: ...}` for
   `data.Tenant(g)`: the `Ledger` over the tenant's wallet (`arandu-wallet`),
   the `Dedupe` over the application's own storage, encrypted. Never take the
   tenant from the request.
3. It calls the generated method:

```go
res, pf, err := directdClient.CadastroPessoaFisicaPlus(ctx, meter, directd.CadastroPessoaFisicaPlusParams{CPF: cpf})
```

4. It records `res` as evidence (consultaUid, outcome, price, masked document,
   `Retorno` encrypted) and lets the application's policy decide.

## Symptoms to recognise

- **`no ledger`** — the Meter has no Ledger; every query needs one.
- **`budget exceeded`** — the Ledger refused the hold; no request was made.
- **`invalid document`** — check digits or the wrong kind (a CNPJ to a PF
  API); no request was made.
- **`invalid parameters`** — an undocumented name, TOKEN or ASync in Params,
  or a document passed in Params instead of `Request.Document`.
- **`product disabled`** — disabled in `Config.Products`, or inactive.
- **`circuit breaker is open`** — the source kept failing; wait for the
  cooldown.
- **Outcome `error` with a message** — an answer the classifier does not
  know; once its `resultadoId` is understood, map it in `Config.ResultCodes`.
- **`callback not verified`** — set `Config.CallbackVerifier`; the
  documentation does not say how Direct Data authenticates callbacks.
- **`Cached: true`** — Dedupe answered a same-day repeat; nothing was billed.
