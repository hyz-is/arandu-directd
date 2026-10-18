package directd

import (
	"sort"
	"strings"
)

// ProductStatus is a product's availability as the documentation states it.
type ProductStatus string

// The product statuses.
const (
	// StatusAvailable is a product the documentation lists as available.
	StatusAvailable ProductStatus = "available"
	// StatusMaintenance is a product the documentation lists as under
	// maintenance ("Em manutenção"). It may still be called; it may fail.
	StatusMaintenance ProductStatus = "maintenance"
	// StatusInactive is a product the documentation lists as inactive. The
	// client refuses it.
	StatusInactive ProductStatus = "inactive"
)

// ParamRole is what a parameter carries, which decides how the client fills
// and checks it.
type ParamRole string

// The parameter roles.
const (
	// RoleCPF carries a CPF. The client fills it from Request.Document after
	// checking the check digits.
	RoleCPF ParamRole = "cpf"
	// RoleCNPJ carries a CNPJ, numeric or alphanumeric. The client fills it
	// from Request.Document after checking the check digits.
	RoleCNPJ ParamRole = "cnpj"
	// RoleDocument carries a CPF or a CNPJ in one parameter.
	RoleDocument ParamRole = "cpf_or_cnpj"
	// RoleReceipt asks for the PDF receipt (GERARCOMPROVANTE). The
	// documentation names its schema but not its values, so the client sends
	// only what Request.Params gives it.
	RoleReceipt ParamRole = "receipt"
	// RoleInput is any other input, sent as Request.Params gives it.
	RoleInput ParamRole = "input"
)

// Param is one query parameter of a product.
type Param struct {
	// Name is the parameter as the request spells it.
	Name string
	// Description is the documentation's description.
	Description string
	// Role is what the parameter carries.
	Role ParamRole
}

// Product is one Direct Data API: where it is, what it takes and what it
// costs. DefaultProducts holds every API of the public catalogue; Config.
// Products overrides or adds entries by slug.
type Product struct {
	// Slug is the endpoint name, such as CadastroPessoaFisicaPlus.
	Slug string
	// Name is the documentation's title.
	Name string
	// Category is the documentation's group.
	Category string
	// Target is the documentation's person type: PF, PJ or PF/PJ.
	Target string
	// Path is the endpoint path, such as /api/CadastroPessoaFisicaPlus.
	Path string
	// Params are the documented query parameters, without TOKEN and ASync,
	// which the client adds itself.
	Params []Param
	// PriceCents is the list price per query in BRL cents. A contract may
	// price differently; override it in Config.Products.
	PriceCents int64
	// Supplier is true when the documentation's origin is "Online": the
	// source is queried when the API is called (terms clause 2.3.1).
	Supplier bool
	// Status is the documentation's availability.
	Status ProductStatus
	// HasPDF is true when the documentation offers a PDF receipt.
	HasPDF bool
	// DocURL is the API's public documentation page.
	DocURL string
	// Disabled refuses the product. The application sets it for every API
	// its contract or purpose does not cover.
	Disabled bool
}

// param returns the parameter with the given name, ignoring case.
func (p Product) param(name string) (Param, bool) {
	for _, q := range p.Params {
		if strings.EqualFold(q.Name, name) {
			return q, true
		}
	}
	return Param{}, false
}

// byRole returns the first parameter with the role.
func (p Product) byRole(r ParamRole) (Param, bool) {
	for _, q := range p.Params {
		if q.Role == r {
			return q, true
		}
	}
	return Param{}, false
}

// TakesCPF reports whether the product takes a CPF as its document.
func (p Product) TakesCPF() bool {
	_, a := p.byRole(RoleCPF)
	_, b := p.byRole(RoleDocument)
	return a || b
}

// TakesCNPJ reports whether the product takes a CNPJ as its document.
func (p Product) TakesCNPJ() bool {
	_, a := p.byRole(RoleCNPJ)
	_, b := p.byRole(RoleDocument)
	return a || b
}

// clone copies the product so that no caller shares its Params.
func (p Product) clone() Product {
	p.Params = append([]Param(nil), p.Params...)
	return p
}

// roleOf infers a parameter's role from its documented name.
func roleOf(name string) ParamRole {
	switch strings.ToLower(name) {
	case "cpf":
		return RoleCPF
	case "cnpj":
		return RoleCNPJ
	case "documento", "cpfcnpj", "cpf_cnpj", "cpfoucnpj", "doc":
		return RoleDocument
	case "gerarcomprovante":
		return RoleReceipt
	}
	return RoleInput
}

// DefaultProducts returns every API of the public catalogue, by slug, read
// from catalog/catalog.json when the package was generated. The map and
// every Product in it are copies.
func DefaultProducts() map[string]Product {
	out := make(map[string]Product, len(catalogProducts))
	for _, p := range catalogProducts {
		out[p.Slug] = p.clone()
	}
	return out
}

// ProductSlugs returns the slugs of DefaultProducts, sorted.
func ProductSlugs() []string {
	out := make([]string, 0, len(catalogProducts))
	for _, p := range catalogProducts {
		out = append(out, p.Slug)
	}
	sort.Strings(out)
	return out
}
