package finance

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// TestNoFloatInFinance es la guarda anti-float de OPS-02 (P-07): ningún float
// entra en los ficheros no-test de internal/finance. La versión mínima de
// FIN-01.6 sólo miraba los identificadores float64/float32 y el verificador
// independiente la burló con decimal.NewFromFloat, strconv.ParseFloat y
// reflect.Value.Float; por eso ahora también se vigilan los literales float
// (cómo entra un 1.5 suelto) y los selectores cuyo nombre delata una API de
// float (NewFromFloat, ParseFloat, Float64frombits, Value.Float, ...). Una
// conversión a float32/float64 sigue cubierta por la regla de identificadores.
// El propio test vive en un _test.go, así que sus literales no se autoinculpan.
func TestNoFloatInFinance(t *testing.T) {
	paths, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob *.go: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no se ha encontrado ningún fichero .go en internal/finance")
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
		ast.Inspect(file, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.Ident:
				if node.Name == "float64" || node.Name == "float32" {
					t.Errorf("%s: aparece el identificador %s (P-07: nada de float en internal/finance)",
						fset.Position(node.Pos()), node.Name)
				}
			case *ast.BasicLit:
				if node.Kind == token.FLOAT {
					t.Errorf("%s: literal float %s (P-07: nada de float en internal/finance)",
						fset.Position(node.Pos()), node.Value)
				}
			case *ast.SelectorExpr:
				name := node.Sel.Name
				if strings.Contains(name, "Float") || strings.Contains(name, "ParseFloat") {
					t.Errorf("%s: selector %s (P-07: nada de float en internal/finance)",
						fset.Position(node.Pos()), name)
				}
			}
			return true
		})
	}
}
