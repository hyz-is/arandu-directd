package directd

import (
	"net/http"
	"strings"
)

// Outcome is what a query came to, as far as the transport and Direct Data's
// own status say. It describes the query, never the person or company
// queried: "found" says the source returned a record, not that the record is
// regular, alive or trustworthy.
type Outcome string

// The outcomes.
const (
	// OutcomeFound is a query Direct Data answered with status "Sucesso".
	OutcomeFound Outcome = "found"
	// OutcomeFoundWithCaveats is a query answered with status "Sucesso com
	// Ressalvas": data came back, and part of the query did not complete.
	OutcomeFoundWithCaveats Outcome = "found_with_caveats"
	// OutcomeNotFound is a query for which the source holds no record.
	OutcomeNotFound Outcome = "not_found"
	// OutcomeInvalidDocument is a parameter Direct Data refused (HTTP 400). A
	// document that fails its check digits never reaches Direct Data: the
	// client refuses it with ErrInvalidDocument instead.
	OutcomeInvalidDocument Outcome = "invalid_document"
	// OutcomeUnavailable is a transport failure, a 5xx, a source that did not
	// answer, or the circuit breaker refusing the call.
	OutcomeUnavailable Outcome = "unavailable"
	// OutcomeInsufficientBalance is a query refused for lack of balance.
	OutcomeInsufficientBalance Outcome = "insufficient_balance"
	// OutcomeUnauthorized is a token Direct Data refused (HTTP 401 or 403).
	OutcomeUnauthorized Outcome = "unauthorized"
	// OutcomeRateLimited is a query refused for its rate (HTTP 429).
	OutcomeRateLimited Outcome = "rate_limited"
	// OutcomeError is every other answer. The message is kept on the result.
	OutcomeError Outcome = "error"
)

// Billable reports whether Direct Data bills a query that came to o. Terms
// clause 2.5: "As consultas serão cobradas quando o status de retorno seja
// 'Sucesso' ou 'Sucesso com Ressalvas'." A duplicate query -- the same
// document on the same API on the same day -- is billed again under the same
// clause.
func (o Outcome) Billable() bool { return o == OutcomeFound || o == OutcomeFoundWithCaveats }

// Classify maps an answer to its Outcome.
//
// The order is: a resultadoId the application mapped in Config.ResultCodes;
// the HTTP status, as the public documentation lists it (400 invalid
// parameter, 401 invalid token, 404 not found, 5xx internal error) plus 402,
// 403 and 429; the status text, where "Sucesso" and "Sucesso com Ressalvas"
// are named by the terms of use; and, last, words in the status and message
// that the documentation does not define but that say what happened
// ("não encontrado", "saldo insuficiente", "token inválido",
// "indisponível"). Anything else is OutcomeError.
//
// meta is nil when the answer carried no metaDados object. The public
// documentation lists no resultadoId values, so none is mapped by
// default; the application fills Config.ResultCodes from what it observes.
func Classify(httpStatus int, meta *MetaData, codes map[int]Outcome) Outcome {
	if meta != nil {
		if o, ok := codes[meta.ResultadoID]; ok {
			return o
		}
	} else {
		meta = &MetaData{}
	}
	switch {
	case httpStatus == http.StatusUnauthorized, httpStatus == http.StatusForbidden:
		return OutcomeUnauthorized
	case httpStatus == http.StatusPaymentRequired:
		return OutcomeInsufficientBalance
	case httpStatus == http.StatusTooManyRequests:
		return OutcomeRateLimited
	case httpStatus == http.StatusNotFound:
		return OutcomeNotFound
	case httpStatus == http.StatusBadRequest, httpStatus == http.StatusUnprocessableEntity:
		return OutcomeInvalidDocument
	case httpStatus >= 500:
		return OutcomeUnavailable
	case httpStatus < 200 || httpStatus > 299:
		return OutcomeError
	}

	status := fold(meta.Resultado)
	switch status {
	case "sucesso":
		return OutcomeFound
	case "sucesso com ressalvas", "sucesso com ressalva":
		return OutcomeFoundWithCaveats
	}
	text := status + " " + fold(meta.Mensagem)
	for _, rule := range wordRules {
		for _, w := range rule.words {
			if strings.Contains(text, w) {
				return rule.outcome
			}
		}
	}
	return OutcomeError
}

// wordRules are the undocumented words Classify reads, in order.
var wordRules = []struct {
	outcome Outcome
	words   []string
}{
	{OutcomeInsufficientBalance, []string{"saldo insuficiente", "sem saldo", "creditos insuficientes", "credito insuficiente"}},
	{OutcomeUnauthorized, []string{"token invalido", "token nao", "nao autorizado", "acesso negado", "unauthorized"}},
	{OutcomeRateLimited, []string{"limite de requisic", "too many requests"}},
	{OutcomeNotFound, []string{"nao encontrad", "nenhum registro", "nenhum resultado", "sem registro", "inexistente"}},
	{OutcomeInvalidDocument, []string{"invalido", "invalida"}},
	{OutcomeUnavailable, []string{"indisponivel", "timeout", "tempo esgotado", "fora do ar", "instabilidade"}},
}

// accents folds the accented letters Portuguese uses.
var accents = strings.NewReplacer(
	"á", "a", "à", "a", "â", "a", "ã", "a", "ä", "a",
	"é", "e", "ê", "e", "è", "e", "ë", "e",
	"í", "i", "î", "i", "ì", "i", "ï", "i",
	"ó", "o", "ô", "o", "õ", "o", "ò", "o", "ö", "o",
	"ú", "u", "û", "u", "ù", "u", "ü", "u",
	"ç", "c",
)

// fold lower-cases a text and removes its accents.
func fold(s string) string { return accents.Replace(strings.ToLower(strings.TrimSpace(s))) }
