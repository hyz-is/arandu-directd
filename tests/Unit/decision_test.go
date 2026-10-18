package unit_test

import (
	"go/ast"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	directd "github.com/hyz-is/arandu-directd"
)

// verdict is the vocabulary of a decision. The package returns evidence --
// what was asked, what the source said, what it cost -- and the decision
// belongs to the application's policy. A name from this list on an exported
// function, type or field would put a verdict where only evidence goes.
var verdict = regexp.MustCompile(`(?i)(approv|reject|decision|decide|verdict|accept|denied|deny|fraud|passed|eligib|trusted|isvalid|isregular|islegit)`)

// TestNothingHereDecides walks the hand-written source. The generated files
// are left out on purpose: their names are Direct Data's own keys and slugs
// (riscoFraude, isRegular, AntifraudePix), which are the source's data and
// travel as evidence like any other field; TestTheGeneratedFilesAreCurrent
// holds that every one of them comes from the catalogue.
func TestNothingHereDecides(t *testing.T) {
	t.Parallel()
	for _, source := range productionGoFiles(t, packageRoot(t)) {
		if strings.HasPrefix(filepath.Base(source.path), "zz_") {
			continue
		}
		ast.Inspect(source.file, func(node ast.Node) bool {
			switch node := node.(type) {
			case *ast.FuncDecl:
				if node.Name.IsExported() && verdict.MatchString(node.Name.Name) {
					t.Errorf("%s declares %s, a name that decides", source.path, node.Name.Name)
				}
			case *ast.TypeSpec:
				if node.Name.IsExported() && verdict.MatchString(node.Name.Name) {
					t.Errorf("%s declares the type %s, a name that decides", source.path, node.Name.Name)
				}
				if st, ok := node.Type.(*ast.StructType); ok {
					for _, field := range st.Fields.List {
						for _, name := range field.Names {
							if name.IsExported() && verdict.MatchString(name.Name) {
								t.Errorf("%s: %s.%s is a verdict field", source.path, node.Name.Name, name.Name)
							}
						}
					}
				}
			}
			return true
		})
	}
}

// TestTheResultCarriesNoVerdict walks the evidence types by reflection. The
// only booleans a Result may hold are facts about the call itself.
func TestTheResultCarriesNoVerdict(t *testing.T) {
	t.Parallel()
	allowed := map[string]bool{
		"Result.Cached": true, "Result.Async": true,
		"Result.Meta.EnviarCallback": true, "Result.Meta.GerarComprovante": true, "Result.Meta.Assincrono": true,
	}
	var walk func(path string, typ reflect.Type, seen map[reflect.Type]bool)
	walk = func(path string, typ reflect.Type, seen map[reflect.Type]bool) {
		switch typ.Kind() {
		case reflect.Bool:
			if !allowed[path] {
				t.Errorf("%s is a boolean, and a boolean on a result reads as a verdict", path)
			}
		case reflect.Pointer, reflect.Slice, reflect.Array:
			walk(path+"[]", typ.Elem(), seen)
		case reflect.Map:
			walk(path+"[key]", typ.Key(), seen)
			walk(path+"[]", typ.Elem(), seen)
		case reflect.Struct:
			if seen[typ] || typ.PkgPath() != reflect.TypeOf(directd.Result{}).PkgPath() {
				return
			}
			seen[typ] = true
			for i := 0; i < typ.NumField(); i++ {
				f := typ.Field(i)
				if f.IsExported() && verdict.MatchString(f.Name) {
					t.Errorf("%s.%s is a verdict field", path, f.Name)
				}
				walk(path+"."+f.Name, f.Type, seen)
			}
		}
	}
	walk("Result", reflect.TypeOf(directd.Result{}), map[reflect.Type]bool{})
	walk("Ticket", reflect.TypeOf(directd.Ticket{}), map[reflect.Type]bool{reflect.TypeOf(directd.Result{}): true})
}
