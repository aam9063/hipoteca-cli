package app

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// TestNoRawDecimalFields extiende a internal/app la guarda de CORE-01 (decisión
// 1 de CMD-01): este paquete construye los structs de salida, así que ningún
// campo de struct puede declararse como decimal.Decimal directamente — los
// structs de salida usan model.Decimal, cuyo MarshalJSON conserva la escala.
// Sin esto, un decimal.Decimal colaría en el contrato y perdería los ceros al
// serializarse.
func TestNoRawDecimalFields(t *testing.T) {
	paths, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob *.go: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no se ha encontrado ningún fichero .go en internal/app")
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
					if isShopspringDecimal(field.Type) {
						t.Errorf("%s: el campo %s de %s es decimal.Decimal: usa model.Decimal (su MarshalJSON conserva la escala)",
							fset.Position(field.Pos()), fieldName(field), ts.Name.Name)
					}
				}
			}
		}
	}
}

// isShopspringDecimal reports whether expr is the type decimal.Decimal
// (including a pointer to it).
func isShopspringDecimal(expr ast.Expr) bool {
	switch e := expr.(type) {
	case *ast.SelectorExpr:
		x, ok := e.X.(*ast.Ident)
		return ok && x.Name == "decimal" && e.Sel.Name == "Decimal"
	case *ast.StarExpr:
		return isShopspringDecimal(e.X)
	}
	return false
}

func fieldName(field *ast.Field) string {
	if len(field.Names) > 0 {
		return field.Names[0].Name
	}
	return "<sin nombre>"
}
