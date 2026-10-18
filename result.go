package directd

import (
	"encoding/json"
	"time"
)

// MetaData is the metaDados object Direct Data puts beside every answer.
type MetaData struct {
	// ConsultaNome is the name of the API that answered.
	ConsultaNome string `json:"consultaNome"`
	// ConsultaUID is Direct Data's identifier of the query: the reference to
	// record as evidence and to reconcile the statement with.
	ConsultaUID string `json:"consultaUid"`
	// Chave is the key the query was made with, as Direct Data echoes it. It
	// may hold the queried document.
	Chave string `json:"chave"`
	// Usuario is the account that made the query.
	Usuario string `json:"usuario"`
	// Mensagem is Direct Data's message about the query.
	Mensagem string `json:"mensagem"`
	// IP is the address the query came from, as Direct Data saw it.
	IP string `json:"ip"`
	// ResultadoID is Direct Data's numeric status code. The public
	// documentation lists no values.
	ResultadoID int `json:"resultadoId"`
	// Resultado is Direct Data's status text, such as "Sucesso" or "Sucesso
	// com Ressalvas" (terms clause 2.5).
	Resultado string `json:"resultado"`
	// APIVersao is the API version, "v3".
	APIVersao string `json:"apiVersao"`
	// EnviarCallback says whether Direct Data will call the configured
	// callback.
	EnviarCallback bool `json:"enviarCallback"`
	// GerarComprovante says whether a PDF receipt was asked for.
	GerarComprovante bool `json:"gerarComprovante"`
	// URLComprovante is where the PDF receipt can be downloaded.
	URLComprovante string `json:"urlComprovante"`
	// Assincrono says whether the query runs asynchronously.
	Assincrono bool `json:"assincrono"`
	// Data is when Direct Data ran the query, as it writes it
	// (dd/MM/yyyy HH:mm:ss).
	Data string `json:"data"`
	// TempoExecucaoMs is Direct Data's own execution time, in milliseconds.
	TempoExecucaoMs int64 `json:"tempoExecucaoMs"`
}

// wireMeta reads metaDados with the tolerant scalar types, since the
// documentation gives only example values.
type wireMeta struct {
	ConsultaNome     Text   `json:"consultaNome"`
	ConsultaUID      Text   `json:"consultaUid"`
	Chave            Text   `json:"chave"`
	Usuario          Text   `json:"usuario"`
	Mensagem         Text   `json:"mensagem"`
	IP               Text   `json:"ip"`
	ResultadoID      Number `json:"resultadoId"`
	Resultado        Text   `json:"resultado"`
	APIVersao        Text   `json:"apiVersao"`
	EnviarCallback   Flag   `json:"enviarCallback"`
	GerarComprovante Flag   `json:"gerarComprovante"`
	URLComprovante   Text   `json:"urlComprovante"`
	Assincrono       Flag   `json:"assincrono"`
	Data             Text   `json:"data"`
	TempoExecucaoMs  Number `json:"tempoExecucaoMs"`
}

func (w wireMeta) meta() MetaData {
	id, _ := w.ResultadoID.Int64()
	ms, _ := w.TempoExecucaoMs.Int64()
	return MetaData{
		ConsultaNome: string(w.ConsultaNome), ConsultaUID: string(w.ConsultaUID), Chave: string(w.Chave),
		Usuario: string(w.Usuario), Mensagem: string(w.Mensagem), IP: string(w.IP), ResultadoID: int(id),
		Resultado: string(w.Resultado), APIVersao: string(w.APIVersao), EnviarCallback: bool(w.EnviarCallback),
		GerarComprovante: bool(w.GerarComprovante), URLComprovante: string(w.URLComprovante),
		Assincrono: bool(w.Assincrono), Data: string(w.Data), TempoExecucaoMs: ms,
	}
}

// Result is one query, as evidence: what was asked, when, what it cost, what
// Direct Data said, and the data. It holds no verdict. Whether a registry
// record supports a case is the application's policy.
type Result struct {
	// Product is the API's slug, such as CadastroPessoaFisicaPlus.
	Product string
	// Document is the queried document, masked (***.***.***-09 for a CPF,
	// 11.***.***/****-81 for a CNPJ), or "" when the query had none.
	Document string
	// RequestedAt is when the request left, in the configured location.
	RequestedAt time.Time
	// ElapsedMs is the round trip as the client measured it.
	ElapsedMs int64
	// ConsultaUID is Direct Data's identifier of the query.
	ConsultaUID string
	// PriceCents is the product's configured list price.
	PriceCents int64
	// ChargedCents is what the client settled on the Ledger for this call:
	// PriceCents when the outcome is billable, 0 when the hold was released
	// or the result came from Dedupe.
	ChargedCents int64
	// Cached is true when the result came from Dedupe and no request was
	// made.
	Cached bool
	// Async is true when the result arrived through a callback.
	Async bool
	// Outcome is what the query came to.
	Outcome Outcome
	// HTTPStatus is the HTTP status of the answer, or of the httpStatusCode
	// field when the body carries one; 0 when no answer arrived.
	HTTPStatus int
	// Status is metaDados.resultado, verbatim.
	Status string
	// StatusID is metaDados.resultadoId.
	StatusID int
	// Message is metaDados.mensagem, or the error envelope's message.
	Message string
	// Errors is the error envelope's errors list, when there is one.
	Errors []string
	// ReceiptURL is metaDados.urlComprovante.
	ReceiptURL string
	// Meta is the whole metaDados object.
	Meta MetaData
	// Retorno is the retorno object, verbatim. It is personal data: store it
	// encrypted, apart from the normalized facts.
	Retorno json.RawMessage
	// Body is the whole answer, verbatim, when it is JSON. Same care as
	// Retorno.
	Body json.RawMessage
	// Data is Retorno decoded into the product's generated type (a pointer,
	// such as *CadastroPessoaFisicaPlusRetorno), or nil when the product has
	// none, there was no retorno, or decoding failed.
	Data any
	// DataError says why Retorno did not decode into Data. Retorno is kept
	// either way.
	DataError string

	// holdID is the Ledger hold the call reserved.
	holdID string
}

// envelope is a whole answer.
type envelope struct {
	MetaDados      *wireMeta       `json:"metaDados"`
	Retorno        json.RawMessage `json:"retorno"`
	HTTPStatusCode Number          `json:"httpStatusCode"`
	Message        Text            `json:"message"`
	Errors         json.RawMessage `json:"errors"`
}

// errorTexts reads the error envelope's errors list, whatever its shape.
func errorTexts(raw json.RawMessage) []string {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var list []json.RawMessage
	if json.Unmarshal(raw, &list) != nil {
		return []string{string(raw)}
	}
	out := make([]string, 0, len(list))
	for _, e := range list {
		var s string
		if json.Unmarshal(e, &s) == nil {
			out = append(out, s)
			continue
		}
		out = append(out, string(e))
	}
	return out
}

// As returns r.Data as *T, and false when it holds something else.
func As[T any](r *Result) (*T, bool) {
	if r == nil {
		return nil, false
	}
	v, ok := r.Data.(*T)
	return v, ok
}
