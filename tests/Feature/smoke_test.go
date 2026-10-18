package feature_test

import (
	"bytes"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hyz-is/arandu-directd/internal/smoke"
)

// personal are the synthetic personal values the double answers with; none
// may reach the smoke run's output or files.
var personal = []string{
	syntheticCPF, "123.456.789", "FULANA SINTETICA", "MAE SINTETICA", "RUA SINTETICA", "(11) 99999-0000",
	"fulana@example.invalid", "01/02/1990", "01234-567",
}

func personalAnswer(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/api/CadastroPessoaJuridica" {
		envelope(w, 404, "Falha", "Registro não encontrado", "uid-404", nil)
		return
	}
	envelope(w, 200, "Sucesso", "Consulta do CPF 123.456.789-09 concluída", "uid-smoke", map[string]any{
		"cpf": "123.456.789-09", "nome": "FULANA SINTETICA", "nomeMae": "MAE SINTETICA", "dataNascimento": "01/02/1990",
		"situacaoCadastral": "REGULAR",
		"enderecos":         []any{map[string]any{"logradouro": "RUA SINTETICA", "cep": "01234-567", "uf": "SP"}},
		"telefones":         []any{map[string]any{"telefoneComDDD": "(11) 99999-0000"}},
		"emails":            []any{map[string]any{"enderecoEmail": "fulana@example.invalid"}},
	})
}

func tokenFile(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(p, []byte(secretToken+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestTheSmokeRunHoldsTheCapAndLeaksNothing runs the smoke command's body
// against the double: the cap stops the run before the call that would pass
// it, a refused query counts as spent, and neither the token nor a personal
// value reaches stdout, stderr or the written answers.
func TestTheSmokeRunHoldsTheCapAndLeaksNothing(t *testing.T) {
	t.Parallel()
	d := newDouble(t, personalAnswer)
	out := filepath.Join(t.TempDir(), "answers")
	var stdout, stderr bytes.Buffer
	err := smoke.Run(t.Context(), smoke.Options{
		TokenFile:       tokenFile(t),
		SpendLimitCents: 60,
		Calls: []smoke.Call{
			{Product: "CadastroPessoaJuridica", Document: syntheticCNPJ},      // 16, answered 404: counted anyway
			{Product: "CadastroPessoaFisicaPlus", Document: "123.456.789-09"}, // 36, total 52
			{Product: "CadastroPessoaFisicaPlus", Document: syntheticCPF},     // dedupe hit: free
			{Product: "CadastroPessoaFisica", Document: syntheticCPF},         // 16 would make 68: refused
			{Product: "CadastroPessoaFisicaPlus", Document: "98765432100"},
		},
		OutDir:  out,
		Stdout:  &stdout,
		Stderr:  &stderr,
		BaseURL: d.server.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	if d.count() != 2 {
		t.Errorf("%d requests reached the double, want 2:\n%s", d.count(), stdout.String())
	}
	report := stdout.String()
	for _, want := range []string{"refused: the next call would pass the cap", "total=R$ 0,52", "outcome=not_found", "cached=true", "consultaUid=uid-smoke"} {
		if !strings.Contains(report, want) {
			t.Errorf("the report lacks %q:\n%s", want, report)
		}
	}

	files, err := filepath.Glob(filepath.Join(out, "*.json"))
	if err != nil || len(files) != 3 {
		t.Fatalf("%d answers written: %v", len(files), err)
	}
	all := report + stderr.String()
	for _, f := range files {
		info, _ := os.Stat(f)
		if info.Mode().Perm() != 0o600 {
			t.Errorf("%s is mode %v", f, info.Mode().Perm())
		}
		all += string(readFile(t, f))
	}
	if !strings.Contains(all, `"situacaoCadastral":"REGULAR"`) || !strings.Contains(all, `"nome":"<name>"`) {
		t.Errorf("the written answers lost their structure:\n%s", all)
	}
	for _, leak := range append([]string{secretToken, "TOKEN"}, personal...) {
		if strings.Contains(all, leak) {
			t.Errorf("%q leaked into the output", leak)
		}
	}
}

// TestTheSmokeRunRefusesBeforeSpending holds every refusal that comes before
// the first request.
func TestTheSmokeRunRefusesBeforeSpending(t *testing.T) {
	t.Parallel()
	d := newDouble(t, personalAnswer)
	one := []smoke.Call{{Product: "CadastroPessoaFisica", Document: syntheticCPF}}
	cases := map[string]smoke.Options{
		"no limit":          {TokenFile: tokenFile(t), Calls: one, OutDir: t.TempDir()},
		"limit above R$ 50": {TokenFile: tokenFile(t), SpendLimitCents: 5001, Calls: one, OutDir: t.TempDir()},
		"no calls":          {TokenFile: tokenFile(t), SpendLimitCents: 100, OutDir: t.TempDir()},
		"invalid document":  {TokenFile: tokenFile(t), SpendLimitCents: 100, Calls: []smoke.Call{{Product: "CadastroPessoaFisica", Document: "12345678900"}}, OutDir: t.TempDir()},
		"unknown product":   {TokenFile: tokenFile(t), SpendLimitCents: 100, Calls: []smoke.Call{{Product: "Nope", Document: syntheticCPF}}, OutDir: t.TempDir()},
		"out inside git":    {TokenFile: tokenFile(t), SpendLimitCents: 100, Calls: one, OutDir: filepath.Join(packageRoot(t), "smoke-answers")},
		"no token file":     {SpendLimitCents: 100, Calls: one, OutDir: t.TempDir()},
		"missing token":     {TokenFile: filepath.Join(t.TempDir(), "absent"), SpendLimitCents: 100, Calls: one, OutDir: t.TempDir()},
	}
	for name, o := range cases {
		o.BaseURL = d.server.URL
		var stdout, stderr bytes.Buffer
		o.Stdout, o.Stderr = &stdout, &stderr
		err := smoke.Run(t.Context(), o)
		if err == nil {
			t.Errorf("%s: accepted", name)
		} else if strings.Contains(err.Error()+stdout.String()+stderr.String(), secretToken) {
			t.Errorf("%s: the token leaked", name)
		}
	}
	if _, err := os.Stat(filepath.Join(packageRoot(t), "smoke-answers")); err == nil {
		t.Error("the refused output directory inside the repository was created")
	}
	if d.count() != 0 {
		t.Errorf("refusals made %d requests", d.count())
	}

	var stdout bytes.Buffer
	err := smoke.Run(t.Context(), smoke.Options{
		TokenFile: filepath.Join(t.TempDir(), "absent"), SpendLimitCents: 100, DryRun: true, BaseURL: d.server.URL,
		Calls:  []smoke.Call{{Product: "CadastroPessoaFisicaPlus", Document: syntheticCPF}, {Product: "SimplesNacional", Document: syntheticCNPJ}},
		Stdout: &stdout,
	})
	if err != nil || d.count() != 0 || !strings.Contains(stdout.String(), "plan total R$ 0,72") || !strings.Contains(stdout.String(), "dry run") {
		t.Errorf("dry run: %v, %d requests:\n%s", err, d.count(), stdout.String())
	}
	if strings.Contains(stdout.String(), syntheticCPF) {
		t.Error("the plan printed a document unmasked")
	}

	calls, err := smoke.ParseCalls(strings.NewReader("# comment\n\nReceitaFederalPessoaFisica 123.456.789-09 DataNascimento=01/01/1990\nConsultaPlaca - Placa=ABC1D23\n"))
	if err != nil || len(calls) != 2 || calls[0].Params["DataNascimento"] != "01/01/1990" || calls[1].Document != "" {
		t.Errorf("ParseCalls: %v %+v", err, calls)
	}
}
