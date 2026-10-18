// Package pii classifies the fields of Direct Data responses by the kind of
// personal data they hold, from the field's JSON key alone.
//
// The generator tags every generated field with its class, and the package's
// Redact replaces the values it classifies. The rules are by key because the
// documentation gives example values only; a key that is not recognised is
// ClassNone here, and Redact still hides its string value unless the key is
// one of the status-like keys in Keep.
package pii

import "strings"

// Class is a kind of personal data.
type Class string

// The classes.
const (
	ClassNone      Class = ""
	ClassDocument  Class = "document"
	ClassName      Class = "name"
	ClassAddress   Class = "address"
	ClassPhone     Class = "phone"
	ClassEmail     Class = "email"
	ClassBirthDate Class = "birthdate"
	ClassVehicle   Class = "vehicle"
	ClassNetwork   Class = "network"
	ClassAccount   Class = "account"
)

// exact maps lower-cased keys to their class.
var exact = map[string]Class{
	"cpf": ClassDocument, "cnpj": ClassDocument, "documento": ClassDocument, "documentoconsultado": ClassDocument,
	"numerocpf": ClassDocument, "numerocnpj": ClassDocument, "cpfcnpj": ClassDocument, "chave": ClassDocument,
	"nis": ClassDocument, "rg": ClassDocument, "cnh": ClassDocument, "curp": ClassDocument, "dni": ClassDocument,
	"numerotituloeleitoral": ClassDocument, "tituloeleitoral": ClassDocument, "pis": ClassDocument,
	"passaporte": ClassDocument, "registronacional": ClassDocument, "inscricaoestadual": ClassDocument,
	"ie": ClassDocument, "im": ClassDocument, "crm": ClassDocument, "oab": ClassDocument, "cpfsocio": ClassDocument,
	"name": ClassName, "surname": ClassName, "sobrenome": ClassName, "nome": ClassName, "alcunha": ClassName,
	"razaosocial": ClassName, "nomefantasia": ClassName, "nomeempresarial": ClassName, "mae": ClassName, "pai": ClassName,
	"logradouro": ClassAddress, "numero": ClassAddress, "complemento": ClassAddress, "bairro": ClassAddress,
	"cep": ClassAddress, "postcode": ClassAddress, "address": ClassAddress, "endereco": ClassAddress,
	"cidade": ClassAddress, "city": ClassAddress, "municipio": ClassAddress, "latitude": ClassAddress, "longitude": ClassAddress,
	"phone": ClassPhone, "celular": ClassPhone, "fone": ClassPhone,
	"email": ClassEmail, "enderecoemail": ClassEmail,
	"dob": ClassBirthDate, "datanascimento": ClassBirthDate, "nascimento": ClassBirthDate,
	"placa": ClassVehicle, "renavam": ClassVehicle, "chassi": ClassVehicle, "motor": ClassVehicle,
	"ip":      ClassNetwork,
	"usuario": ClassAccount,
}

// prefixes classify keys by their beginning, after exact.
var prefixes = []struct {
	prefix string
	class  Class
}{
	{"nome", ClassName}, {"telefone", ClassPhone}, {"email", ClassEmail}, {"endereco", ClassAddress},
	{"logradouro", ClassAddress}, {"cpf", ClassDocument}, {"cnpj", ClassDocument},
	{"datanascimento", ClassBirthDate}, {"numerocpf", ClassDocument},
}

// contains classify keys holding a fragment, after prefixes.
var contains = []struct {
	part  string
	class Class
}{
	{"nomemae", ClassName}, {"nomepai", ClassName}, {"email", ClassEmail}, {"telefone", ClassPhone},
	{"celular", ClassPhone}, {"cpf", ClassDocument}, {"cnpj", ClassDocument},
}

// Of returns the class of a JSON key.
func Of(key string) Class {
	k := strings.ToLower(strings.TrimSpace(key))
	if c, ok := exact[k]; ok {
		return c
	}
	for _, p := range prefixes {
		if strings.HasPrefix(k, p.prefix) {
			return p.class
		}
	}
	for _, p := range contains {
		if strings.Contains(k, p.part) {
			return p.class
		}
	}
	return ClassNone
}

// keepParts are fragments of keys whose string values describe a status, a
// category or a count rather than a person, and which Redact keeps.
var keepParts = []string{
	"situacao", "status", "resultado", "mensagem", "descricao", "description", "tipo", "porte", "natureza",
	"cnae", "sexo", "genero", "gender", "obito", "pep", "classe", "faixa", "quantidade", "total", "apiversao",
	"consultanome", "consultauid", "distance", "grau", "qualificacao", "cargo", "ramo", "opcao", "tributacao",
	"matriz", "orgao", "uf", "state", "operadora", "data", "regime", "especie", "categoria", "origem",
	"urlcomprovante", "possui", "consta", "optante", "enquadrado", "baixada", "ativo", "nivel", "score", "risco",
	"confiabilidade", "periodo", "competencia", "ano", "mes", "codigo",
}

// Keep reports whether Redact keeps the string value of a key that Of does
// not classify.
func Keep(key string) bool {
	k := strings.ToLower(key)
	for _, p := range keepParts {
		if strings.Contains(k, p) {
			return true
		}
	}
	return false
}
