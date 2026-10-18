# Security policy

## Supported versions

The latest minor release of Direct Data receives security fixes. Older minor
releases do not: a published Go module version is immutable, so a fix is a new
release rather than a moved tag.

## Reporting a vulnerability

Report it privately, through GitHub's advisory form:

<https://github.com/hyz-is/arandu-directd/security/advisories/new>

Do not open a public issue and do not describe the problem in a pull request.
A report that arrives in public is a report every reader of this repository can
act on before there is a release to upgrade to.

Include what you need to reproduce it: the version, the configuration, and
the call that triggers it, with synthetic documents -- never a real CPF, CNPJ,
token or Direct Data answer.

Expect an acknowledgement within a few days. If the report is confirmed, the
fix, the release and the advisory are published together, and you are credited
unless you ask not to be.

## What is in scope

Anything that spends money, leaks a credential or leaks personal data. In
particular:

- a way for the token to reach a log, an error, a redirect target or any host
  but the configured one;
- a request made without a Ledger hold, after the Ledger refused, for an
  invalid document, while the breaker is open, or on a dedupe hit;
- a hold settled twice, or settled for an outcome that is not billable;
- an asynchronous callback read without a verifier, or completed against a
  ticket whose consultaUid it does not carry;
- a personal value that survives `Redact`, or reaches a log line;
- the documentation crawler contacting the API or the portal host;
- an answer that crashes the client or costs memory out of proportion to
  its size.

## What is not

- A vulnerability in the Direct Data API itself. Report it to Direct Data.
- What an application decides from the evidence, or what it stores. This
  package returns results and stores nothing.
- A price, a field or a status that differs from the public documentation.
  That is a catalogue update, not a vulnerability.
