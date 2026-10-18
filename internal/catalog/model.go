// Package catalog reads Direct Data's public API documentation and holds the
// catalogue built from it.
//
// The documentation is a Next.js site whose pages carry their data in the
// React Server Components payload (self.__next_f.push). The catalogue index
// lists every API with its endpoint template, price, status, field list and
// response schema; each API's own page adds its query parameters. Nothing here
// talks to the API itself: the crawler refuses the API and portal hosts.
package catalog

// SchemaVersion is the version of the catalogue file format. It changes when a
// field is renamed or removed, not when one is added.
const SchemaVersion = 1

// IndexURL is the public catalogue index.
const IndexURL = "https://directd.com.br/central-de-ajuda/apis/catalogo"

// PageURL is the public documentation page of the API with the given slug.
func PageURL(slug string) string { return IndexURL + "/" + slug }

// Catalog is the file catalog/catalog.json.
type Catalog struct {
	// SchemaVersion is the file format version, SchemaVersion when written.
	SchemaVersion int `json:"schema_version"`
	// CrawledAt is the date the documentation was read, YYYY-MM-DD.
	CrawledAt string `json:"crawled_at"`
	// Source is the catalogue index URL.
	Source string `json:"source"`
	// Notes record how fields were derived, so a reader does not take an
	// inference for a statement of the documentation.
	Notes []string `json:"notes"`
	// Count is len(APIs).
	Count int `json:"count"`
	// Categories counts the APIs per category, as the index groups them.
	Categories map[string]int `json:"categories"`
	// APIs holds one entry per API, sorted by category and slug.
	APIs []API `json:"apis"`
}

// API is one Direct Data API as its public documentation describes it.
type API struct {
	// Slug is the endpoint name, the last element of Path, and the last
	// element of DocURL.
	Slug string `json:"slug"`
	// Name is the title the documentation gives the API.
	Name string `json:"name"`
	// Category is the documentation's group.
	Category string `json:"category"`
	// Sphere is the documentation's sphere (Federal, Estadual, ...).
	Sphere string `json:"sphere"`
	// Target is the documentation's person type: PF, PJ or PF/PJ.
	Target string `json:"target"`
	// Origin is the documentation's origin: "Online" (the source is queried
	// when the API is called) or "Base" (Direct Data's own base).
	Origin string `json:"origin"`
	// Supplier is true when Origin is "Online": the query is forwarded to an
	// outside source at call time, whose availability Direct Data does not
	// control (terms clause 2.3.1).
	Supplier bool `json:"supplier"`
	// UpstreamURL is the source the documentation names for the data.
	UpstreamURL string `json:"upstream_url,omitempty"`
	// Method is the HTTP method. Every documented API is GET.
	Method string `json:"method"`
	// Path is the endpoint path, such as /api/CadastroPessoaFisica.
	Path string `json:"path"`
	// URLTemplate is the endpoint template exactly as documented.
	URLTemplate string `json:"url_template"`
	// PriceCents is the list price per query in BRL cents.
	PriceCents int64 `json:"price_cents"`
	// Price is the list price as documented, in BRL.
	Price float64 `json:"price_brl"`
	// Status is "available" or "maintenance" (isMaintenance), or "inactive"
	// (isActive false).
	Status string `json:"status"`
	// HasPDF is the documentation's hasPDF: a PDF receipt can be generated.
	HasPDF bool `json:"has_pdf"`
	// Description is the documentation's description of the API.
	Description string `json:"description"`
	// Params are the query parameters from the API's own page.
	Params []Param `json:"params"`
	// Fields pair the documented field labels with their descriptions.
	Fields []Field `json:"fields"`
	// CustomFields are the documentation's customProperties.
	CustomFields []Field `json:"custom_fields,omitempty"`
	// ResponseExample is the documented response schema example, verbatim.
	ResponseExample any `json:"response_example"`
	// ResponsePaths lists every leaf path of ResponseExample, with the JSON
	// type of the example value: "retorno.telefones[].operadora string".
	ResponsePaths []string `json:"response_paths"`
	// DocURL is the API's public documentation page.
	DocURL string `json:"doc_url"`
	// Incomplete is true when the page could not be read or lacks a part.
	Incomplete bool `json:"incomplete"`
	// IncompleteReasons says what is missing and why.
	IncompleteReasons []string `json:"incomplete_reasons,omitempty"`
	// Warnings note what was read but could not be used as documented, such
	// as field labels and descriptions whose counts differ.
	Warnings []string `json:"warnings,omitempty"`
}

// Param is one query parameter of an API.
type Param struct {
	// Name is the parameter as the page's parameter table spells it.
	Name string `json:"name"`
	// QueryName is the spelling the endpoint template uses, when the template
	// names the parameter; otherwise Name.
	QueryName string `json:"query_name"`
	// In is where the parameter goes: always "query".
	In string `json:"in"`
	// Type is the schema type ("string"), or "ref" with Ref set.
	Type string `json:"type"`
	// Ref is the schema reference name, such as OpcaoGerarComprovante.
	Ref string `json:"ref,omitempty"`
	// Description is the page's description, verbatim.
	Description string `json:"description"`
	// Required is the page's required flag. The pages omit it, which the
	// OpenAPI convention reads as false; the description may still say the
	// parameter must be given.
	Required bool `json:"required"`
	// InTemplate is true when the endpoint template names the parameter.
	InTemplate bool `json:"in_template"`
}

// Field is one documented field label and its description.
type Field struct {
	// Label is the documentation's friendly name.
	Label string `json:"label"`
	// Description is the documentation's description.
	Description string `json:"description,omitempty"`
}
