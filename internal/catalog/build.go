package catalog

import (
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"sort"
	"strings"
)

// Notes is what every catalogue records about how its fields were derived.
var Notes = []string{
	"Everything here was read from the public documentation pages; no API was called.",
	"price_cents is the documented list price (price_brl) times 100, rounded; a contract may price differently.",
	"supplier is derived from origin: \"Online\" means the source is queried when the API is called.",
	"status is \"inactive\" when isActive is false, \"maintenance\" when isMaintenance is true, otherwise \"available\".",
	"Parameter pages omit \"required\"; required=false follows the OpenAPI default, and a description may still say the parameter must be given.",
	"query_name is the parameter's spelling in the endpoint template; the API is documented with both spellings.",
	"response_example is the documented schema example (placeholder values such as \"string\" and 0), not a real response.",
	"fields pair the documented labels and descriptions by position when their counts agree; otherwise a warning says so.",
}

// IndexItems reads the API list from the catalogue index page.
func IndexItems(html []byte) ([]map[string]any, error) {
	f, err := ParseFlight(html)
	if err != nil {
		return nil, err
	}
	props, err := f.Element("items", "initialSearch")
	if err != nil {
		return nil, err
	}
	return objects(props["items"], "items")
}

// PageParams reads an API page: its item, as the page shows it, and its
// parameters.
func PageParams(html []byte) (item map[string]any, params []map[string]any, err error) {
	f, err := ParseFlight(html)
	if err != nil {
		return nil, nil, err
	}
	props, err := f.Element("item", "params")
	if err != nil {
		return nil, nil, err
	}
	item, _ = props["item"].(map[string]any)
	params, err = objects(props["params"], "params")
	return item, params, err
}

// objects converts a JSON array of objects.
func objects(v any, name string) ([]map[string]any, error) {
	arr, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("catalog: %s is not an array", name)
	}
	out := make([]map[string]any, 0, len(arr))
	for _, e := range arr {
		m, ok := e.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("catalog: an element of %s is not an object", name)
		}
		out = append(out, m)
	}
	return out, nil
}

// str returns m[k] as a string.
func str(m map[string]any, k string) string {
	s, _ := m[k].(string)
	return strings.TrimSpace(s)
}

// boolean returns m[k] as a bool.
func boolean(m map[string]any, k string) bool {
	b, _ := m[k].(bool)
	return b
}

// FromIndex builds an API entry from one index item. Parameters are filled
// later from the API's page.
func FromIndex(item map[string]any) API {
	a := API{
		Name:        str(item, "apiName"),
		Category:    str(item, "group"),
		Sphere:      str(item, "sphere"),
		Target:      str(item, "personType"),
		Origin:      str(item, "origin"),
		UpstreamURL: str(item, "sourceURL"),
		Method:      "GET",
		URLTemplate: str(item, "url"),
		HasPDF:      boolean(item, "hasPDF"),
		Description: str(item, "description"),
		Params:      []Param{},
		Fields:      []Field{},
	}
	a.Supplier = a.Origin == "Online"
	if u, err := url.Parse(a.URLTemplate); err == nil && strings.HasPrefix(u.Path, "/api/") {
		a.Path = u.Path
		a.Slug = strings.TrimPrefix(u.Path, "/api/")
	} else {
		a.incomplete("the endpoint template %q has no /api/ path", a.URLTemplate)
	}
	a.DocURL = PageURL(a.Slug)

	if price, ok := item["price"].(float64); ok && price > 0 {
		a.Price = price
		a.PriceCents = int64(math.Round(price * 100))
	} else {
		a.incomplete("the index gives no price")
	}
	switch {
	case !boolean(item, "isActive"):
		a.Status = "inactive"
	case boolean(item, "isMaintenance"):
		a.Status = "maintenance"
	default:
		a.Status = "available"
	}

	if props, err := objects(item["properties"], "properties"); err == nil {
		for _, p := range props {
			a.Fields = append(a.Fields, a.pairFields(str(p, "friendlyName"), str(p, "description"))...)
		}
	}
	if custom, err := objects(item["customProperties"], "customProperties"); err == nil {
		for _, p := range custom {
			a.CustomFields = append(a.CustomFields, Field{Label: str(p, "friendlyName"), Description: str(p, "description")})
		}
	}
	if ex, ok := item["jsonExample"]; ok && ex != nil {
		a.ResponseExample = ex
		a.ResponsePaths = LeafPaths(ex)
	} else {
		a.incomplete("the documentation has no response example")
		a.ResponsePaths = []string{}
	}
	return a
}

// pairFields splits the "; "-separated label and description lists and pairs
// them by position when their counts agree.
func (a *API) pairFields(labels, descriptions string) []Field {
	ls := splitList(labels)
	ds := splitList(descriptions)
	out := make([]Field, 0, len(ls))
	paired := len(ls) == len(ds)
	if !paired && descriptions != "" {
		a.Warnings = append(a.Warnings, fmt.Sprintf("fields: %d labels and %d descriptions, so descriptions are not paired", len(ls), len(ds)))
	}
	for i, l := range ls {
		f := Field{Label: l}
		if paired {
			f.Description = ds[i]
		}
		out = append(out, f)
	}
	return out
}

// splitList splits a "; "-separated documentation list.
func splitList(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ";")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// ApplyParams fills the parameters read from the API's page.
func (a *API) ApplyParams(params []map[string]any) {
	template := templateParams(a.URLTemplate)
	a.Params = []Param{}
	for _, p := range params {
		name := str(p, "name")
		pr := Param{Name: name, QueryName: name, In: str(p, "in"), Description: str(p, "description"), Required: boolean(p, "required")}
		if q, ok := template[strings.ToLower(name)]; ok {
			pr.QueryName = q
			pr.InTemplate = true
		}
		if schema, ok := p["schema"].(map[string]any); ok {
			if ref := str(schema, "$ref"); ref != "" {
				pr.Type = "ref"
				pr.Ref = ref[strings.LastIndexByte(ref, '/')+1:]
			} else {
				pr.Type = str(schema, "type")
			}
		}
		a.Params = append(a.Params, pr)
	}
	if len(a.Params) == 0 {
		a.incomplete("the documentation page lists no parameter")
	}
	for lower, q := range template {
		found := false
		for _, p := range a.Params {
			if strings.ToLower(p.Name) == lower {
				found = true
			}
		}
		if !found {
			a.Warnings = append(a.Warnings, fmt.Sprintf("the endpoint template names %s, which the parameter table does not", q))
		}
	}
}

// incomplete marks the entry incomplete with a reason.
func (a *API) incomplete(format string, args ...any) {
	a.Incomplete = true
	a.IncompleteReasons = append(a.IncompleteReasons, fmt.Sprintf(format, args...))
}

// MarkIncomplete records that part of the entry could not be read.
func (a *API) MarkIncomplete(reason string) { a.incomplete("%s", reason) }

// templateParams maps the lower-cased name of every query parameter in an
// endpoint template to its spelling there.
func templateParams(template string) map[string]string {
	out := map[string]string{}
	q := template
	if i := strings.IndexByte(q, '?'); i >= 0 {
		q = q[i+1:]
	} else {
		return out
	}
	for _, pair := range strings.Split(q, "&") {
		name, _, _ := strings.Cut(pair, "=")
		if name != "" {
			out[strings.ToLower(name)] = name
		}
	}
	return out
}

// LeafPaths lists every leaf of a JSON value as "path type", arrays written
// with [].
func LeafPaths(v any) []string {
	var out []string
	var walk func(prefix string, v any)
	walk = func(prefix string, v any) {
		switch v := v.(type) {
		case map[string]any:
			if len(v) == 0 {
				out = append(out, prefix+" object")
				return
			}
			keys := make([]string, 0, len(v))
			for k := range v {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				p := k
				if prefix != "" {
					p = prefix + "." + k
				}
				walk(p, v[k])
			}
		case []any:
			if len(v) == 0 {
				out = append(out, prefix+"[] unknown")
				return
			}
			walk(prefix+"[]", v[0])
		case string:
			out = append(out, prefix+" string")
		case float64, json.Number:
			out = append(out, prefix+" number")
		case bool:
			out = append(out, prefix+" boolean")
		case nil:
			out = append(out, prefix+" null")
		}
	}
	walk("", v)
	if out == nil {
		out = []string{}
	}
	return out
}

// Assemble sorts the entries and fills the catalogue's header.
func Assemble(apis []API, crawledAt string) Catalog {
	sort.Slice(apis, func(i, j int) bool {
		if apis[i].Category != apis[j].Category {
			return apis[i].Category < apis[j].Category
		}
		return apis[i].Slug < apis[j].Slug
	})
	cats := map[string]int{}
	for _, a := range apis {
		cats[a.Category]++
	}
	return Catalog{
		SchemaVersion: SchemaVersion,
		CrawledAt:     crawledAt,
		Source:        IndexURL,
		Notes:         Notes,
		Count:         len(apis),
		Categories:    cats,
		APIs:          apis,
	}
}
