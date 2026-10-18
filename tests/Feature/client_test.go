package feature_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	directd "github.com/hyz-is/arandu-directd"
)

// TestEveryProductSendsItsDocumentedRequest drives every API of the
// catalogue through the double: the documented path, the token as the TOKEN
// query parameter, the document bare in the documented parameter, no ASync;
// and the catalogue's own response example, answered back, decodes into the
// generated type.
func TestEveryProductSendsItsDocumentedRequest(t *testing.T) {
	t.Parallel()
	cat := loadCatalog(t)
	examples := map[string]map[string]any{}
	for _, a := range cat.APIs {
		ex, _ := a.ResponseExample.(map[string]any)
		examples[a.Path] = ex
	}
	d := newDouble(t, func(w http.ResponseWriter, r *http.Request) {
		ex := examples[r.URL.Path]
		body := map[string]any{}
		for k, v := range ex {
			body[k] = v
		}
		meta := map[string]any{}
		if m, ok := ex["metaDados"].(map[string]any); ok {
			for k, v := range m {
				meta[k] = v
			}
		}
		meta["resultado"] = "Sucesso"
		meta["consultaUid"] = "uid" + r.URL.Path
		body["metaDados"] = meta
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	})
	c := d.client(t, directd.Config{})

	exercised := 0
	t.Cleanup(func() {
		if exercised < len(cat.APIs) {
			t.Errorf("%d of %d products exercised", exercised, len(cat.APIs))
		}
	})
	for slug, p := range c.Products() {
		if p.Status == directd.StatusInactive {
			continue
		}
		exercised++
		req := directd.Request{Product: slug, Params: map[string]string{}}
		var docParam, want string
		for _, q := range p.Params {
			switch {
			case q.Role == directd.RoleCPF && req.Document == "":
				req.Document, docParam, want = "123.456.789-09", q.Name, syntheticCPF
			case q.Role == directd.RoleCNPJ && req.Document == "" && !p.TakesCPF():
				req.Document, docParam, want = "11.222.333/0001-81", q.Name, syntheticCNPJ
			case q.Role == directd.RoleDocument && req.Document == "":
				req.Document, docParam, want = syntheticCPF, q.Name, syntheticCPF
			}
		}
		if req.Document == "" {
			for _, q := range p.Params {
				if q.Role == directd.RoleInput {
					req.Params[q.Name] = "SYNTHETIC"
					docParam, want = q.Name, "SYNTHETIC"
					break
				}
			}
		}
		if docParam == "" {
			t.Errorf("%s: no parameter to query by", slug)
			continue
		}
		m, ledger := meter(10000)
		res, err := c.Query(t.Context(), m, req)
		if err != nil {
			t.Errorf("%s: %v", slug, err)
			continue
		}
		got := d.last()
		if got.Path != p.Path {
			t.Errorf("%s: path %s, want %s", slug, got.Path, p.Path)
		}
		if got.Query.Get("TOKEN") != secretToken {
			t.Errorf("%s: the token is not the TOKEN query parameter", slug)
		}
		if got.Query.Get(docParam) != want {
			t.Errorf("%s: %s = %q, want %q", slug, docParam, got.Query.Get(docParam), want)
		}
		if got.Query.Has("ASync") {
			t.Errorf("%s: a synchronous query sent ASync", slug)
		}
		if res.Outcome != directd.OutcomeFound || res.ConsultaUID != "uid"+p.Path || res.ChargedCents != p.PriceCents || ledger.SpentCents() != p.PriceCents {
			t.Errorf("%s: outcome %s uid %q charged %d spent %d", slug, res.Outcome, res.ConsultaUID, res.ChargedCents, ledger.SpentCents())
		}
		if res.DataError != "" {
			t.Errorf("%s: the documented example does not decode: %s", slug, res.DataError)
		}
		if len(res.Retorno) > 2 && res.Data == nil {
			t.Errorf("%s: retorno came back and Data is nil", slug)
		}
	}
}

// TestTheTypedMethodsDecodeSyntheticAnswers checks the generated methods
// and types for the KYC products on hand-written synthetic answers.
func TestTheTypedMethodsDecodeSyntheticAnswers(t *testing.T) {
	t.Parallel()
	answers := map[string]any{
		"/api/CadastroPessoaFisicaPlus": map[string]any{
			"cpf": "123.456.789-09", "nome": "FULANA SINTETICA", "dataNascimento": "01/02/1990", "nomeMae": "MAE SINTETICA",
			"situacaoCadastral": "REGULAR", "obito": false, "idade": 36,
			"parentescos": []any{map[string]any{"grauParentesco": "MAE", "cpf": "987.654.321-00", "nome": "MAE SINTETICA", "obito": "false"}},
		},
		"/api/ReceitaFederalPessoaFisica": map[string]any{
			"numeroCPF": "123.456.789-09", "nomePessoaFisica": "FULANA SINTETICA", "situacaoCadastral": "REGULAR",
			"possuiObito": false, "anoObito": "", "codigoControleComprovante": "AAAA.BBBB",
		},
		"/api/CadastroPessoaJuridicaPlus": map[string]any{
			"cnpj": "11.222.333/0001-81", "razaoSocial": "EMPRESA SINTETICA LTDA", "situacaoCadastral": "ATIVA",
			"cnaeCodigo": "6201500", "naturezaJuridicaCodigo": 2062,
			"socios": []any{map[string]any{"documento": "***456789**", "nome": "SOCIO SINTETICO", "cargo": "ADMINISTRADOR"}},
		},
		"/api/Similarity": map[string]any{
			"name": map[string]any{"name": "FULANA SINTETICA", "description": "MATCH PARCIAL", "distance": 2},
			"dob":  map[string]any{"dob": "1990/02/01", "description": "MATCH EXATO", "distance": 0},
		},
		"/api/AML": map[string]any{
			"cpf": "123.456.789-09", "pep": false, "obito": false,
			"sociedades": []any{map[string]any{"cnpj": "11.222.333/0001-81", "baixada": false, "socios": []any{map[string]any{"qualificacaoSocio": "SOCIO"}}}},
		},
	}
	d := newDouble(t, func(w http.ResponseWriter, r *http.Request) {
		envelope(w, 200, "Sucesso", "", "uid-typed", answers[r.URL.Path])
	})
	c := d.client(t, directd.Config{})
	m, _ := meter(10000)
	ctx := t.Context()

	_, pf, err := c.CadastroPessoaFisicaPlus(ctx, m, directd.CadastroPessoaFisicaPlusParams{CPF: syntheticCPF})
	if err != nil || pf == nil {
		t.Fatalf("PF Plus: %v", err)
	}
	if pf.Nome != "FULANA SINTETICA" || pf.SituacaoCadastral != "REGULAR" || bool(pf.Obito) || len(pf.Parentescos) != 1 || pf.Parentescos[0].GrauParentesco != "MAE" {
		t.Errorf("PF Plus: %+v", pf)
	}
	if age, ok := pf.Idade.Int64(); !ok || age != 36 {
		t.Errorf("PF Plus idade: %v", pf.Idade)
	}

	_, rf, err := c.ReceitaFederalPessoaFisica(ctx, m, directd.ReceitaFederalPessoaFisicaParams{CPF: syntheticCPF, DataNascimento: "01/02/1990"})
	if err != nil || rf == nil || rf.SituacaoCadastral != "REGULAR" || bool(rf.PossuiObito) {
		t.Fatalf("Receita Federal PF: %v %+v", err, rf)
	}
	if q := d.last().Query; q.Get("DataNascimento") != "01/02/1990" || q.Get("Cpf") != syntheticCPF {
		t.Errorf("Receita Federal PF query: %v", q)
	}

	_, pj, err := c.CadastroPessoaJuridicaPlus(ctx, m, directd.CadastroPessoaJuridicaPlusParams{CNPJ: syntheticCNPJ})
	if err != nil || pj == nil || pj.RazaoSocial != "EMPRESA SINTETICA LTDA" || pj.CnaeCodigo != "6201500" || pj.NaturezaJuridicaCodigo != "2062" || len(pj.Socios) != 1 {
		t.Fatalf("PJ Plus: %v %+v", err, pj)
	}

	_, sim, err := c.Similarity(ctx, m, directd.SimilarityParams{CPF: syntheticCPF, Name: "FULANA", DOB: "1990/02/01"})
	if err != nil || sim == nil || sim.Name.Description != "MATCH PARCIAL" || sim.Name.Distance != "2" || sim.Dob.Description != "MATCH EXATO" {
		t.Fatalf("Similarity: %v %+v", err, sim)
	}

	_, aml, err := c.AML(ctx, m, directd.AMLParams{CPF: syntheticCPF})
	if err != nil || aml == nil || bool(aml.Pep) || len(aml.Sociedades) != 1 || aml.Sociedades[0].Socios[0].QualificacaoSocio != "SOCIO" {
		t.Fatalf("AML: %v %+v", err, aml)
	}

	if _, _, err := c.ProcessosJudiciaisSimplificada(ctx, m, directd.ProcessosJudiciaisSimplificadaParams{CPF: syntheticCPF, CNPJ: syntheticCNPJ}); !errors.Is(err, directd.ErrInvalidParams) {
		t.Errorf("two documents at once: %v", err)
	}
}

// TestTheOutcomeDecidesSettleOrRelease holds the Ledger contract: a billable
// answer is settled at the list price, anything else is released.
func TestTheOutcomeDecidesSettleOrRelease(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		answer  func(w http.ResponseWriter)
		outcome directd.Outcome
		spent   int64
	}{
		{"success", func(w http.ResponseWriter) { envelope(w, 200, "Sucesso", "", "u1", map[string]any{"nome": "X"}) }, directd.OutcomeFound, 36},
		{"caveats", func(w http.ResponseWriter) { envelope(w, 200, "Sucesso com Ressalvas", "", "u2", map[string]any{}) }, directd.OutcomeFoundWithCaveats, 36},
		{"not found", func(w http.ResponseWriter) { envelope(w, 404, "Falha", "Registro não encontrado", "u3", nil) }, directd.OutcomeNotFound, 0},
		{"not found in 200", func(w http.ResponseWriter) { envelope(w, 200, "Falha", "Nenhum registro encontrado", "u4", nil) }, directd.OutcomeNotFound, 0},
		{"server error", func(w http.ResponseWriter) { w.WriteHeader(500) }, directd.OutcomeUnavailable, 0},
		{"token refused", func(w http.ResponseWriter) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"httpStatusCode":401,"message":"Token inválido","errors":["token"]}`))
		}, directd.OutcomeUnauthorized, 0},
		{"no balance", func(w http.ResponseWriter) { envelope(w, 200, "Erro", "Saldo insuficiente", "u5", nil) }, directd.OutcomeInsufficientBalance, 0},
		{"redirect", func(w http.ResponseWriter) {
			w.Header().Set("Location", "https://elsewhere.invalid/")
			w.WriteHeader(302)
		}, directd.OutcomeError, 0},
	}
	for _, cs := range cases {
		d := newDouble(t, func(w http.ResponseWriter, _ *http.Request) { cs.answer(w) })
		c := d.client(t, directd.Config{})
		m, ledger := meter(1000)
		res, err := c.Query(t.Context(), m, directd.Request{Product: "CadastroPessoaFisicaPlus", Document: syntheticCPF})
		if err != nil {
			t.Errorf("%s: %v", cs.name, err)
			continue
		}
		if res.Outcome != cs.outcome || ledger.SpentCents() != cs.spent || ledger.HeldCents() != 0 || res.ChargedCents != cs.spent {
			t.Errorf("%s: outcome %s spent %d held %d charged %d", cs.name, res.Outcome, ledger.SpentCents(), ledger.HeldCents(), res.ChargedCents)
		}
		if d.count() != 1 {
			t.Errorf("%s: %d requests", cs.name, d.count())
		}
	}
}

// TestRefusalsMakeNoRequest holds that an invalid document, an exhausted
// budget, a missing ledger and a dedupe hit each cost no request.
func TestRefusalsMakeNoRequest(t *testing.T) {
	t.Parallel()
	d := newDouble(t, func(w http.ResponseWriter, _ *http.Request) {
		envelope(w, 200, "Sucesso", "", "uid-dedupe", map[string]any{"nome": "X"})
	})
	c := d.client(t, directd.Config{})
	ctx := t.Context()

	m, ledger := meter(1000)
	for _, doc := range []string{"12345678900", "11222333000180", "abc", "000.000.000-00"} {
		if _, err := c.Query(ctx, m, directd.Request{Product: "CadastroPessoaFisicaPlus", Document: doc}); !errors.Is(err, directd.ErrInvalidDocument) {
			t.Errorf("%q: %v", doc, err)
		}
	}
	if d.count() != 0 || ledger.HeldCents() != 0 || ledger.SpentCents() != 0 {
		t.Fatalf("invalid documents: %d requests, %d held", d.count(), ledger.HeldCents())
	}

	if _, err := c.Query(ctx, directd.Meter{}, directd.Request{Product: "CadastroPessoaFisicaPlus", Document: syntheticCPF}); !errors.Is(err, directd.ErrNoLedger) {
		t.Errorf("no ledger: %v", err)
	}
	poor, _ := meter(35)
	if _, err := c.Query(ctx, poor, directd.Request{Product: "CadastroPessoaFisicaPlus", Document: syntheticCPF}); !errors.Is(err, directd.ErrBudgetExceeded) {
		t.Errorf("budget: %v", err)
	}
	if d.count() != 0 {
		t.Fatalf("refusals made %d requests", d.count())
	}

	dedupe := directd.NewMemoryDedupe()
	dm := directd.Meter{Ledger: ledger, Dedupe: dedupe}
	first, err := c.Query(ctx, dm, directd.Request{Product: "CadastroPessoaFisicaPlus", Document: "123.456.789-09"})
	if err != nil || first.Cached {
		t.Fatal(err)
	}
	second, err := c.Query(ctx, dm, directd.Request{Product: "CadastroPessoaFisicaPlus", Document: syntheticCPF})
	if err != nil {
		t.Fatal(err)
	}
	if !second.Cached || second.ChargedCents != 0 || second.ConsultaUID != "uid-dedupe" || d.count() != 1 || ledger.SpentCents() != 36 {
		t.Errorf("dedupe: cached %v charged %d requests %d spent %d", second.Cached, second.ChargedCents, d.count(), ledger.SpentCents())
	}
	if _, err := c.Query(ctx, dm, directd.Request{Product: "CadastroPessoaFisica", Document: syntheticCPF}); err != nil || d.count() != 2 {
		t.Errorf("another product is another query: %v, %d requests", err, d.count())
	}
	rf := func(birth string) directd.Request {
		return directd.Request{Product: "ReceitaFederalPessoaFisica", Document: syntheticCPF, Params: map[string]string{"DataNascimento": birth}}
	}
	_, _ = c.Query(ctx, dm, rf("01/01/1990"))
	_, _ = c.Query(ctx, dm, rf("02/01/1990"))
	_, _ = c.Query(ctx, dm, rf("01/01/1990"))
	if d.count() != 4 {
		t.Errorf("other parameters are another query, the same ones are not: %d requests", d.count())
	}
}

// TestTheBreakerOpensAndRecovers drives the circuit breaker with a fake
// clock.
func TestTheBreakerOpensAndRecovers(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	failing := true
	d := newDouble(t, func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if failing {
			w.WriteHeader(503)
			return
		}
		envelope(w, 200, "Sucesso", "", "u", map[string]any{})
	})
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	var clock sync.Mutex
	c := d.client(t, directd.Config{
		Breaker: directd.BreakerConfig{Threshold: 2, Cooldown: time.Minute},
		Now: func() time.Time {
			clock.Lock()
			defer clock.Unlock()
			return now
		},
	})
	m, ledger := meter(10000)
	req := directd.Request{Product: "CadastroPessoaFisica", Document: syntheticCPF}
	for i := 0; i < 2; i++ {
		if res, err := c.Query(t.Context(), m, req); err != nil || res.Outcome != directd.OutcomeUnavailable {
			t.Fatalf("failure %d: %v", i, err)
		}
	}
	if c.BreakerState() != directd.BreakerOpen {
		t.Fatalf("state %s after two failures", c.BreakerState())
	}
	if _, err := c.Query(t.Context(), m, req); !errors.Is(err, directd.ErrUnavailable) || d.count() != 2 || ledger.HeldCents() != 0 {
		t.Fatalf("open breaker: %v, %d requests", err, d.count())
	}
	clock.Lock()
	now = now.Add(time.Minute)
	clock.Unlock()
	mu.Lock()
	failing = false
	mu.Unlock()
	if res, err := c.Query(t.Context(), m, req); err != nil || res.Outcome != directd.OutcomeFound {
		t.Fatalf("probe: %v", err)
	}
	if c.BreakerState() != directd.BreakerClosed || ledger.SpentCents() != 16 {
		t.Errorf("state %s spent %d", c.BreakerState(), ledger.SpentCents())
	}
}

// TestTheTokenNeverReachesLogsOrErrors runs successes, refusals and a
// transport failure with a JSON logger and checks every log line and error.
func TestTheTokenNeverReachesLogsOrErrors(t *testing.T) {
	t.Parallel()
	d := newDouble(t, func(w http.ResponseWriter, _ *http.Request) {
		envelope(w, 200, "Sucesso", "", "u", map[string]any{"nome": "FULANA SINTETICA"})
	})
	var logs bytes.Buffer
	var mu sync.Mutex
	logger := slog.New(slog.NewJSONHandler(&lockedWriter{w: &logs, mu: &mu}, &slog.HandlerOptions{Level: slog.LevelDebug}))
	c := d.client(t, directd.Config{Logger: logger})
	m, _ := meter(10000)
	var errs []error
	_, err := c.Query(t.Context(), m, directd.Request{Product: "CadastroPessoaFisicaPlus", Document: syntheticCPF})
	errs = append(errs, err)
	_, err = c.Query(t.Context(), m, directd.Request{Product: "CadastroPessoaFisicaPlus", Document: "12345678900"})
	errs = append(errs, err)

	d.server.Close()
	res, err := c.Query(t.Context(), m, directd.Request{Product: "CadastroPessoaFisicaPlus", Document: syntheticCPF})
	if err == nil || res == nil || res.Outcome != directd.OutcomeUnavailable {
		t.Fatalf("a closed server: %v %+v", err, res)
	}
	errs = append(errs, err)

	mu.Lock()
	text := logs.String()
	mu.Unlock()
	for _, e := range errs {
		if e != nil {
			text += "\n" + e.Error() + fmt.Sprintf(" %v %+v", e, e)
		}
	}
	for _, leak := range []string{secretToken, "TOKEN=", syntheticCPF, "FULANA"} {
		if strings.Contains(text, leak) {
			t.Errorf("%q leaked:\n%s", leak, text)
		}
	}
	if !strings.Contains(text, `"document":"***.***.***-09"`) || !strings.Contains(text, `"product":"CadastroPessoaFisicaPlus"`) {
		t.Errorf("the log lacks the masked document or the product:\n%s", text)
	}
}

// lockedWriter serializes writes from concurrent log calls.
type lockedWriter struct {
	w  *bytes.Buffer
	mu *sync.Mutex
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}

// TestConcurrentQueries runs queries from many goroutines under the race
// detector.
func TestConcurrentQueries(t *testing.T) {
	t.Parallel()
	d := newDouble(t, func(w http.ResponseWriter, _ *http.Request) {
		envelope(w, 200, "Sucesso", "", "u", map[string]any{"nome": "X"})
	})
	c := d.client(t, directd.Config{})
	m := directd.Meter{Ledger: directd.NewMemoryLedger(100000), Dedupe: directd.NewMemoryDedupe()}
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := c.Query(t.Context(), m, directd.Request{Product: "CadastroPessoaFisica", Document: syntheticCPF}); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if d.count() < 1 || d.count() > 16 {
		t.Errorf("%d requests", d.count())
	}
}
