package unit_test

import (
	"strings"
	"testing"

	directd "github.com/hyz-is/arandu-directd"
)

// syntheticAnswer is a made-up answer in Direct Data's envelope, with a value
// of every personal kind, each one distinctive so a leak is found by search.
const syntheticAnswer = `{
  "metaDados": {"consultaNome": "Cadastro - Pessoa Física - Plus", "consultaUid": "uid-synthetic-1",
    "chave": "12345678909", "usuario": "owner@example.invalid", "mensagem": "Consulta ao CPF 123.456.789-09 ok",
    "ip": "203.0.113.7", "resultadoId": 1, "resultado": "Sucesso", "apiVersao": "v3", "assincrono": false,
    "data": "18/10/2026 10:00:00", "tempoExecucaoMs": 120},
  "retorno": {
    "cpf": "123.456.789-09", "nome": "FULANA SINTETICA DE TESTE", "nomeMae": "MAE SINTETICA DE TESTE",
    "nomePai": "PAI SINTETICO DE TESTE", "dataNascimento": "01/02/1990", "sexo": "F", "idade": 36,
    "situacaoCadastral": "REGULAR", "obito": false,
    "telefones": [{"telefoneComDDD": "(11) 99999-0000", "operadora": "OPERADORA X", "whatsApp": true}],
    "enderecos": [{"logradouro": "RUA SINTETICA", "numero": "123", "complemento": "APTO 9", "bairro": "BAIRRO SINTETICO",
      "cidade": "CIDADE SINTETICA", "uf": "SP", "cep": "01234-567"}],
    "emails": [{"enderecoEmail": "fulana.sintetica@example.invalid"}],
    "parentescos": [{"grauParentesco": "MAE", "cpf": "987.654.321-00", "nome": "MAE SINTETICA DE TESTE"}],
    "socios": [{"documento": "11.222.333/0001-81", "nome": "SOCIO SINTETICO", "cargo": "ADMINISTRADOR"}],
    "observacoes": "texto livre sobre FULANA SINTETICA",
    "rendaEstimada": "R$ 5.000,00"
  }
}`

func TestRedactLeavesNoPersonalValue(t *testing.T) {
	t.Parallel()
	out, ok := directd.Redact([]byte(syntheticAnswer))
	if !ok {
		t.Fatal("the synthetic answer did not redact")
	}
	text := string(out)
	for _, leak := range []string{
		"12345678909", "123.456.789", "456.789", "987.654.321", "11.222.333/0001", "FULANA", "SINTETICA", "SINTETICO",
		"01/02/1990", "99999-0000", "01234-567", "RUA ", "APTO", "example.invalid", "203.0.113.7", "R$ 5.000",
	} {
		if strings.Contains(text, leak) {
			t.Errorf("%q survived redaction:\n%s", leak, text)
		}
	}
	for _, kept := range []string{
		`"situacaoCadastral":"REGULAR"`, `"resultado":"Sucesso"`, `"uf":"SP"`, `"sexo":"F"`, `"obito":false`,
		`"cargo":"ADMINISTRADOR"`, `"whatsApp":true`, `"nome":"<name>"`, `"nomeMae":"<name>"`, `"cep":"<address>"`,
		`"enderecoEmail":"<email>"`, `"telefoneComDDD":"<phone>"`, `"cpf":"***.***.***-09"`, `"dataNascimento":"<birthdate>"`,
		`"consultaUid":"uid-synthetic-1"`, `"mensagem":"<redacted>"`,
	} {
		if !strings.Contains(text, kept) {
			t.Errorf("%s is not in the redacted answer:\n%s", kept, text)
		}
	}
	if _, ok := directd.Redact([]byte("<html>12345678909</html>")); ok {
		t.Error("a body that is not JSON must not come back")
	}
}
