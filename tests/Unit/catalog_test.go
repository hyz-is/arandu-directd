package unit_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	directd "github.com/hyz-is/arandu-directd"
	"github.com/hyz-is/arandu-directd/internal/catalog"
	"github.com/hyz-is/arandu-directd/internal/gen"
)

// TestTheGeneratedFilesAreCurrent holds the generated Go files and the
// README's API table equal to what the generator writes from the committed
// catalogue now. A catalogue edited without go generate, or a generated file
// edited by hand, fails here.
func TestTheGeneratedFilesAreCurrent(t *testing.T) {
	t.Parallel()
	root := packageRoot(t)
	c, err := gen.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	readme, err := os.ReadFile(filepath.Join(root, gen.ReadmeFile))
	if err != nil {
		t.Fatal(err)
	}
	files, err := gen.Generate(c, readme)
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range files {
		got, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%s is stale or edited by hand: run go generate ./...", name)
		}
	}
}

// TestTheCatalogueIsWhole checks the committed catalogue against itself and
// against the products the package ships.
func TestTheCatalogueIsWhole(t *testing.T) {
	t.Parallel()
	c, err := gen.Load(packageRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	if c.SchemaVersion != catalog.SchemaVersion || c.Count != len(c.APIs) || c.CrawledAt == "" || c.Source != catalog.IndexURL {
		t.Fatalf("header: version %d count %d/%d crawled %q source %q", c.SchemaVersion, c.Count, len(c.APIs), c.CrawledAt, c.Source)
	}
	sum := 0
	for _, n := range c.Categories {
		sum += n
	}
	if sum != c.Count {
		t.Errorf("categories add up to %d, want %d", sum, c.Count)
	}
	if c.CrawledAt != directd.CatalogCrawledAt {
		t.Errorf("the products were generated from a catalogue of %s, not %s", directd.CatalogCrawledAt, c.CrawledAt)
	}
	products := directd.DefaultProducts()
	seen := map[string]bool{}
	for _, a := range c.APIs {
		if seen[a.Slug] {
			t.Errorf("%s appears twice", a.Slug)
		}
		seen[a.Slug] = true
		if a.Incomplete && len(a.IncompleteReasons) == 0 {
			t.Errorf("%s is incomplete without a reason", a.Slug)
		}
		u, err := url.Parse(a.DocURL)
		if err != nil || catalog.CheckURL(u) != nil {
			t.Errorf("%s: documentation URL %q is not on the public documentation", a.Slug, a.DocURL)
		}
		p, ok := products[a.Slug]
		if !ok {
			t.Errorf("%s is in the catalogue and not a product", a.Slug)
			continue
		}
		if p.PriceCents != a.PriceCents || p.Path != a.Path || string(p.Status) != a.Status || p.Supplier != a.Supplier {
			t.Errorf("%s: product %+v disagrees with the catalogue", a.Slug, p)
		}
	}
	if len(products) != len(c.APIs) {
		t.Errorf("%d products for %d catalogue entries", len(products), len(c.APIs))
	}
}

// TestTheCrawlerRefusesTheAPIAndThePortal holds that the documentation
// crawler cannot be pointed at a host where a request is billed or
// authenticated.
func TestTheCrawlerRefusesTheAPIAndThePortal(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{
		"https://apiv3.directd.com.br/api/CadastroPessoaFisica?CPF=1&TOKEN=x",
		"https://app.directd.com.br/dashboard",
		"http://directd.com.br/central-de-ajuda/apis/catalogo",
		"https://example.com/central-de-ajuda/apis/catalogo",
	} {
		u, _ := url.Parse(raw)
		if err := catalog.CheckURL(u); !errors.Is(err, catalog.ErrRefusedHost) {
			t.Errorf("%s: %v", raw, err)
		} else if strings.Contains(err.Error(), "TOKEN=x") {
			t.Errorf("the refusal prints the query: %v", err)
		}
		if _, err := catalog.NewFetcher(0).Get(t.Context(), raw); !errors.Is(err, catalog.ErrRefusedHost) {
			t.Errorf("Get(%s) = %v", raw, err)
		}
	}
	u, _ := url.Parse(catalog.PageURL("PessoaExpostaPoliticamente"))
	if err := catalog.CheckURL(u); err != nil {
		t.Errorf("the documentation itself is refused: %v", err)
	}
}

// flightPage wraps rows into a Next.js page the way the documentation serves
// them: each row a self.__next_f.push chunk.
func flightPage(t *testing.T, rows ...string) []byte {
	t.Helper()
	var b bytes.Buffer
	b.WriteString("<html><body>")
	for _, r := range rows {
		lit, err := json.Marshal(r)
		if err != nil {
			t.Fatal(err)
		}
		b.WriteString(`<script>self.__next_f.push([1,` + string(lit) + `])</script>`)
	}
	b.WriteString("</body></html>")
	return b.Bytes()
}

// TestTheParserReadsTheDocumentationPayload parses synthetic index and API
// pages built the way the documentation serves them: an element row with the
// items, a text row referenced by "$id", and a page with its params.
func TestTheParserReadsTheDocumentationPayload(t *testing.T) {
	t.Parallel()
	item := `{"apiName":"API Sintetica","description":"$1a","group":"Cadastral","sphere":"Federal","personType":"PF",` +
		`"url":"https://apiv3.directd.com.br/api/ApiSintetica?Cpf={0}&DataNascimento={1}","origin":"Online","sourceURL":"https://example.invalid",` +
		`"properties":[{"friendlyName":"CPF; Nome","description":"CPF da pessoa; Nome da pessoa"}],"customProperties":[],` +
		`"jsonExample":{"metaDados":{"resultado":"string"},"retorno":{"cpf":"string","telefones":[{"numero":"string"}],"idade":0,"obito":true}},` +
		`"isActive":true,"isMaintenance":true,"hasPDF":true,"price":0.36}`
	index := flightPage(t,
		"1a:T13,descricao\nsintetica\n",
		`2e:["$","$L36",null,{"items":[`+item+`],"initialSearch":""}]`+"\n")
	items, err := catalog.IndexItems(index)
	if err != nil || len(items) != 1 {
		t.Fatalf("IndexItems: %v, %d items", err, len(items))
	}
	api := catalog.FromIndex(items[0])
	if api.Slug != "ApiSintetica" || api.Path != "/api/ApiSintetica" || api.PriceCents != 36 || api.Status != "maintenance" ||
		!api.Supplier || !api.HasPDF || api.Description != "descricao\nsintetica" {
		t.Fatalf("FromIndex: %+v", api)
	}
	if len(api.Fields) != 2 || api.Fields[1].Label != "Nome" || api.Fields[1].Description != "Nome da pessoa" {
		t.Errorf("fields: %+v", api.Fields)
	}
	wantPaths := "metaDados.resultado string,retorno.cpf string,retorno.idade number,retorno.obito boolean,retorno.telefones[].numero string"
	if got := strings.Join(api.ResponsePaths, ","); got != wantPaths {
		t.Errorf("paths: %s", got)
	}

	page := flightPage(t, `30:["$","$L38",null,{"item":`+item+`,"siblings":[],"params":[`+
		`{"name":"CPF","in":"query","description":"O CPF.","schema":{"type":"string"}},`+
		`{"name":"GERARCOMPROVANTE","in":"query","description":"PDF.","schema":{"$ref":"#/components/schemas/OpcaoGerarComprovante"}}]}]`+"\n")
	_, params, err := catalog.PageParams(page)
	if err != nil {
		t.Fatal(err)
	}
	api.ApplyParams(params)
	if len(api.Params) != 2 || api.Params[0].QueryName != "Cpf" || !api.Params[0].InTemplate ||
		api.Params[1].Type != "ref" || api.Params[1].Ref != "OpcaoGerarComprovante" {
		t.Errorf("params: %+v", api.Params)
	}
	if len(api.Warnings) != 1 || !strings.Contains(api.Warnings[0], "DataNascimento") {
		t.Errorf("a template parameter missing from the table must warn: %v", api.Warnings)
	}
	if _, err := catalog.IndexItems([]byte("<html>nothing</html>")); err == nil {
		t.Error("a page without a payload parsed")
	}
}
