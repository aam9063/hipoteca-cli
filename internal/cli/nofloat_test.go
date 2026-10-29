package cli

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// TestNoRawDecimalFields extiende a internal/cli la guarda de CORE-01
// (decisión 1 de CMD-01): la CLI también declara structs de salida, así que
// ningún campo puede declararse como decimal.Decimal — los structs de salida
// usan model.Decimal, cuyo MarshalJSON conserva la escala.
func TestNoRawDecimalFields(t *testing.T) {
	paths, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob *.go: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no se ha encontrado ningún fichero .go en internal/cli")
	}
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parsear %s: %v", path, err)
		}
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.TYPE {
				continue
			}
			for _, spec := range gen.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				st, ok := ts.Type.(*ast.StructType)
				if !ok {
					continue
				}
				for _, field := range st.Fields.List {
					if isShopspringDecimalCLI(file, field.Type) {
						t.Errorf("%s: el campo %s de %s es decimal.Decimal: usa model.Decimal (su MarshalJSON conserva la escala)",
							fset.Position(field.Pos()), fieldNameCLI(field), ts.Name.Name)
					}
				}
			}
		}
	}
}

// TestGuardaDetectaDecimalConAliasYColeccionesCLI fija la cobertura real de la
// guarda en internal/cli (ronda de corrección de CMD-01): un campo declarado
// con un alias de importación (shdec.Decimal) o dentro de un slice, un mapa o
// un puntero antes pasaba inadvertido. Los casos con decimal.Decimal deben
// detectarse; los benignos, no.
func TestGuardaDetectaDecimalConAliasYColeccionesCLI(t *testing.T) {
	cases := []struct {
		name string
		decl string
		want bool
	}{
		{"nombre directo", "type S struct { V decimal.Decimal }", true},
		{"puntero directo", "type S struct { V *decimal.Decimal }", true},
		{"alias de importación", "type S struct { V shdec.Decimal }", true},
		{"puntero con alias", "type S struct { V *shdec.Decimal }", true},
		{"slice", "type S struct { V []decimal.Decimal }", true},
		{"mapa", "type S struct { V map[string]decimal.Decimal }", true},
		{"tipo benigno", "type S struct { V string }", false},
		{"time", "type S struct { V time.Time }", false},
		{"otro tipo del mismo paquete", "type S struct { V shdec.Money }", false},
		{"slice benigno", "type S struct { V []string }", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := snippetDetectaDecimalCLI(t, tc.decl); got != tc.want {
				t.Errorf("la guarda devolvió %v, se esperaba %v", got, tc.want)
			}
		})
	}
}

// snippetDetectaDecimalCLI monta un fichero de prueba con las importaciones
// pertinentes (sin alias y con el alias shdec) y aplica la guarda al primer
// campo del struct declarado.
func snippetDetectaDecimalCLI(t *testing.T, decl string) bool {
	t.Helper()
	src := "package p\n" +
		"import \"github.com/shopspring/decimal\"\n" +
		"import shdec \"github.com/shopspring/decimal\"\n" +
		"import \"time\"\n\n" + decl + "\n"
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "snippet.go", src, 0)
	if err != nil {
		t.Fatalf("parsear el fragmento: %v", err)
	}
	for _, d := range file.Decls {
		gen, ok := d.(*ast.GenDecl)
		if !ok || gen.Tok != token.TYPE {
			continue
		}
		for _, spec := range gen.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			if st, ok := ts.Type.(*ast.StructType); ok {
				return isShopspringDecimalCLI(file, st.Fields.List[0].Type)
			}
		}
	}
	t.Fatal("el fragmento no declara ningún struct")
	return false
}

// isShopspringDecimalCLI reports whether expr is the type decimal.Decimal
// (including through a pointer, a slice or a map key/element), resolving the
// selector's package identifier through the file's imports so that an import
// alias (shdec.Decimal) cannot evade the guard.
func isShopspringDecimalCLI(file *ast.File, expr ast.Expr) bool {
	switch e := expr.(type) {
	case *ast.SelectorExpr:
		x, ok := e.X.(*ast.Ident)
		return ok && e.Sel.Name == "Decimal" && importaShopspringDecimalCLI(file, x.Name)
	case *ast.StarExpr:
		return isShopspringDecimalCLI(file, e.X)
	case *ast.ArrayType:
		return isShopspringDecimalCLI(file, e.Elt)
	case *ast.MapType:
		return isShopspringDecimalCLI(file, e.Key) || isShopspringDecimalCLI(file, e.Value)
	}
	return false
}

// importaShopspringDecimalCLI reports whether the identifier pkg names the
// shopspring decimal package in this file: either the default name `decimal`
// for a plain import, or the alias given to the import. Revisa todas las
// importaciones del path: un fichero puede importarlo dos veces (sin alias y
// con alias), y la primera coincidencia no puede cortar la búsqueda.
func importaShopspringDecimalCLI(file *ast.File, pkg string) bool {
	for _, imp := range file.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		if path != "github.com/shopspring/decimal" {
			continue
		}
		name := "decimal"
		if imp.Name != nil {
			name = imp.Name.Name
		}
		if name == pkg {
			return true
		}
	}
	return false
}

func fieldNameCLI(field *ast.Field) string {
	if len(field.Names) > 0 {
		return field.Names[0].Name
	}
	return "<sin nombre>"
}
