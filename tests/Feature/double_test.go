package feature_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	directd "github.com/hyz-is/arandu-directd"
	"github.com/hyz-is/arandu-directd/internal/catalog"
	"github.com/hyz-is/arandu-directd/internal/gen"
)

// Synthetic documents and token. The documents have valid check digits and
// belong to nobody known; the token is made up.
const (
	syntheticCPF  = "12345678909"
	syntheticCNPJ = "11222333000181"
	secretToken   = "tok-SYNTHETIC-0123456789abcdef"
)

// seen is one request the double received.
type seen struct {
	Path  string
	Query url.Values
}

// double is an httptest stand-in for the Direct Data API. It never forwards
// anything anywhere: every test in this package talks to it and to nothing
// else.
type double struct {
	t      *testing.T
	server *httptest.Server

	mu       sync.Mutex
	requests []seen
	// answer writes the answer to a request.
	answer func(w http.ResponseWriter, r *http.Request)
}

func newDouble(t *testing.T, answer func(w http.ResponseWriter, r *http.Request)) *double {
	t.Helper()
	d := &double{t: t, answer: answer}
	d.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		d.mu.Lock()
		d.requests = append(d.requests, seen{Path: r.URL.Path, Query: r.URL.Query()})
		answer := d.answer
		d.mu.Unlock()
		if r.Method != http.MethodGet {
			t.Errorf("method %s, want GET", r.Method)
		}
		answer(w, r)
	}))
	t.Cleanup(d.server.Close)
	return d
}

func (d *double) count() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.requests)
}

func (d *double) last() seen {
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.requests) == 0 {
		d.t.Fatal("no request reached the double")
	}
	return d.requests[len(d.requests)-1]
}

func (d *double) client(t *testing.T, cfg directd.Config) *directd.Client {
	t.Helper()
	cfg.BaseURL = d.server.URL
	if cfg.Token == "" {
		cfg.Token = secretToken
	}
	c, err := directd.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// envelope writes an answer in Direct Data's envelope.
func envelope(w http.ResponseWriter, status int, resultado, mensagem, uid string, retorno any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"metaDados": map[string]any{
			"consultaNome": "synthetic", "consultaUid": uid, "chave": "", "usuario": "", "mensagem": mensagem,
			"ip": "", "resultadoId": 1, "resultado": resultado, "apiVersao": "v3", "enviarCallback": false,
			"gerarComprovante": false, "urlComprovante": "", "assincrono": false, "data": "18/10/2026 10:00:00",
			"tempoExecucaoMs": 5,
		},
		"retorno": retorno,
	})
}

func packageRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("finding package root")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func loadCatalog(t *testing.T) catalog.Catalog {
	t.Helper()
	c, err := gen.Load(packageRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func meter(capCents int64) (directd.Meter, *directd.MemoryLedger) {
	l := directd.NewMemoryLedger(capCents)
	return directd.Meter{Ledger: l}, l
}
