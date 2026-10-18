<p align="center">
  <img src=".github/logo.svg" alt="Arandu" width="180">
</p>

# Arandu Direct Data

A Go client for the [Direct Data](https://directd.com.br) API, the Brazilian
data bureau: CPF and CNPJ registry data, sanctions and restriction lists,
court records, tax status, vehicles, rural producers, environmental records
and more. It covers **every one of the 154 APIs** in Direct Data's public
catalogue, each with a typed request builder and a typed response, generated
from a catalogue read off the public documentation.

It returns **evidence, never a decision**. A `Result` says which API was asked
about which document (masked), when, at what price, what Direct Data answered
(its `consultaUid`, status and message), the `Outcome` of the query, the raw
`retorno` and the `retorno` decoded into its generated type. Whether a record
supports a case is the application's policy.

Every query costs money, so every call goes through a **Meter**: a `Ledger` the
application implements holds the price before the request and settles or
releases it after, and an optional `Dedupe` answers a same-day repeat, which
Direct Data bills again, without a request. A CPF or CNPJ is checked (check
digits, alphanumeric CNPJ included, with
[`arandu-br`](https://github.com/hyz-is/arandu-br)) before anything is
reserved or sent, and a circuit breaker stops calling a source that keeps
failing.

## What it is not

- **Not a verdict.** No field says a person or company is regular, approved,
  trusted or risky. `found` means the source returned a record. The provider's
  own fields (a `situacaoCadastral`, a score, a `riscoFraude`) come back as
  data, exactly as Direct Data sent them.
- **Not a wallet.** It keeps no balance, no hold and no remembered result. The
  `Ledger` and `Dedupe` interfaces are implemented by the application, over
  its wallet (`arandu-wallet`) and its own storage, bound to the tenant of the
  Grant the query is made under.
- **Not a tenant reader.** It never looks at a request, a path, a header or a
  body. The application builds the `Meter` from an authorized Grant and passes
  it in.
- **Not a route.** `Routes` registers nothing: a query costs money and returns
  personal data, so it is made through the application's controllers, behind
  its policies.
- **Not a store of personal data.** Nothing is written to disk or logged
  except one log record per call with the product, the masked document, the
  outcome, the status, the timing and the `consultaUid`.

## Requirements

| | |
| --- | --- |
| Go | 1.26 or later |
| Arandu | framework 0.57 or later, `github.com/hyz-is/arandu-br` |
| Direct Data | an account and its API token (portal: Configurações → Integrações) |

## Install

```bash
go get github.com/hyz-is/arandu-directd@v0.1.0
```

## Wire it

An Arandu application builds the client explicitly. There is no service
provider, no container and no discovery, so these are the lines to write into
`bootstrap/app.go` and there are no others.

The import, with the other module imports:

```go
import (
	directd "github.com/hyz-is/arandu-directd"
)
```

The construction, in `Build`, before `k.Register`. Every value comes from the
application's own typed configuration; this package reads no environment
variable and no file:

```go
	directdClient, err := directd.New(directd.Config{
		Token:  directd.Token(cfg.DirectData.Token),
		Logger: logger,
		Products: map[string]directd.Product{
			// Disable every API the contract or the purpose does not cover.
			"CNJMandadosPrisao": {Disabled: true},
		},
	})
	if err != nil {
		return App{}, err
	}
```

The registration, inside the `k.Register(...)` call already there, so the
application lists the module (it adds no route, no table and no loop):

```go
		directdClient,
```

and the client is passed, as a constructor argument, to the services that
query.

## Configuration

| Field | Default | What it does |
| --- | --- | --- |
| `Token` | required | The API token, sent as the `TOKEN` query parameter. A `Token` never prints: `String`, `%#v`, `LogValue` and JSON all write `[redacted]` |
| `BaseURL` | `https://apiv3.directd.com.br` | The API host; https only, except a loopback test double |
| `HTTPClient` | a new `http.Client` | Copied; every redirect is refused, so the token never travels elsewhere |
| `Timeout` | 30 s | Bound on one request |
| `Products` | none | Overrides or additions by slug: a contract price, `Disabled`, an API newer than the catalogue |
| `Breaker` | 5 failures, 30 s | Consecutive transport errors or 5xx answers that open the breaker, and how long it stays open |
| `Location` | UTC−3 | Where a day starts and ends for `Dedupe` |
| `Logger` | none | One record per call, never the token, the URL or a response value |
| `ResultCodes` | none | `metaDados.resultadoId` values mapped to outcomes, once observed |
| `CallbackVerifier` | none | Authenticates an asynchronous callback; without one every callback is refused |
| `CallbackURLParam` | none | The parameter carrying a per-request callback URL, which the documentation does not name |
| `MaxBodyBytes` | 8 MiB | Bound on the answer read |

## Query

Every API has a generated method, named after its slug, with a typed
parameter struct and a typed result:

```go
meter := directd.Meter{Ledger: walletLedger, Dedupe: dayCache} // built from the Grant's tenant

res, pf, err := directdClient.CadastroPessoaFisicaPlus(ctx, meter,
	directd.CadastroPessoaFisicaPlusParams{CPF: cpf})
if err != nil {
	return err // no answer could be read, or the Ledger refused
}
// res: Product, Document (masked), RequestedAt, ElapsedMs, ConsultaUID,
//      PriceCents, ChargedCents, Cached, Outcome, HTTPStatus, Status, Message,
//      ReceiptURL, Meta, Retorno, Body, Data, DataError
// pf:  *directd.CadastroPessoaFisicaPlusRetorno (nil when nothing decoded)
switch res.Outcome {
case directd.OutcomeFound, directd.OutcomeFoundWithCaveats:
	_ = pf.SituacaoCadastral // "REGULAR", as Direct Data sent it
case directd.OutcomeNotFound:
}
```

The same query without the generated method, for an API added later through
`Config.Products`:

```go
res, err := directdClient.Query(ctx, meter, directd.Request{
	Product:  "ReceitaFederalPessoaFisica",
	Document: cpf,                                         // checked, sent bare in Cpf
	Params:   map[string]string{"DataNascimento": birth}, // other documented parameters
})
data, ok := directd.As[directd.ReceitaFederalPessoaFisicaRetorno](res)
```

`Client.Check(req)` says why a request would be refused without costing
anything. A query Direct Data answered returns a nil error whatever its
outcome; `not_found` is an answer.

The response fields are typed from the documented examples with tolerant
scalars: `Text` (a string, or a number or boolean sent where a string was
documented), `Number` (kept as its text, with `Int64` and `Float64`) and
`Flag` (a boolean, or `"true"`/`"false"`). A retorno that still does not decode
leaves `Data` nil and says why in `DataError`; `Retorno` keeps the bytes either
way. Each generated field carries a `pii:"..."` tag (`document`, `name`,
`address`, `phone`, `email`, `birthdate`, `vehicle`, ...) when its key holds
personal data.

## Costs, budget and dedupe

The hooks the application implements:

```go
type Ledger interface {
	Reserve(ctx context.Context, c Charge) (holdID string, err error) // wrap ErrBudgetExceeded to refuse
	Settle(ctx context.Context, holdID string, cents int64) error
	Release(ctx context.Context, holdID string) error
}

type Dedupe interface {
	Seen(ctx context.Context, key DedupeKey) (*Result, bool, error)
	Remember(ctx context.Context, key DedupeKey, r *Result) error
}
```

The order of a call:

1. The product and the document are checked. An invalid document is refused
   with `ErrInvalidDocument` before anything else; no request, no hold.
2. `Dedupe.Seen` may answer with a result remembered for the day
   (`DedupeKey{Product, Document, ParamsDigest, Day}`); it comes back
   `Cached: true`, `ChargedCents: 0`, and no request is made.
3. An open circuit breaker refuses with `ErrUnavailable`; no request, no hold.
4. `Ledger.Reserve` holds the list price; a refusal (`ErrBudgetExceeded`)
   means no request.
5. The request is made.
6. The hold is **settled** at the list price when the outcome is billable
   (`found`, `found_with_caveats`; terms clause 2.5) and **released**
   otherwise, a transport failure included.
7. A billable result is remembered with `Dedupe.Remember`.

`MemoryLedger` and `MemoryDedupe` are in-memory implementations for tests and
commands; they are not storage. The prices are Direct Data's documented list
prices; a contract that prices differently overrides them in
`Config.Products`, and the application reconciles its ledger with Direct
Data's statement by `consultaUid`.

## Outcomes

| Outcome | When | Source |
| --- | --- | --- |
| `found` | `metaDados.resultado` is "Sucesso" | [Terms, clause 2.5](https://www.directd.com.br/termos) |
| `found_with_caveats` | "Sucesso com Ressalvas" | Terms, clause 2.5 |
| `not_found` | HTTP 404, or a status or message saying no record was found | [Quickstart](https://directd.com.br/central-de-ajuda/apis/quickstart) (404); the words are undocumented |
| `invalid_document` | HTTP 400 or 422, or a message saying a value is invalid | Quickstart (400 "parâmetro inválido") |
| `unauthorized` | HTTP 401 or 403, or a message about the token | Quickstart (401 "token inválido") |
| `insufficient_balance` | HTTP 402, or a message about balance | undocumented |
| `rate_limited` | HTTP 429 | undocumented; the [introduction](https://directd.com.br/central-de-ajuda/apis/introducao) says there are no rate limits |
| `unavailable` | transport failure, timeout, HTTP 5xx, or a message saying the source is down | Quickstart (500) |
| `error` | anything else; `Message` and `Errors` keep what Direct Data said | — |

The HTTP status is the answer's, or the `httpStatusCode` field of the error
envelope (`{httpStatusCode, message, errors[]}`, introduction) when the body
carries one. **The public documentation lists no `resultadoId` values**, so
none is mapped by default: `Config.ResultCodes` takes them once observed, and
they take precedence over everything else.

## Asynchronous queries

Every product page offers `ASync=habilitar`, at no extra cost, with the result
delivered to a webhook.

```go
ticket, err := directdClient.QueryAsync(ctx, meter, req, directd.AsyncOptions{})
// store ticket (ConsultaUID, HoldID, PriceCents); the hold stays open

// in the application's callback controller:
res, err := directdClient.ParseCallback(directd.CallbackRequest{Header: r.Header, Query: r.URL.Query(), Body: body})
ticket := findTicket(res.ConsultaUID)
err = directdClient.CompleteAsync(ctx, meter.Ledger, ticket, res) // settles or releases, once
```

**The documentation does not say how Direct Data authenticates a callback, nor
which parameter carries a per-request callback URL** (the callback appears to
be configured in the portal). So `ParseCallback` refuses every callback until a
`CallbackVerifier` is configured, and `AsyncOptions.CallbackURL` is refused
until `Config.CallbackURLParam` names the parameter. `SecretQueryVerifier`
checks a secret of the application's own in the registered callback URL
(`https://app.example/directd/callback?k=<secret>`), in constant time;
`SecretHeaderVerifier` checks a header, should the portal allow one. A secret
in a URL is only as private as the logs of every hop that sees it; the
`consultaUid` match `CompleteAsync` requires is a second check, not a
substitute.

## Redaction

`Redact(body)` rewrites an answer for logs, tickets or fixtures: the structure
is kept, a CPF or CNPJ becomes its mask, names, addresses, phones, e-mails,
birth dates and vehicle identifiers become placeholders, status-like values
(`situacaoCadastral`, `resultado`, `cnaeDescricao`, ...) are kept unless they
hold a run of digits or an `@`, and every other string becomes `<string>`.

## The catalogue

`catalog/catalog.json` holds every API as the public documentation describes
it, with the crawl date and the source page of each entry: slug, name,
category, target (PF, PJ, PF/PJ), endpoint path and template, method,
parameters (name, spelling in the template, type, required flag as
documented, description), list price in cents, supplier flag (origin
*Online*: the source is queried at call time), status (available or
maintenance), whether a PDF receipt exists, the documented response example,
its field paths, and the field labels and descriptions. Entries the
documentation leaves incomplete say so, with the reason; nothing is invented to
fill them.

```bash
go run ./cmd/directd-catalog -out catalog/catalog.json   # reads only the public documentation
go generate ./...                                         # products, types, methods, the table below
```

`cmd/directd-catalog` reads `https://directd.com.br/central-de-ajuda/apis/catalogo`
and one page per API, with a pause between requests, refuses the API and the
portal hosts by name, and stops without retrying on a 401, 403 or 429. A test
holds the generated files equal to what the generator writes from the
committed catalogue.

## Checking against the real API

`cmd/directd-smoke` makes a **capped** run of real queries, each billed to
the token's owner:

```bash
DIRECTD_TOKEN_FILE=$HOME/.config/directd/token \
go run ./cmd/directd-smoke -spend-limit-cents 300 -out /tmp/directd-smoke \
  -call 'CadastroPessoaFisicaPlus <cpf>' \
  -call 'ReceitaFederalPessoaFisica <cpf> DataNascimento=<dd/mm/aaaa>' \
  -dry-run
```

The token comes from the file `DIRECTD_TOKEN_FILE` names, never an argument,
and is never printed. `-spend-limit-cents` is required and refused above 5000
(R$ 50,00); each call's list price is held on a capped ledger before the
request, a refused query counts as spent, and the run stops at the first call
that would pass the cap. `-dry-run` prints the plan and its cost without
reading the token. One line per call: product, masked document, HTTP status,
outcome, status, message (redacted), `consultaUid`, elapsed time, price and
running total. Each answer is written redacted to `-out`, which must be
outside every git repository.

## Security and privacy

CPF and CNPJ registry data, kinship, addresses, phones and court records are
personal data under the LGPD. This package holds none of it beyond the call:
it stores nothing and logs only the masked document. Collecting only what is
needed, encrypting `Retorno` and `Body` apart from the normalized facts,
controlling and auditing every access (`arandu-activitylog`), a lawful basis
and deletion on schedule are the application's.

The token is a credential that spends money. It travels only in the request
URL, because Direct Data authenticates that way; transport errors are
stripped of the URL, redirects are refused, and the base URL must be https.
Report vulnerabilities privately; see [SECURITY.md](SECURITY.md).

## Direct Data's terms that bind the integrating application

From the [terms of use](https://www.directd.com.br/termos), read on
2026-10-18. They are the provider's, not this package's, and the application's
contract with Direct Data governs.

- **2.5** — a query is billed when its status is "Sucesso" or "Sucesso com
  Ressalvas"; a duplicate (same document, same API, same day) is billed again.
- **2.3.1** — APIs that depend on outside sources may become unavailable.
- **7.4 (ii)** — the client may not disclose the data to third parties,
  directly or indirectly. An application that shows results to its own
  customers must confirm its contract allows it.
- **8.1 and 8.3** — Direct Data may collect, store and relate the queries
  made, and may sell products derived from them, with the client's personal
  data and its customers' removed or anonymised. The application's privacy
  notice should say so.
- **9.2** — personal data of individuals may only be processed for credit
  assessment, entering into contracts, collection, or legitimate interest.

## Uso no KUAA

O KUAA consulta a Direct Data **somente depois** que o documento foi extraído
e validado: o OCR leu o documento, o CPF ou CNPJ passou nos dígitos
verificadores e o caso tem uma finalidade registrada. A consulta é evidência
do caso, nunca a decisão: o `Result` é gravado (cifrado, com o
`consultaUid`) e a política do KUAA decide.

- **Por onde passa.** O controller do KUAA autoriza a ação (`Policy` →
  `security.Authorize` → `Grant`), monta o `Meter` com o `Ledger` sobre a
  carteira do tenant (`arandu-wallet`, tenant de `data.Tenant(g)`) e o
  `Dedupe` sobre o armazenamento do próprio KUAA, e chama o método gerado. O
  módulo nunca lê tenant de requisição.
- **O que consultar primeiro.**

  | API | Slug | Preço de tabela |
  | --- | --- | --- |
  | Cadastro PF Básica | `CadastroPessoaFisica` | R$ 0,16 |
  | Cadastro PF Plus (situação na Receita, óbito, parentescos) | `CadastroPessoaFisicaPlus` | R$ 0,36 |
  | Receita Federal PF (CPF + data de nascimento, comprovante) | `ReceitaFederalPessoaFisica` | R$ 0,36 |
  | Cadastro PJ Básica | `CadastroPessoaJuridica` | R$ 0,16 |
  | Cadastro PJ Plus (QSA, filiais, tributação) | `CadastroPessoaJuridicaPlus` | R$ 0,36 |
  | AML — vínculos societários (óbito, PEP, parentescos, sociedades) | `AML` | R$ 0,72 |
  | Similarity — Brazil (dados informados × base, por campo) | `Similarity` | R$ 0,72 |
  | Simples Nacional | `SimplesNacional` | R$ 0,36 |
  | Processos Judiciais — Simplificada | `ProcessosJudiciaisSimplificada` | R$ 1,50 |

  O `Similarity` é o que compara o que o OCR leu com a base: cada campo volta
  com `description` ("MATCH EXATO", "MATCH PARCIAL") e `distance`; a política
  do KUAA decide o que cada distância significa.
- **Custo.** Toda consulta é cobrada; a repetida no mesmo dia também (cláusula
  2.5). O `Dedupe` evita a repetição; o `Ledger` segura o preço antes e só
  liquida quando o status é cobrável. Desative em `Config.Products` toda API
  fora da finalidade.
- **Termos.** Antes de mostrar resultado da Direct Data ao cliente do KUAA,
  confirmar que o contrato permite (cláusula 7.4(ii)). Informar no aviso de
  privacidade que o fornecedor guarda as consultas e pode comercializar
  derivados anonimizados (8.1 e 8.3). Dados de pessoa física só para as
  finalidades da cláusula 9.2.
- **Callback.** Enquanto a Direct Data não documentar como autentica o
  webhook, usar consulta síncrona, ou registrar no portal uma URL com segredo
  próprio e `SecretQueryVerifier`.

## Tests

```bash
export GOWORK=off
test -z "$(gofmt -l $(find . -name '*.go' -not -path '*/testdata/*'))"
go build ./...
go vet ./...
go test -race -count=1 ./...
```

**No test calls Direct Data.** Every request goes to an `httptest` double:
every catalogue API is driven through it with its documented path, token
placement and parameters, and answered with its own documented example;
synthetic answers check the typed KYC products, the outcomes, the
settle-or-release contract, dedupe, the budget, the breaker, the async flow,
redaction, the smoke command's cap, and that neither the token nor a personal
value reaches a log, an error or a file. The documents in the tests have valid
check digits and belong to nobody known.

## Versioning and releases

The module follows [Semantic Versioning](https://semver.org/). While the major
version is `v0`, a minor release may break the API, and every break is named in
[UPGRADE.md](UPGRADE.md). A new catalogue changes products, prices, types and
methods, and is always a changelog line. A release is a tag `vX.Y.Z` on `main`
whose notes are the `## [X.Y.Z] - date` entry in [CHANGELOG.md](CHANGELOG.md).

## Licence

The code is MIT; see [LICENSE.md](LICENSE.md). Copyright HYZIS - Serviços
Digitais Ltda. Direct Data is a trademark of its owner; this package is not
affiliated with it, and the catalogue records facts from its public
documentation.

## API catalogue

<!-- catalog:start -->

154 APIs in 13 categories, read from the [public catalogue](https://directd.com.br/central-de-ajuda/apis/catalogo) on 2026-10-18. Prices are list prices per query; *Online* means the source is queried at call time. The slug is `Request.Product`, and the generated method on `*directd.Client` is the slug as a Go name (`SintegraCCC-Completa` → `SintegraCCCCompleta`).

### Ambiental (5)

| API | Slug | Target | Parameters | Price | Origin | Notes |
| --- | --- | --- | --- | --- | --- | --- |
| [IBAMA - Certidão Negativa de Débitos](https://directd.com.br/central-de-ajuda/apis/catalogo/IBAMACertidaoNegativaDebitos) | `IBAMACertidaoNegativaDebitos` | PF/PJ | `Cpf`, `Cnpj`, `GERARCOMPROVANTE` | R$ 0,36 | Online | maintenance; PDF receipt |
| [IBAMA - Certidão Negativa de Embargo](https://directd.com.br/central-de-ajuda/apis/catalogo/IBAMACertidaoNegativaEmbargos) | `IBAMACertidaoNegativaEmbargos` | PF/PJ | `Cpf`, `Cnpj`, `GERARCOMPROVANTE` | R$ 0,36 | Online | maintenance; PDF receipt |
| [IBAMA - Certificado de Regularidade](https://directd.com.br/central-de-ajuda/apis/catalogo/IBAMACertificadoRegularidade) | `IBAMACertificadoRegularidade` | PF/PJ | `Cpf`, `Cnpj`, `GERARCOMPROVANTE` | R$ 0,36 | Online | maintenance; PDF receipt |
| [IBAMA - Consulta de Autuações Ambientais](https://directd.com.br/central-de-ajuda/apis/catalogo/IBAMAConsultaAutuacoesAmbientais) | `IBAMAConsultaAutuacoesAmbientais` | PF/PJ | `Cpf`, `Cnpj`, `GERARCOMPROVANTE` | R$ 0,36 | Online | maintenance; PDF receipt |
| [Suframa - CNPJ](https://directd.com.br/central-de-ajuda/apis/catalogo/SuframaCNPJ) | `SuframaCNPJ` | PJ | `Cnpj`, `GERARCOMPROVANTE` | R$ 0,36 | Online | PDF receipt |

### Cadastral (37)

| API | Slug | Target | Parameters | Price | Origin | Notes |
| --- | --- | --- | --- | --- | --- | --- |
| [Auxílio Emergencial](https://directd.com.br/central-de-ajuda/apis/catalogo/AuxilioEmergencial) | `AuxilioEmergencial` | PF | `CPF`, `NIS` | R$ 0,36 | Online |  |
| [Auxílio Reconstrução](https://directd.com.br/central-de-ajuda/apis/catalogo/AuxilioReconstrucao) | `AuxilioReconstrucao` | PF | `CPF`, `NIS` | R$ 0,36 | Base |  |
| [Benefício de Prestação Continuada - BPC](https://directd.com.br/central-de-ajuda/apis/catalogo/BeneficioPrestacaoContinuada) | `BeneficioPrestacaoContinuada` | PF | `Cpf`, `Nis` | R$ 0,36 | Online |  |
| [Benefícios Sociais - Pessoa Física](https://directd.com.br/central-de-ajuda/apis/catalogo/BeneficiosSociais) | `BeneficiosSociais` | PF | `CPF` | R$ 0,75 | Base |  |
| [Bolsa Família](https://directd.com.br/central-de-ajuda/apis/catalogo/BolsaFamilia) | `BolsaFamilia` | PF | `Nis`, `MesReferencia`, `AnoReferencia`, `MesCompetencia`, `AnoCompetencia` | R$ 0,36 | Online |  |
| [CFM - Conselho Federal de Medicina](https://directd.com.br/central-de-ajuda/apis/catalogo/CFMConselhoFederalMedicina) | `CFMConselhoFederalMedicina` | PF/PJ | `CRM`, `UF` | R$ 0,36 | Online | PDF receipt; incomplete: the documentation page lists no parameter |
| [CONFEA/CREA - Conselho Federal de Engenharia e Agronomia](https://directd.com.br/central-de-ajuda/apis/catalogo/CONFEA) | `CONFEA` | PF | `CPF`, `RegistroNacional`, `GERARCOMPROVANTE` | R$ 0,36 | Online | PDF receipt |
| [CVM - Comissão de Valores Mobiliários](https://directd.com.br/central-de-ajuda/apis/catalogo/CVMComissaodeValoresMobiliarios) | `CVMComissaodeValoresMobiliarios` | PF/PJ | `CPF`, `CNPJ` | R$ 0,36 | Online |  |
| [CVM - Processos Administrativos Sancionadores](https://directd.com.br/central-de-ajuda/apis/catalogo/CVMProcessosAdministrativosSancionadores) | `CVMProcessosAdministrativosSancionadores` | PF/PJ | `CPF`, `CNPJ` | R$ 0,36 | Online |  |
| [Cadastro - Pessoa Física - Básica](https://directd.com.br/central-de-ajuda/apis/catalogo/CadastroPessoaFisica) | `CadastroPessoaFisica` | PF | `CPF` | R$ 0,16 | Base |  |
| [Cadastro - Pessoa Física - Plus](https://directd.com.br/central-de-ajuda/apis/catalogo/CadastroPessoaFisicaPlus) | `CadastroPessoaFisicaPlus` | PF | `CPF` | R$ 0,36 | Base |  |
| [Cadastro - Pessoa Jurídica - Básica](https://directd.com.br/central-de-ajuda/apis/catalogo/CadastroPessoaJuridica) | `CadastroPessoaJuridica` | PJ | `CNPJ` | R$ 0,16 | Base |  |
| [Cadastro - Pessoa Jurídica - Plus](https://directd.com.br/central-de-ajuda/apis/catalogo/CadastroPessoaJuridicaPlus) | `CadastroPessoaJuridicaPlus` | PJ | `CNPJ` | R$ 0,36 | Base |  |
| [Cadastro + Receita Federal - Pessoa Física](https://directd.com.br/central-de-ajuda/apis/catalogo/CadastroReceitaPessoaFisica) | `CadastroReceitaPessoaFisica` | PF | `CPF`, `DATANASCIMENTO`, `GERARCOMPROVANTE` | R$ 0,42 | Online | PDF receipt |
| [CNH - Carteira Nacional de Habilitação](https://directd.com.br/central-de-ajuda/apis/catalogo/CarteiraNacionalHabilitacao) | `CarteiraNacionalHabilitacao` | PF | `CPF` | R$ 0,90 | Online |  |
| [CNH - Carteira Nacional de Habilitação Plus](https://directd.com.br/central-de-ajuda/apis/catalogo/CnhPlus) | `CnhPlus` | PF | `CPF` | R$ 1,45 | Online |  |
| [Correios - Busca CEP](https://directd.com.br/central-de-ajuda/apis/catalogo/CorreiosBuscaCEP) | `CorreiosBuscaCEP` | PF/PJ | `Endereco` | R$ 0,36 | Online |  |
| [Enriquecimento de Lead](https://directd.com.br/central-de-ajuda/apis/catalogo/EnriquecimentoLead) | `EnriquecimentoLead` | PF | `Celular`, `Email` | R$ 0,16 | Base |  |
| [Gás do Povo](https://directd.com.br/central-de-ajuda/apis/catalogo/GasDoPovo) | `GasDoPovo` | PF | `CPF` | R$ 0,36 | Online |  |
| [Identity Verification - Argentina](https://directd.com.br/central-de-ajuda/apis/catalogo/IdentityVerificationArgentina) | `IdentityVerificationArgentina` | PF | `DNI`, `Name`, `Surname`, `DOB`, `GENDER`, `Address`, `City`, `State`, `PostCode`, `Phone`, `Country` | R$ 0,72 | Base |  |
| [Ministério do Trabalho - PIS](https://directd.com.br/central-de-ajuda/apis/catalogo/MinisterioTrabalhoPIS) | `MinisterioTrabalhoPIS` | PF | `Cpf` | R$ 0,36 | Base |  |
| [OAB - Cadastro Nacional dos Advogados](https://directd.com.br/central-de-ajuda/apis/catalogo/OABCadastroNacionalAdvogados) | `OABCadastroNacionalAdvogados` | PF | `Nome`, `INSCRICAO`, `UF`, `GERARCOMPROVANTE`, `IE` | R$ 0,36 | Online | PDF receipt |
| [Óbito](https://directd.com.br/central-de-ajuda/apis/catalogo/Obito) | `Obito` | PF | `Cpf` | R$ 0,36 | Base |  |
| [Portal da Transparência - Indicadores Pessoa Física](https://directd.com.br/central-de-ajuda/apis/catalogo/PortalTransparenciaIndicadoresPessoaFisica) | `PortalTransparenciaIndicadoresPessoaFisica` | PF | `CPF`, `NIS` | R$ 1,30 | Online |  |
| [Portal da Transparência - Indicadores Pessoa Jurídica](https://directd.com.br/central-de-ajuda/apis/catalogo/PortalTransparenciaIndicadoresPessoaJuridica) | `PortalTransparenciaIndicadoresPessoaJuridica` | PJ | `CNPJ` | R$ 1,30 | Online |  |
| [Portal da Transparência - Servidores do Poder Executivo Federal](https://directd.com.br/central-de-ajuda/apis/catalogo/PortalTransparenciaServidores) | `PortalTransparenciaServidores` | PF | `CPF` | R$ 0,72 | Online |  |
| [Portal da Transparência - Servidores - Completa](https://directd.com.br/central-de-ajuda/apis/catalogo/PortalTransparenciaServidoresCompleta) | `PortalTransparenciaServidoresCompleta` | PF | `CPF`, `GERARCOMPROVANTE` | R$ 2,30 | Online | PDF receipt |
| [Portal da Transparência - Servidores - Remuneração](https://directd.com.br/central-de-ajuda/apis/catalogo/PortalTransparenciaServidoresRemuneracao) | `PortalTransparenciaServidoresRemuneracao` | PF | `CPF`, `MesAno` | R$ 0,36 | Online |  |
| [Portal da Transparência - Servidores Viagens e despesas](https://directd.com.br/central-de-ajuda/apis/catalogo/PortalTransparenciaServidoresViagens) | `PortalTransparenciaServidoresViagens` | PF | `CPF` | R$ 0,36 | Online |  |
| [Registration Data - Argentina](https://directd.com.br/central-de-ajuda/apis/catalogo/RegistrationDataArgentina) | `RegistrationDataArgentina` | PF | `DNI` | R$ 0,36 | Base |  |
| [Registration Data - Brazil](https://directd.com.br/central-de-ajuda/apis/catalogo/RegistrationDataBrazil) | `RegistrationDataBrazil` | PF | `Cpf`, `Name`, `Surname`, `DOB` | R$ 0,36 | Base |  |
| [Registration Data - México](https://directd.com.br/central-de-ajuda/apis/catalogo/RegistrationDataMexico) | `RegistrationDataMexico` | PF | `Curp` | R$ 0,36 | Base |  |
| [Seguro Defeso](https://directd.com.br/central-de-ajuda/apis/catalogo/SeguroDefeso) | `SeguroDefeso` | PF | `Cpf`, `Nis` | R$ 0,36 | Online |  |
| [Similarity - Brazil](https://directd.com.br/central-de-ajuda/apis/catalogo/Similarity) | `Similarity` | PF | `Cpf`, `Name`, `Surname`, `DOB`, `Address`, `City`, `State`, `PostCode`, `Phone` | R$ 0,72 | Base |  |
| [Similarity - Argentina](https://directd.com.br/central-de-ajuda/apis/catalogo/SimilarityArgentina) | `SimilarityArgentina` | PF | `DNI`, `Name`, `Surname`, `DOB`, `Address`, `City`, `State`, `PostCode`, `Phone` | R$ 0,72 | Base |  |
| [Similarity - México](https://directd.com.br/central-de-ajuda/apis/catalogo/SimilarityMexico) | `SimilarityMexico` | PF | `CURP`, `Name`, `Surname`, `DOB`, `Address`, `City`, `State`, `PostCode`, `Phone` | R$ 0,72 | Base |  |
| [Vínculo Empregatício](https://directd.com.br/central-de-ajuda/apis/catalogo/VinculoEmpregaticio) | `VinculoEmpregaticio` | PJ | `CNPJ` | R$ 0,72 | Base |  |

### Crédito (15)

| API | Slug | Target | Parameters | Price | Origin | Notes |
| --- | --- | --- | --- | --- | --- | --- |
| [Antifraude Chave PIX](https://directd.com.br/central-de-ajuda/apis/catalogo/AntifraudePix) | `AntifraudePix` | PF/PJ | `Documento`, `Chave`, `Tipo` | R$ 1,55 | Base | maintenance |
| [Boa Vista - Acerta Completo Positivo Pessoa Física](https://directd.com.br/central-de-ajuda/apis/catalogo/BoaVistaAcertaCompletoPositivoPF) | `BoaVistaAcertaCompletoPositivoPF` | PF | `CPF`, `GERARCOMPROVANTE` | R$ 14,03 | Online | PDF receipt |
| [Boa Vista - Acerta Mais Positivo Pessoa Física](https://directd.com.br/central-de-ajuda/apis/catalogo/BoaVistaAcertaMaisPositivoPF) | `BoaVistaAcertaMaisPositivoPF` | PF | `CPF`, `GERARCOMPROVANTE` | R$ 11,18 | Online | PDF receipt |
| [Boa Vista - Define Limite Positivo Pessoa Jurídica](https://directd.com.br/central-de-ajuda/apis/catalogo/BoaVistaDefineLimitePositivoPJ) | `BoaVistaDefineLimitePositivoPJ` | PJ | `CNPJ`, `GERARCOMPROVANTE` | R$ 21,00 | Online | PDF receipt |
| [Boa Vista - Risco Positivo Pessoa Jurídica](https://directd.com.br/central-de-ajuda/apis/catalogo/BoaVistaRiscoPositivoPJ) | `BoaVistaRiscoPositivoPJ` | PJ | `CNPJ`, `GERARCOMPROVANTE` | R$ 14,03 | Online | PDF receipt |
| [CADIN - Secretaria da Fazenda](https://directd.com.br/central-de-ajuda/apis/catalogo/CADINSecretariaFazendaEstaduais) | `CADINSecretariaFazendaEstaduais` | PF/PJ | `Cpf`, `Cnpj`, `Uf`, `GERARCOMPROVANTE` | R$ 0,36 | Online | PDF receipt |
| [CADIN - Secretaria da Fazenda - São Paulo](https://directd.com.br/central-de-ajuda/apis/catalogo/CADINSecretariaFazendaSP) | `CADINSecretariaFazendaSP` | PF/PJ | `Cpf`, `Cnpj`, `GERARCOMPROVANTE` | R$ 0,36 | Online | PDF receipt |
| [Detalhamento Negativo QUOD](https://directd.com.br/central-de-ajuda/apis/catalogo/DetalhamentoNegativo) | `DetalhamentoNegativo` | PF/PJ | `CPF`, `CNPJ` | R$ 2,38 | Online |  |
| [Dossiê QUOD - Completo](https://directd.com.br/central-de-ajuda/apis/catalogo/DossieCreditoCompleto) | `DossieCreditoCompleto` | PF/PJ | `Cpf`, `Cnpj` | R$ 6,99 | Online |  |
| [Protestos Nacional - IEPTB Básica](https://directd.com.br/central-de-ajuda/apis/catalogo/ProtestosNacionalBasica) | `ProtestosNacionalBasica` | PF/PJ | `Cnpj`, `Cpf` | R$ 2,95 | Online | incomplete: the documentation page lists no parameter |
| [Protestos Nacional - IEPTB Online](https://directd.com.br/central-de-ajuda/apis/catalogo/ProtestosOnline) | `ProtestosOnline` | PF/PJ | `Cnpj`, `Cpf` | R$ 3,50 | Online |  |
| [SCR Analítico - Resumo BACEN](https://directd.com.br/central-de-ajuda/apis/catalogo/SCRBacen) | `SCRBacen` | PF/PJ | `CNPJ`, `CPF` | R$ 3,90 | Base |  |
| [SCR Detalhada - Resumo BACEN](https://directd.com.br/central-de-ajuda/apis/catalogo/SCRBacenDetalhada) | `SCRBacenDetalhada` | PF/PJ | `CNPJ`, `CPF`, `MESANO`, `GERARCOMPROVANTE` | R$ 4,90 | Base | PDF receipt |
| [Score de Crédito - QUOD](https://directd.com.br/central-de-ajuda/apis/catalogo/Score) | `Score` | PF/PJ | `CPF`, `CNPJ` | R$ 1,98 | Online |  |
| [Nível Socioeconômico e Renda Estimada](https://directd.com.br/central-de-ajuda/apis/catalogo/nivelSocioeconomico) | `nivelSocioeconomico` | PF | `CPF` | R$ 0,36 | Base | incomplete: the documentation page lists no parameter |

### Fiscal e Tributário (15)

| API | Slug | Target | Parameters | Price | Origin | Notes |
| --- | --- | --- | --- | --- | --- | --- |
| [CCD - Certidão Conjunta de Débitos - Pessoa Física](https://directd.com.br/central-de-ajuda/apis/catalogo/CertidaoConjuntaDebitosPessoaFisica) | `CertidaoConjuntaDebitosPessoaFisica` | PF | `Cpf`, `DATANASCIMENTO`, `GERARCOMPROVANTE` | R$ 0,36 | Online | PDF receipt |
| [CCD - Certidão Conjunta de Débitos - Pessoa Jurídica](https://directd.com.br/central-de-ajuda/apis/catalogo/CertidaoConjuntaDebitosPessoaJuridica) | `CertidaoConjuntaDebitosPessoaJuridica` | PJ | `Cnpj`, `GERARCOMPROVANTE` | R$ 0,36 | Online | PDF receipt |
| [CND - Certidão Negativa de Débitos Estaduais](https://directd.com.br/central-de-ajuda/apis/catalogo/CertidaoNegativaDebitos) | `CertidaoNegativaDebitos` | PF/PJ | `Cnpj`, `ie`, `Cpf`, `Uf`, `GERARCOMPROVANTE` | R$ 0,36 | Online | PDF receipt |
| [CNDIR - Certidão Negativa de Débitos e Dívida Ativa da União de Imóvel Rural](https://directd.com.br/central-de-ajuda/apis/catalogo/CertidaoNegativaDebitosImovelRural) | `CertidaoNegativaDebitosImovelRural` | PF/PJ | `CIB`, `GERARCOMPROVANTE` | R$ 0,36 | Online | PDF receipt |
| [CNDM - Certidão Negativa de Débitos Municipal](https://directd.com.br/central-de-ajuda/apis/catalogo/CertidaoNegativaDebitosMunicipal) | `CertidaoNegativaDebitosMunicipal` | PF/PJ | `Cnpj`, `IM`, `Cpf`, `Municipio`, `GERARCOMPROVANTE` | R$ 0,36 | Online | PDF receipt |
| [NFe - Nota Fiscal Eletrônica - Completa](https://directd.com.br/central-de-ajuda/apis/catalogo/NotaFiscalEletronicaCompleta) | `NotaFiscalEletronicaCompleta` | PF/PJ | `Chave`, `GERARCOMPROVANTE` | R$ 1,30 | Online | PDF receipt |
| [NFe - Nota Fiscal Eletrônica - Inutilizações](https://directd.com.br/central-de-ajuda/apis/catalogo/NotaFiscalEletronicaInutilizacao) | `NotaFiscalEletronicaInutilizacao` | PJ | `CNPJ`, `Ano`, `GERARCOMPROVANTE` | R$ 1,30 | Online | PDF receipt |
| [Receita Federal - Pessoa Física](https://directd.com.br/central-de-ajuda/apis/catalogo/ReceitaFederalPessoaFisica) | `ReceitaFederalPessoaFisica` | PF | `Cpf`, `DataNascimento`, `GERARCOMPROVANTE` | R$ 0,36 | Online | PDF receipt |
| [Receita Federal - Pessoa Jurídica](https://directd.com.br/central-de-ajuda/apis/catalogo/ReceitaFederalPessoaJuridica) | `ReceitaFederalPessoaJuridica` | PJ | `Cnpj`, `Qsa`, `GERARCOMPROVANTE` | R$ 0,36 | Online | PDF receipt |
| [Receita Federal - Pessoa Jurídica - QSA + Participação Societária](https://directd.com.br/central-de-ajuda/apis/catalogo/ReceitaPJParticipacaoSocietaria) | `ReceitaPJParticipacaoSocietaria` | PJ | `CNPJ` | R$ 2,80 | Online |  |
| [Restituição Imposto de Renda - IRPF](https://directd.com.br/central-de-ajuda/apis/catalogo/RestituicaoIRPF) | `RestituicaoIRPF` | PF | `Cpf`, `DataNascimento`, `AnoExercicio` | R$ 0,36 | Online |  |
| [Simples Nacional](https://directd.com.br/central-de-ajuda/apis/catalogo/SimplesNacional) | `SimplesNacional` | PJ | `Cnpj`, `GERARCOMPROVANTE` | R$ 0,36 | Online | PDF receipt |
| [Sintegra - Cadastros Estaduais](https://directd.com.br/central-de-ajuda/apis/catalogo/Sintegra) | `Sintegra` | PF/PJ | `Cnpj`, `ie`, `Cpf`, `Uf`, `GERARCOMPROVANTE` | R$ 0,36 | Online | PDF receipt |
| [Sintegra - Cadastro Centralizado de Contribuinte - CCC](https://directd.com.br/central-de-ajuda/apis/catalogo/SintegraCCC) | `SintegraCCC` | PF/PJ | `Cnpj`, `ie`, `Cpf`, `Uf`, `GERARCOMPROVANTE` | R$ 0,36 | Online | PDF receipt |
| [Sintegra - CCC - Completa](https://directd.com.br/central-de-ajuda/apis/catalogo/SintegraCCC-Completa) | `SintegraCCC-Completa` | PF/PJ | `Cnpj`, `ie`, `Cpf`, `Uf`, `GERARCOMPROVANTE` | R$ 1,80 | Online | PDF receipt |

### Processos (7)

| API | Slug | Target | Parameters | Price | Origin | Notes |
| --- | --- | --- | --- | --- | --- | --- |
| [CARF - Conselho Administrativo de Recursos Fiscais](https://directd.com.br/central-de-ajuda/apis/catalogo/CARFConselhoAdmistrativodeRecursosFiscais) | `CARFConselhoAdmistrativodeRecursosFiscais` | PF/PJ | `CPF`, `CNPJ` | R$ 0,36 | Online | incomplete: the documentation page lists no parameter |
| [TCU - Certidão Negativa de Processo](https://directd.com.br/central-de-ajuda/apis/catalogo/CertidaoNegativaProcesso) | `CertidaoNegativaProcesso` | PF/PJ | `Cpf`, `Cnpj` | R$ 0,36 | Online | PDF receipt; incomplete: the documentation page lists no parameter |
| [Ministério do Trabalho e do Emprego (MTE) - Infrações Trabalhistas](https://directd.com.br/central-de-ajuda/apis/catalogo/MTEInfracoesTrabalhistas) | `MTEInfracoesTrabalhistas` | PF/PJ | `Cpf`, `Cnpj`, `TIPO`, `GERARCOMPROVANTE` | R$ 0,36 | Online | PDF receipt |
| [Processos Judiciais - Agrupada (Base)](https://directd.com.br/central-de-ajuda/apis/catalogo/ProcessosJudiciaisAgrupada) | `ProcessosJudiciaisAgrupada` | PF/PJ | `Cpf`, `Cnpj` | R$ 1,10 | Base |  |
| [Processos Judiciais - Completa (Base)](https://directd.com.br/central-de-ajuda/apis/catalogo/ProcessosJudiciaisCompleta) | `ProcessosJudiciaisCompleta` | PF/PJ | `Cpf`, `Cnpj` | R$ 3,30 | Base |  |
| [Processos Judiciais - Simplificada (Base)](https://directd.com.br/central-de-ajuda/apis/catalogo/ProcessosJudiciaisSimplificada) | `ProcessosJudiciaisSimplificada` | PF/PJ | `Cpf`, `Cnpj` | R$ 1,50 | Base |  |
| [TJ - Tribunal de Justiça - Processos](https://directd.com.br/central-de-ajuda/apis/catalogo/TribunalJustica) | `TribunalJustica` | PJ | `Cpf`, `Cnpj`, `NomeParte`, `NumeroProcesso`, `Uf`, `Grau` | R$ 0,36 | Online |  |

### Produtores Rurais (8)

| API | Slug | Target | Parameters | Price | Origin | Notes |
| --- | --- | --- | --- | --- | --- | --- |
| [APF Rural - Autorização Provisória de Funcionamento Rural](https://directd.com.br/central-de-ajuda/apis/catalogo/APFRural) | `APFRural` | PF/PJ | `CNPJ`, `CPF`, `NumeroAPF`, `NumeroCAR`, `GERARCOMPROVANTE` | R$ 0,36 | Online | PDF receipt |
| [CAF - Cadastro Nacional de Agricultura Familiar - PF](https://directd.com.br/central-de-ajuda/apis/catalogo/CAFCadastroNacionalAgriculturaPF) | `CAFCadastroNacionalAgriculturaPF` | PF | `CPF`, `GERARCOMPROVANTE` | R$ 0,36 | Online | PDF receipt |
| [CAF - Cadastro Nacional de Agricultura Familiar - PJ](https://directd.com.br/central-de-ajuda/apis/catalogo/CAFCadastroNacionalAgriculturaPJ) | `CAFCadastroNacionalAgriculturaPJ` | PJ | `CNPJ`, `GERARCOMPROVANTE` | R$ 0,36 | Online | PDF receipt |
| [CAR - Cadastro Ambiental Rural](https://directd.com.br/central-de-ajuda/apis/catalogo/CadastroAmbientalRural) | `CadastroAmbientalRural` | PF/PJ | `NumeroCAR`, `GERARCOMPROVANTE` | R$ 0,36 | Online | PDF receipt |
| [CAFIR - Cadastros de Imóveis Rurais](https://directd.com.br/central-de-ajuda/apis/catalogo/CadastroImoveisRurais) | `CadastroImoveisRurais` | PF/PJ | `CIB`, `GERARCOMPROVANTE` | R$ 0,36 | Online | PDF receipt |
| [DAP - Declaração de Aptidão ao Pronaf - Pessoa Física](https://directd.com.br/central-de-ajuda/apis/catalogo/DAPPessoaFisica) | `DAPPessoaFisica` | PF | `CPF`, `GERARCOMPROVANTE`, `DataNascimento` | R$ 0,72 | Online | PDF receipt |
| [DAP - Declaração de Aptidão ao Pronaf - Pessoa Jurídica](https://directd.com.br/central-de-ajuda/apis/catalogo/DAPPessoaJuridica) | `DAPPessoaJuridica` | PJ | `CNPJ`, `GERARCOMPROVANTE` | R$ 0,72 | Online | PDF receipt |
| [Garantia Safra](https://directd.com.br/central-de-ajuda/apis/catalogo/GarantiaSafra) | `GarantiaSafra` | PF | `CPF`, `NIS` | R$ 0,36 | Online |  |

### Regularidade (7)

| API | Slug | Target | Parameters | Price | Origin | Notes |
| --- | --- | --- | --- | --- | --- | --- |
| [Consultoria Geral da União - CGU](https://directd.com.br/central-de-ajuda/apis/catalogo/CGUConsultoriaGeralUniao) | `CGUConsultoriaGeralUniao` | PF/PJ | `Cpf`, `Cnpj`, `Tipo`, `GERARCOMPROVANTE` | R$ 0,36 | Online | PDF receipt |
| [FGTS - Regularidade do Empregador](https://directd.com.br/central-de-ajuda/apis/catalogo/CaixaRegularidadeEmpregadorFGTS) | `CaixaRegularidadeEmpregadorFGTS` | PJ | `Cnpj`, `CPF`, `GERARCOMPROVANTE` | R$ 0,36 | Online | PDF receipt |
| [STM - Certidão Negativa da União](https://directd.com.br/central-de-ajuda/apis/catalogo/STMCertidaoNegativauniao) | `STMCertidaoNegativauniao` | PF | `Nome`, `CPF`, `DataNascimento`, `NomeMae` | R$ 0,36 | Online | PDF receipt; incomplete: the documentation page lists no parameter |
| [TSE - Situação Eleitoral](https://directd.com.br/central-de-ajuda/apis/catalogo/SituacaoEleitoral) | `SituacaoEleitoral` | PF | `Nome`, `CPF`, `NumeroTituloEleitoral`, `DataNascimento` | R$ 0,36 | Online |  |
| [TSE - Certidão de Crimes Eleitorais](https://directd.com.br/central-de-ajuda/apis/catalogo/TSECertidaoCrimesEleitorais) | `TSECertidaoCrimesEleitorais` | PF | `Nome`, `CPF`, `NomeMae`, `NOMEPAI`, `NumeroTituloEleitoral`, `DataNascimento`, `GERARCOMPROVANTE` | R$ 0,36 | Online | PDF receipt |
| [TSE - Certidão de Quitação Eleitoral](https://directd.com.br/central-de-ajuda/apis/catalogo/TSECertidaodeQuitacaoEleitoral) | `TSECertidaodeQuitacaoEleitoral` | PF | `Nome`, `CPF`, `NOMEMAE`, `NOMEPAI`, `NUMEROTITULOELEITORAL`, `DATANASCIMENTO`, `GERARCOMPROVANTE` | R$ 0,36 | Online | PDF receipt |
| [TSE - Título e Local de Votação](https://directd.com.br/central-de-ajuda/apis/catalogo/TituloLocalVotacao) | `TituloLocalVotacao` | PF | `Nome`, `CPF`, `NumeroTituloEleitoral`, `DataNascimento`, `NomeMae` | R$ 0,72 | Online |  |

### Sanções e Restrições (22)

| API | Slug | Target | Parameters | Price | Origin | Notes |
| --- | --- | --- | --- | --- | --- | --- |
| [Acordos de Leniência](https://directd.com.br/central-de-ajuda/apis/catalogo/AcordosLeniencia) | `AcordosLeniencia` | PJ | `Cnpj` | R$ 0,36 | Online |  |
| [Banco Central - Quadro Geral de Inabilitados](https://directd.com.br/central-de-ajuda/apis/catalogo/BancoCentralInabilitados) | `BancoCentralInabilitados` | PF | `Cpf` | R$ 0,36 | Online |  |
| [Banco Central - Quadro Geral de Proibidos](https://directd.com.br/central-de-ajuda/apis/catalogo/BancoCentralProibidos) | `BancoCentralProibidos` | PF/PJ | `Cnpj`, `Cpf` | R$ 0,36 | Online |  |
| [CNJ - Mandados de Prisão](https://directd.com.br/central-de-ajuda/apis/catalogo/CNJMandadosPrisao) | `CNJMandadosPrisao` | PF | `Cpf`, `Nome`, `NomeMae`, `NomePai`, `Alcunha`, `GERARCOMPROVANTE` | R$ 0,72 | Online | PDF receipt |
| [CEIS - Cadastro de Empresas Inidôneas e Suspensas](https://directd.com.br/central-de-ajuda/apis/catalogo/CadastroEmpresasInidoneasSuspensas) | `CadastroEmpresasInidoneasSuspensas` | PF/PJ | `Cnpj`, `Cpf` | R$ 0,36 | Online |  |
| [CEPIM - Cadastro de Entidades Privadas Sem Fins Lucrativos Impedidas](https://directd.com.br/central-de-ajuda/apis/catalogo/CadastroEntidadesPrivadasImpedidas) | `CadastroEntidadesPrivadasImpedidas` | PJ | `Cnpj` | R$ 0,36 | Online |  |
| [CEAF - Cadastro de Expulsões da Administração Federal](https://directd.com.br/central-de-ajuda/apis/catalogo/CadastroExpulsoesAdministracaoFederal) | `CadastroExpulsoesAdministracaoFederal` | PF | `Cpf` | R$ 0,36 | Online |  |
| [CNEP - Cadastro Nacional de Empresas Punidas](https://directd.com.br/central-de-ajuda/apis/catalogo/CadastroNacionalEmpresasPunidas) | `CadastroNacionalEmpresasPunidas` | PF/PJ | `Cnpj`, `Cpf` | R$ 0,36 | Online |  |
| [CNIA - Cadastro Nacional de Condenações Cíveis por Ato de Improbidade Administrativa e Inelegibilidade](https://directd.com.br/central-de-ajuda/apis/catalogo/CadastroNacionalImprobidadeAdministrativa) | `CadastroNacionalImprobidadeAdministrativa` | PF/PJ | `Cnpj`, `Cpf` | R$ 0,36 | Online | PDF receipt |
| [Ministério Público Federal - Certidão Negativa](https://directd.com.br/central-de-ajuda/apis/catalogo/MPFCertidaoNegativa) | `MPFCertidaoNegativa` | PF/PJ | `Cpf`, `Cnpj`, `GERARCOMPROVANTE` | R$ 0,36 | Online | PDF receipt |
| [Ministério Público do Estado de Mato Grosso (MPMT) - Procedimentos Investigatórios Extrajudiciais](https://directd.com.br/central-de-ajuda/apis/catalogo/MPMTMinisterioPublicoMatoGrosso) | `MPMTMinisterioPublicoMatoGrosso` | PF/PJ | `Cpf`, `Cnpj`, `GERARCOMPROVANTE` | R$ 0,36 | Online | PDF receipt |
| [PEP Estendida - Pessoa Exposta Politicamente + Parentescos](https://directd.com.br/central-de-ajuda/apis/catalogo/PEPParentescos) | `PEPParentescos` | PF | `Cpf` | R$ 0,36 | Base |  |
| [PETI -  Programa de Erradicação do Trabalho Infantil](https://directd.com.br/central-de-ajuda/apis/catalogo/PETITrabalhoInfantil) | `PETITrabalhoInfantil` | PF | `CPF`, `NIS` | R$ 0,36 | Online |  |
| [PGFN - Lista de Devedores da União](https://directd.com.br/central-de-ajuda/apis/catalogo/PGFNListaDevedoresUniao) | `PGFNListaDevedoresUniao` | PF/PJ | `Cpf`, `Cnpj`, `GERARCOMPROVANTE` | R$ 0,36 | Online | PDF receipt |
| [PEP - Pessoa Exposta Politicamente](https://directd.com.br/central-de-ajuda/apis/catalogo/PessoaExpostaPoliticamente) | `PessoaExpostaPoliticamente` | PF | `Cpf` | R$ 0,36 | Base |  |
| [Polícia Civil - Antecedentes Criminais](https://directd.com.br/central-de-ajuda/apis/catalogo/PoliciaCivilAntecedentesCriminais) | `PoliciaCivilAntecedentesCriminais` | PF | `CPF`, `Rg`, `NomeMae`, `Nome`, `DataNascimento`, `GENERO`, `Uf`, `GERARCOMPROVANTE` | R$ 0,36 | Online | PDF receipt |
| [Polícia Federal - Antecedentes Criminais](https://directd.com.br/central-de-ajuda/apis/catalogo/PoliciaFederalAntecedentesCriminais) | `PoliciaFederalAntecedentesCriminais` | PF | `Cpf`, `Nome`, `DataNascimento`, `NomeMae`, `NomePai`, `GERARCOMPROVANTE` | R$ 0,36 | Online | PDF receipt |
| [TCU - Certidão Negativa de Licitante Inidôneo](https://directd.com.br/central-de-ajuda/apis/catalogo/TCUCertidaoNegativaLicitanteInidoneo) | `TCUCertidaoNegativaLicitanteInidoneo` | PF/PJ | `Cpf`, `Cnpj`, `GERARCOMPROVANTE` | R$ 0,36 | Online | PDF receipt |
| [TCU - Consulta Consolidada de Pessoa Jurídica - APF](https://directd.com.br/central-de-ajuda/apis/catalogo/TCUConsultaConsolidadaPessoaJuridica) | `TCUConsultaConsolidadaPessoaJuridica` | PJ | `Cnpj`, `GERARCOMPROVANTE` | R$ 0,72 | Online | PDF receipt |
| [TJ - Certidão Cível, Criminal e Fiscal - Tribunal de Justiça](https://directd.com.br/central-de-ajuda/apis/catalogo/TJCertidaoCivelCriminalFiscal) | `TJCertidaoCivelCriminalFiscal` | PF/PJ | `CPF`, `CNPJ`, `Nome`, `DataNascimento`, `NomeMae`, `NomePai`, `RG`, `Genero`, `UF`, `Tipo`, `GERARCOMPROVANTE` | R$ 0,36 | Online | PDF receipt |
| [TRF - Tribunal Regional Federal (Certidão Cível, Eleitoral ou Criminal)](https://directd.com.br/central-de-ajuda/apis/catalogo/TribunalRegionalFederal) | `TribunalRegionalFederal` | PF/PJ | `Cnpj`, `Cpf`, `Regiao`, `Tipo`, `GERARCOMPROVANTE` | R$ 0,36 | Online | PDF receipt |
| [Verificação de Empregador - Trabalho Forçado](https://directd.com.br/central-de-ajuda/apis/catalogo/VerificacaoEmpregadorTrabalhoForcado) | `VerificacaoEmpregadorTrabalhoForcado` | PF/PJ | `Cnpj`, `Cpf` | R$ 0,36 | Online |  |

### Sanções e Restrições (Internacional) (11)

| API | Slug | Target | Parameters | Price | Origin | Notes |
| --- | --- | --- | --- | --- | --- | --- |
| [Austrália DFAT - Sanções Internacionais](https://directd.com.br/central-de-ajuda/apis/catalogo/AustraliaDFAT) | `AustraliaDFAT` | PF/PJ | `Nome` | R$ 0,36 | Base |  |
| [Canada SEMA - Sanções Internacionais](https://directd.com.br/central-de-ajuda/apis/catalogo/CanadaSEMA) | `CanadaSEMA` | PF/PJ | `Nome` | R$ 0,36 | Base |  |
| [EU Financial Sanctions List](https://directd.com.br/central-de-ajuda/apis/catalogo/EUFinancialList) | `EUFinancialList` | PF/PJ | `Nome` | R$ 0,36 | Online |  |
| [FBI - Most Wanted](https://directd.com.br/central-de-ajuda/apis/catalogo/FBIMostWanted) | `FBIMostWanted` | PF | `Nome`, `Sobrenome` | R$ 0,36 | Online |  |
| [FINCEN - Financial Crimes Enforcement Network](https://directd.com.br/central-de-ajuda/apis/catalogo/FinCEN) | `FinCEN` | PF/PJ | `Nome` | R$ 0,36 | Online | PDF receipt |
| [INTERPOL](https://directd.com.br/central-de-ajuda/apis/catalogo/Interpol) | `Interpol` | PF | `Nome`, `Sobrenome`, `DataNascimento` | R$ 0,36 | Online |  |
| [OFAC - Sanctions List (SND and Non-SDN)](https://directd.com.br/central-de-ajuda/apis/catalogo/OFAC) | `OFAC` | PF/PJ | `Nome`, `Sobrenome` | R$ 0,36 | Online |  |
| [Offshore Leaks - ICIJ](https://directd.com.br/central-de-ajuda/apis/catalogo/OffshoreLeaks) | `OffshoreLeaks` | PF/PJ | `Nome`, `NodeID` | R$ 0,36 | Base |  |
| [Suíça SECO - Sanções Internacionais](https://directd.com.br/central-de-ajuda/apis/catalogo/SuicaSECO) | `SuicaSECO` | PF/PJ | `Nome` | R$ 0,36 | Base |  |
| [UK Hm Treasury - Financial Sanctions](https://directd.com.br/central-de-ajuda/apis/catalogo/UKHmTreasury) | `UKHmTreasury` | PF/PJ | `Nome` | R$ 0,36 | Online |  |
| [UNSCCL - United Nations Security Council Consolidated List](https://directd.com.br/central-de-ajuda/apis/catalogo/UnitedNationsSecurityList) | `UnitedNationsSecurityList` | PF/PJ | `Nome`, `Sobrenome` | R$ 0,36 | Online |  |

### Societário (3)

| API | Slug | Target | Parameters | Price | Origin | Notes |
| --- | --- | --- | --- | --- | --- | --- |
| [AML - Anti Money Laundering (Vínculos Societários)](https://directd.com.br/central-de-ajuda/apis/catalogo/AML) | `AML` | PF | `Cpf` | R$ 0,72 | Base |  |
| [Beneficiário Final](https://directd.com.br/central-de-ajuda/apis/catalogo/BeneficiarioFinal) | `BeneficiarioFinal` | PF/PJ | `Cnpj`, `Cpf` | R$ 1,44 | Base |  |
| [Vínculos Societários](https://directd.com.br/central-de-ajuda/apis/catalogo/VinculosSocietarios) | `VinculosSocietarios` | PF/PJ | `CNPJ`, `CPF` | R$ 1,84 | Base |  |

### Socio-Demográficos (2)

| API | Slug | Target | Parameters | Price | Origin | Notes |
| --- | --- | --- | --- | --- | --- | --- |
| [Certificado Anbima EDU](https://directd.com.br/central-de-ajuda/apis/catalogo/AnbimaCertificadoEDU) | `AnbimaCertificadoEDU` | PF | `CPF` | R$ 0,36 | Online |  |
| [Bet Safe Compliance](https://directd.com.br/central-de-ajuda/apis/catalogo/BetSafeCompliance) | `BetSafeCompliance` | PF | `CPF` | R$ 0,72 | Base |  |

### Trabalhista (3)

| API | Slug | Target | Parameters | Price | Origin | Notes |
| --- | --- | --- | --- | --- | --- | --- |
| [MPT - Ministério Público do Trabalho](https://directd.com.br/central-de-ajuda/apis/catalogo/MinisterioPublicoTrabalho) | `MinisterioPublicoTrabalho` | PF/PJ | `Cnpj`, `Cpf`, `Regiao`, `GERARCOMPROVANTE` | R$ 1,30 | Online | PDF receipt |
| [TST - Certidão Negativa de Débitos Trabalhistas](https://directd.com.br/central-de-ajuda/apis/catalogo/TSTCertidaoNegativaDebitosTrabalhistas) | `TSTCertidaoNegativaDebitosTrabalhistas` | PF/PJ | `Cpf`, `Cnpj`, `GERARCOMPROVANTE` | R$ 0,36 | Online | PDF receipt |
| [TRT - Tribunal Regional do Trabalho](https://directd.com.br/central-de-ajuda/apis/catalogo/TribunalRegionalTrabalho) | `TribunalRegionalTrabalho` | PF/PJ | `Cnpj`, `Cpf`, `Regiao`, `TIPO`, `GERARCOMPROVANTE` | R$ 0,36 | Online | PDF receipt |

### Veículos (19)

| API | Slug | Target | Parameters | Price | Origin | Notes |
| --- | --- | --- | --- | --- | --- | --- |
| [ANTT - Código Identificador Operação Transporte](https://directd.com.br/central-de-ajuda/apis/catalogo/ANTTConsultaCodigoIdentificadorOperacaoTransporte) | `ANTTConsultaCodigoIdentificadorOperacaoTransporte` | PF/PJ | `RNTRC`, `RENAVAM` | R$ 0,36 | Online |  |
| [ANTT - Consulta de Regularidade da Transportadora](https://directd.com.br/central-de-ajuda/apis/catalogo/ANTTConsultaRegularidadeTransportadora) | `ANTTConsultaRegularidadeTransportadora` | PF/PJ | `CPF`, `CNPJ`, `RNTRC`, `GERARCOMPROVANTE` | R$ 0,36 | Online | PDF receipt |
| [Consulta Veicular - Leilão Veicular](https://directd.com.br/central-de-ajuda/apis/catalogo/ConsultaLeilaoVeicular) | `ConsultaLeilaoVeicular` | PF/PJ | `Placa`, `Chassi` | R$ 12,00 | Online |  |
| [Consulta Veicular Serpro - Multas Radar](https://directd.com.br/central-de-ajuda/apis/catalogo/ConsultaSerproMultasRadar) | `ConsultaSerproMultasRadar` | PF/PJ | `Placa`, `Renavam` | R$ 0,36 | Online | PDF receipt; incomplete: the documentation page lists no parameter |
| [Consulta Veicular - Nacional](https://directd.com.br/central-de-ajuda/apis/catalogo/ConsultaVeicular) | `ConsultaVeicular` | PF/PJ | `Placa` | R$ 1,10 | Online |  |
| [Consulta Veicular - CRLV-e SP](https://directd.com.br/central-de-ajuda/apis/catalogo/ConsultaVeicularCrlvSP) | `ConsultaVeicularCrlvSP` | PF/PJ | `Placa`, `GERARCOMPROVANTE` | R$ 12,00 | Online | PDF receipt |
| [Consulta Veicular - Estadual](https://directd.com.br/central-de-ajuda/apis/catalogo/ConsultaVeicularEstadual) | `ConsultaVeicularEstadual` | PF/PJ | `Placa`, `Chassi` | R$ 1,90 | Online |  |
| [Veículos - FIPE](https://directd.com.br/central-de-ajuda/apis/catalogo/ConsultaVeicularFipe) | `ConsultaVeicularFipe` | PF/PJ | `Placa`, `Chassi` | R$ 3,89 | Online |  |
| [Consulta Veicular - Frotas Nacional](https://directd.com.br/central-de-ajuda/apis/catalogo/ConsultaVeicularFrotasNacional) | `ConsultaVeicularFrotasNacional` | PF/PJ | `Cpf`, `Cnpj` | R$ 4,55 | Online |  |
| [Consulta Veicular - Gravame](https://directd.com.br/central-de-ajuda/apis/catalogo/ConsultaVeicularGravame) | `ConsultaVeicularGravame` | PF/PJ | `Placa`, `Chassi` | R$ 1,80 | Online |  |
| [Consulta Veicular - Histórico de Proprietários](https://directd.com.br/central-de-ajuda/apis/catalogo/ConsultaVeicularHistoricoProprietarios) | `ConsultaVeicularHistoricoProprietarios` | PF/PJ | `Placa` | R$ 4,10 | Base |  |
| [Consulta Veicular - Proprietário Atual](https://directd.com.br/central-de-ajuda/apis/catalogo/ConsultaVeicularProprietarioAtual) | `ConsultaVeicularProprietarioAtual` | PF/PJ | `Placa` | R$ 0,90 | Online |  |
| [Consulta Veicular - Recall](https://directd.com.br/central-de-ajuda/apis/catalogo/ConsultaVeicularRecall) | `ConsultaVeicularRecall` | PF/PJ | `Placa` | R$ 1,20 | Online |  |
| [Consulta Veicular - Renainf](https://directd.com.br/central-de-ajuda/apis/catalogo/ConsultaVeicularRenainf) | `ConsultaVeicularRenainf` | PF/PJ | `Placa` | R$ 2,10 | Base |  |
| [Consulta Veicular - Renajud](https://directd.com.br/central-de-ajuda/apis/catalogo/ConsultaVeicularRenajud) | `ConsultaVeicularRenajud` | PF/PJ | `Placa` | R$ 1,90 | Base |  |
| [Consulta Veicular - Roubo e Furto](https://directd.com.br/central-de-ajuda/apis/catalogo/ConsultaVeicularRouboFurto) | `ConsultaVeicularRouboFurto` | PF/PJ | `Placa`, `Chassi` | R$ 1,45 | Online |  |
| [Histórico Veicular PF e PJ - São Paulo](https://directd.com.br/central-de-ajuda/apis/catalogo/HistoricoVeiculos) | `HistoricoVeiculos` | PF/PJ | `Cpf`, `Cnpj` | R$ 0,36 | Base |  |
| [Histórico Veicular PF e PJ - Nacional](https://directd.com.br/central-de-ajuda/apis/catalogo/HistoricoVeiculosNacional) | `HistoricoVeiculosNacional` | PF/PJ | `Cpf`, `Cnpj` | R$ 1,80 | Base |  |
| [Polícia Rodoviária Federal - Infrações](https://directd.com.br/central-de-ajuda/apis/catalogo/PRFInfracoes) | `PRFInfracoes` | PF/PJ | `Placa`, `Renavam`, `TIPO` | R$ 0,36 | Online |  |

<!-- catalog:end -->
