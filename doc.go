// Package directd is a Go client for the Direct Data API
// (https://directd.com.br), the Brazilian data bureau: registry data of CPFs
// and CNPJs, sanctions and restriction lists, court records, tax status,
// vehicles and more -- every one of the APIs in its public catalogue.
//
// It returns evidence, never a decision. A Result says which API was asked
// about which (masked) document, when, at what price, what Direct Data
// answered (its consultaUid, status and message), the Outcome of the query
// (found, not_found, unavailable, ...), the raw retorno and, for every API,
// the retorno decoded into a type generated from the documentation. Whether a
// record supports a case is the application's policy.
//
// Every query costs money, so every call takes a Meter: the Ledger the
// application implements (normally over its wallet, bound to the tenant of the
// Grant the query is made under) holds the price before the request and
// settles or releases it after, and an optional Dedupe answers a same-day
// repeat -- which Direct Data bills again -- without a request. A document is
// checked (CPF and alphanumeric CNPJ check digits, with arandu-br) before
// anything is reserved or sent, and a circuit breaker stops calling a source
// that keeps failing.
//
// An application builds one Client in bootstrap/app.go with New and passes it
// to the services that query. There is no service provider, no container and
// no discovery. The package reads no environment variable and no file.
//
// The products, the response types and the per-API methods are generated
// from catalog/catalog.json, which cmd/directd-catalog reads from the public
// documentation; go generate rewrites them.
package directd

import "encoding/json"

//go:generate go run ./internal/gen/run

// decodeAs decodes a retorno into *T.
func decodeAs[T any](raw json.RawMessage) (any, error) {
	var v T
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, err
	}
	return &v, nil
}
