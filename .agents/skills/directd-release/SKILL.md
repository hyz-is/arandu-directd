---
name: directd-release
description: Get this Arandu package green, declared and published. Use when the request is to "run the gates", "check it builds", "the tests fail", "go build says the directory prefix does not contain modules listed in go.work", "update arandu.mod.toml", "add a dependency", "bump the framework", "tag a release", "publish it", "write the changelog", or before opening a pull request. Covers the four gates, the generated files, what the manifest declares and who checks it, and the immutability of a published module version.
license: MIT
---

# Getting it green and out

## The four gates

```sh
export GOWORK=off
test -z "$(gofmt -l $(find . -name '*.go' -not -path '*/testdata/*'))"
go build ./...
go vet ./...
go test -race -count=1 ./...
```

`GOWORK=off` because this checkout may sit beside a workspace that does not
list it. No test reaches the network: every request goes to an `httptest`
double. `aru doctor` is not a gate: it reads applications, and this is a
library.

## What the manifest declares, and who reads it

```toml
name = "hyz-is/directd"
framework = ">= 0.57"
profiles = ["conventional", "performance"]

[permissions]
network = true
filesystem = false
exec = false
migrations = false
```

`tests/Unit/audit_test.go` compares it with what the code calls. `network` is
the API itself. The commands (`cmd/directd-catalog`, `cmd/directd-smoke`) and
the internal packages only they import read and write files; no application
can link them, and `TestTheCommandsAreWhereTheFilesAre` holds that the root
package imports none of them. The `framework` line is a floor, and `go.mod`
says it too. Move both together.

## Dependencies

The framework (for `foundation.Module`) and `arandu-br` (CPF and CNPJ), at
tags, never through a local `replace`. A new `require` needs an argument in
the pull request.

## Publishing

A published Go module version is **immutable**. Before tagging: the gates
pass, `go generate ./...` changes nothing, `arandu.mod.toml` matches the code,
`CHANGELOG.md` has the entry under the version and `UPGRADE.md` names every
incompatible change. A new catalogue changes prices, products, types and
methods for every installer: it is a changelog line, and a removed API or
field is an incompatible change.
