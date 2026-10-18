package feature_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"testing"

	directd "github.com/hyz-is/arandu-directd"
)

func asyncAnswer(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("ASync") != "habilitar" {
		envelope(w, 200, "Sucesso", "", "uid-sync", map[string]any{"nome": "X"})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"metaDados": map[string]any{"consultaUid": "uid-async", "resultado": "Processando", "assincrono": true, "enviarCallback": true},
		"retorno":   nil,
	})
}

// TestAsyncKeepsTheHoldUntilTheCallback walks one asynchronous query: the
// submission, a refused unverified callback, a mismatched one, the verified
// one, and the hold settled once.
func TestAsyncKeepsTheHoldUntilTheCallback(t *testing.T) {
	t.Parallel()
	d := newDouble(t, asyncAnswer)
	verifier := directd.SecretQueryVerifier{Param: "k", Secret: "callback-secret"}
	c := d.client(t, directd.Config{CallbackVerifier: verifier})
	m, ledger := meter(1000)
	ctx := t.Context()

	ticket, err := c.QueryAsync(ctx, m, directd.Request{Product: "CadastroPessoaFisicaPlus", Document: syntheticCPF}, directd.AsyncOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if ticket.HoldID == "" || ticket.ConsultaUID != "uid-async" || ticket.Result != nil || ledger.HeldCents() != 36 || ledger.SpentCents() != 0 {
		t.Fatalf("ticket %+v held %d", ticket, ledger.HeldCents())
	}
	if d.last().Query.Get("ASync") != "habilitar" {
		t.Error("ASync=habilitar was not sent")
	}

	body, _ := json.Marshal(map[string]any{
		"metaDados": map[string]any{"consultaUid": "uid-async", "resultado": "Sucesso", "chave": syntheticCPF},
		"retorno":   map[string]any{"nome": "FULANA SINTETICA", "situacaoCadastral": "REGULAR"},
	})
	if _, err := c.ParseCallback(directd.CallbackRequest{Body: body, Query: url.Values{"k": {"wrong"}}}); !errors.Is(err, directd.ErrCallbackUnverified) {
		t.Errorf("a wrong secret: %v", err)
	}
	unverified := d.client(t, directd.Config{})
	if _, err := unverified.ParseCallback(directd.CallbackRequest{Body: body, Query: url.Values{"k": {"callback-secret"}}}); !errors.Is(err, directd.ErrCallbackUnverified) {
		t.Errorf("no verifier configured: %v", err)
	}

	res, err := c.ParseCallback(directd.CallbackRequest{Body: body, Query: url.Values{"k": {"callback-secret"}}})
	if err != nil || !res.Async || res.Outcome != directd.OutcomeFound || res.Document != "***.***.***-09" {
		t.Fatalf("callback: %v %+v", err, res)
	}
	other := *res
	other.ConsultaUID = "uid-other"
	if err := c.CompleteAsync(ctx, ledger, ticket, &other); !errors.Is(err, directd.ErrCallbackMismatch) {
		t.Errorf("a mismatched callback: %v", err)
	}
	if err := c.CompleteAsync(ctx, ledger, ticket, res); err != nil {
		t.Fatal(err)
	}
	pf, ok := directd.As[directd.CadastroPessoaFisicaPlusRetorno](res)
	if !ok || pf.SituacaoCadastral != "REGULAR" {
		t.Errorf("the callback's data did not decode with the ticket's product: %+v", res.Data)
	}
	if ledger.SpentCents() != 36 || ledger.HeldCents() != 0 || res.ChargedCents != 36 {
		t.Errorf("spent %d held %d charged %d", ledger.SpentCents(), ledger.HeldCents(), res.ChargedCents)
	}
	if err := c.CompleteAsync(ctx, ledger, ticket, res); !errors.Is(err, directd.ErrTicketClosed) {
		t.Errorf("a second completion: %v", err)
	}
}

// TestAsyncCallbackURLNeedsADocumentedParameter holds that a per-request
// callback URL is refused, before any request, until the application names
// the parameter that carries it.
func TestAsyncCallbackURLNeedsADocumentedParameter(t *testing.T) {
	t.Parallel()
	d := newDouble(t, asyncAnswer)
	c := d.client(t, directd.Config{})
	m, ledger := meter(1000)
	req := directd.Request{Product: "CadastroPessoaFisicaPlus", Document: syntheticCPF}
	if _, err := c.QueryAsync(t.Context(), m, req, directd.AsyncOptions{CallbackURL: "https://app.example/cb"}); !errors.Is(err, directd.ErrCallbackParamUnknown) {
		t.Errorf("%v", err)
	}
	if d.count() != 0 || ledger.HeldCents() != 0 {
		t.Fatalf("%d requests", d.count())
	}
	named := d.client(t, directd.Config{CallbackURLParam: "UrlCallback"})
	if _, err := named.QueryAsync(t.Context(), m, req, directd.AsyncOptions{CallbackURL: "https://app.example/cb"}); err != nil {
		t.Fatal(err)
	}
	if d.last().Query.Get("UrlCallback") != "https://app.example/cb" {
		t.Errorf("query %v", d.last().Query)
	}
}

// TestAsyncAnsweredAtOnceSettlesHere covers a submission Direct Data answers
// synchronously anyway.
func TestAsyncAnsweredAtOnceSettlesHere(t *testing.T) {
	t.Parallel()
	d := newDouble(t, func(w http.ResponseWriter, _ *http.Request) {
		envelope(w, 200, "Sucesso", "", "uid-now", map[string]any{"nome": "X"})
	})
	c := d.client(t, directd.Config{})
	m, ledger := meter(1000)
	ticket, err := c.QueryAsync(t.Context(), m, directd.Request{Product: "CadastroPessoaFisica", Document: syntheticCPF}, directd.AsyncOptions{})
	if err != nil || ticket.HoldID != "" || ticket.Result == nil || ticket.Result.Outcome != directd.OutcomeFound || ledger.SpentCents() != 16 {
		t.Fatalf("%v %+v spent %d", err, ticket, ledger.SpentCents())
	}
}
