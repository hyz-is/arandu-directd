package unit_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	br "github.com/hyz-is/arandu-br"

	directd "github.com/hyz-is/arandu-directd"
)

// Synthetic documents: valid check digits, issued to nobody known.
const (
	syntheticCPF  = "12345678909"
	syntheticCNPJ = "11222333000181"
	secretToken   = "tok-SYNTHETIC-0123456789abcdef"
)

func syntheticAlphanumericCNPJ(t *testing.T) string {
	t.Helper()
	base := "12ABC34501DE"
	check, err := br.CNPJCheckDigits(base)
	if err != nil {
		t.Fatal(err)
	}
	return base + check
}

func newClient(t *testing.T, cfg directd.Config) *directd.Client {
	t.Helper()
	if cfg.Token == "" {
		cfg.Token = secretToken
	}
	c, err := directd.New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

func TestTheTokenNeverPrints(t *testing.T) {
	t.Parallel()
	tok := directd.Token(secretToken)
	cfg := directd.Config{Token: tok, BaseURL: "https://example.invalid"}

	var logs bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&logs, nil))
	log.Info("config", "token", tok, "cfg", cfg)
	log.Info("text", slog.Any("token", tok))
	text := slog.New(slog.NewTextHandler(&logs, nil))
	text.Info("config", "token", tok, "cfg", cfg)

	js, _ := json.Marshal(struct{ T directd.Token }{tok})
	outputs := []string{
		fmt.Sprint(tok), fmt.Sprintf("%v %s %q %x %+v %#v", tok, tok, tok, tok, tok, tok),
		fmt.Sprintf("%v %+v %#v", cfg, cfg, cfg), string(js), logs.String(),
	}
	for _, out := range outputs {
		if strings.Contains(out, secretToken) {
			t.Errorf("the token printed: %s", out)
		}
	}
}

func TestConfigRefusesWhatCannotWork(t *testing.T) {
	t.Parallel()
	cases := map[string]directd.Config{
		"no token":         {},
		"token with space": {Token: "a b"},
		"http host":        {Token: secretToken, BaseURL: "http://apiv3.example.com"},
		"query in base":    {Token: secretToken, BaseURL: "https://apiv3.example.com?TOKEN=x"},
		"negative timeout": {Token: secretToken, Timeout: -time.Second},
		"free product":     {Token: secretToken, Products: map[string]directd.Product{"X": {Path: "/api/X"}}},
		"bad path":         {Token: secretToken, Products: map[string]directd.Product{"X": {Path: "/X", PriceCents: 1}}},
		"unknown outcome":  {Token: secretToken, ResultCodes: map[int]directd.Outcome{7: "approved"}},
	}
	for name, cfg := range cases {
		if _, err := directd.New(cfg); err == nil {
			t.Errorf("%s: New accepted it", name)
		} else if strings.Contains(err.Error(), secretToken) {
			t.Errorf("%s: the error carries the token", name)
		}
	}
	if _, err := directd.New(directd.Config{Token: secretToken, BaseURL: "http://127.0.0.1:9"}); err != nil {
		t.Errorf("a loopback http host is a test double and must be accepted: %v", err)
	}
}

func TestClassifyMapsTheDocumentedAnswers(t *testing.T) {
	t.Parallel()
	meta := func(resultado, mensagem string) *directd.MetaData {
		return &directd.MetaData{Resultado: resultado, Mensagem: mensagem}
	}
	cases := []struct {
		status int
		meta   *directd.MetaData
		codes  map[int]directd.Outcome
		want   directd.Outcome
	}{
		{200, meta("Sucesso", ""), nil, directd.OutcomeFound},
		{200, meta("SUCESSO", ""), nil, directd.OutcomeFound},
		{200, meta("Sucesso com Ressalvas", ""), nil, directd.OutcomeFoundWithCaveats},
		{400, nil, nil, directd.OutcomeInvalidDocument},
		{401, nil, nil, directd.OutcomeUnauthorized},
		{403, nil, nil, directd.OutcomeUnauthorized},
		{402, nil, nil, directd.OutcomeInsufficientBalance},
		{404, nil, nil, directd.OutcomeNotFound},
		{429, nil, nil, directd.OutcomeRateLimited},
		{500, nil, nil, directd.OutcomeUnavailable},
		{503, meta("Sucesso", ""), nil, directd.OutcomeUnavailable},
		{302, nil, nil, directd.OutcomeError},
		{200, meta("Falha", "Registro não encontrado"), nil, directd.OutcomeNotFound},
		{200, meta("Erro", "Saldo insuficiente para a consulta"), nil, directd.OutcomeInsufficientBalance},
		{200, meta("Erro", "Token inválido"), nil, directd.OutcomeUnauthorized},
		{200, meta("Erro", "Fonte indisponível no momento"), nil, directd.OutcomeUnavailable},
		{200, meta("Erro", "CPF inválido"), nil, directd.OutcomeInvalidDocument},
		{200, meta("Algo novo", "sem explicação"), nil, directd.OutcomeError},
		{200, &directd.MetaData{ResultadoID: 3, Resultado: "Sucesso"}, map[int]directd.Outcome{3: directd.OutcomeNotFound}, directd.OutcomeNotFound},
		{200, nil, map[int]directd.Outcome{0: directd.OutcomeNotFound}, directd.OutcomeError},
	}
	for i, c := range cases {
		if got := directd.Classify(c.status, c.meta, c.codes); got != c.want {
			t.Errorf("case %d (%d %+v): %s, want %s", i, c.status, c.meta, got, c.want)
		}
	}
	for _, o := range []directd.Outcome{directd.OutcomeFound, directd.OutcomeFoundWithCaveats} {
		if !o.Billable() {
			t.Errorf("%s must be billable (terms clause 2.5)", o)
		}
	}
	for _, o := range []directd.Outcome{directd.OutcomeNotFound, directd.OutcomeError, directd.OutcomeUnavailable, directd.OutcomeInvalidDocument} {
		if o.Billable() {
			t.Errorf("%s must not be billable", o)
		}
	}
}

func TestCheckRefusesBeforeAnyRequest(t *testing.T) {
	t.Parallel()
	c := newClient(t, directd.Config{Products: map[string]directd.Product{"CadastroPessoaFisica": {Disabled: true}}})
	cases := []struct {
		req  directd.Request
		want error
	}{
		{directd.Request{Product: "NoSuchAPI", Document: syntheticCPF}, directd.ErrUnknownProduct},
		{directd.Request{Product: "CadastroPessoaFisica", Document: syntheticCPF}, directd.ErrProductDisabled},
		{directd.Request{Product: "CadastroPessoaFisicaPlus", Document: "12345678900"}, directd.ErrInvalidDocument},
		{directd.Request{Product: "CadastroPessoaFisicaPlus", Document: "111.111.111-11"}, directd.ErrInvalidDocument},
		{directd.Request{Product: "CadastroPessoaFisicaPlus", Document: syntheticCNPJ}, directd.ErrInvalidDocument},
		{directd.Request{Product: "CadastroPessoaJuridica", Document: syntheticCPF}, directd.ErrInvalidDocument},
		{directd.Request{Product: "CadastroPessoaFisicaPlus"}, directd.ErrInvalidParams},
		{directd.Request{Product: "CadastroPessoaFisicaPlus", Document: syntheticCPF, Params: map[string]string{"TOKEN": "x"}}, directd.ErrInvalidParams},
		{directd.Request{Product: "CadastroPessoaFisicaPlus", Document: syntheticCPF, Params: map[string]string{"async": "habilitar"}}, directd.ErrInvalidParams},
		{directd.Request{Product: "CadastroPessoaFisicaPlus", Document: syntheticCPF, Params: map[string]string{"Nome": "x"}}, directd.ErrInvalidParams},
		{directd.Request{Product: "CadastroPessoaFisicaPlus", Params: map[string]string{"CPF": syntheticCPF}}, directd.ErrInvalidParams},
	}
	for i, cs := range cases {
		if err := c.Check(cs.req); !errors.Is(err, cs.want) {
			t.Errorf("case %d: %v, want %v", i, err, cs.want)
		}
	}
	for _, ok := range []directd.Request{
		{Product: "CadastroPessoaFisicaPlus", Document: "123.456.789-09"},
		{Product: "CadastroPessoaJuridicaPlus", Document: "11.222.333/0001-81"},
		{Product: "CadastroPessoaJuridicaPlus", Document: syntheticAlphanumericCNPJ(t)},
		{Product: "ProcessosJudiciaisSimplificada", Document: syntheticCPF},
		{Product: "ProcessosJudiciaisSimplificada", Document: syntheticCNPJ},
		{Product: "ReceitaFederalPessoaFisica", Document: syntheticCPF, Params: map[string]string{"datanascimento": "01/01/1990"}},
	} {
		if err := c.Check(ok); err != nil {
			t.Errorf("%+v: %v", ok, err)
		}
	}
}

func TestMaskDocumentHidesTheDocument(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		syntheticCPF:         "***.***.***-09",
		"123.456.789-09":     "***.***.***-09",
		syntheticCNPJ:        "11.***.***/****-81",
		"11.222.333/0001-81": "11.***.***/****-81",
		"not a document":     "",
	}
	for in, want := range cases {
		if got := directd.MaskDocument(in); got != want {
			t.Errorf("MaskDocument(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTheMemoryLedgerHoldsTheCap(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	l := directd.NewMemoryLedger(100)
	a, err := l.Reserve(ctx, directd.Charge{Product: "A", PriceCents: 60})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := l.Reserve(ctx, directd.Charge{Product: "B", PriceCents: 41}); !errors.Is(err, directd.ErrBudgetExceeded) {
		t.Fatalf("a hold past the cap: %v", err)
	}
	if err := l.Release(ctx, a); err != nil {
		t.Fatal(err)
	}
	b, err := l.Reserve(ctx, directd.Charge{Product: "B", PriceCents: 100})
	if err != nil {
		t.Fatal(err)
	}
	if err := l.Settle(ctx, b, 100); err != nil {
		t.Fatal(err)
	}
	if l.SpentCents() != 100 || l.RemainingCents() != 0 || l.HeldCents() != 0 {
		t.Errorf("spent %d remaining %d held %d", l.SpentCents(), l.RemainingCents(), l.HeldCents())
	}
	if err := l.Settle(ctx, b, 1); err == nil {
		t.Error("a hold settled twice")
	}
}

func TestEveryCatalogueProductIsUsable(t *testing.T) {
	t.Parallel()
	products := directd.DefaultProducts()
	if len(products) != len(directd.ProductSlugs()) || len(products) < 150 {
		t.Fatalf("%d products", len(products))
	}
	for slug, p := range products {
		if p.Slug != slug || !strings.HasPrefix(p.Path, "/api/") || p.PriceCents <= 0 || p.DocURL == "" {
			t.Errorf("%s: %+v", slug, p)
		}
		for _, q := range p.Params {
			if strings.EqualFold(q.Name, "TOKEN") || strings.EqualFold(q.Name, "ASync") {
				t.Errorf("%s lists %s, which the client sets", slug, q.Name)
			}
		}
	}
	// The products KUAA queries first, with their documented list prices.
	prices := map[string]int64{
		"CadastroPessoaFisica": 16, "CadastroPessoaFisicaPlus": 36, "ReceitaFederalPessoaFisica": 36,
		"CadastroPessoaJuridica": 16, "CadastroPessoaJuridicaPlus": 36, "AML": 72, "Similarity": 72,
		"SimplesNacional": 36, "ProcessosJudiciaisSimplificada": 150,
	}
	for slug, cents := range prices {
		if got := products[slug].PriceCents; got != cents {
			t.Errorf("%s costs %d cents, want %d", slug, got, cents)
		}
	}
	if p := products["ReceitaFederalPessoaFisica"]; !p.Supplier || !p.TakesCPF() || p.TakesCNPJ() {
		t.Errorf("ReceitaFederalPessoaFisica: %+v", p)
	}
	if p := products["ProcessosJudiciaisSimplificada"]; !p.TakesCPF() || !p.TakesCNPJ() {
		t.Errorf("ProcessosJudiciaisSimplificada takes a CPF or a CNPJ: %+v", p)
	}
}
