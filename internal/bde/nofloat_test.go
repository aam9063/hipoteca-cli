package bde

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// TestNoFloatInBDE es la guarda anti-float de BDE-01 (P-07, decisión 1): ningún
// float entra en los ficheros no-test de internal/bde. Es la misma guarda de
// internal/finance aplicada al paquete que recibe los números JSON del Banco:
// un float64 aquí rompería P-07 en el borde mismo del sistema, antes de
// cualquier cálculo. Se vigilan los identificadores float64/float32, los
// literales float (cómo entra un 1.5 suelto) y los selectores cuya API delata
// un paso por float (NewFromFloat, ParseFloat, Float64frombits, Value.Float...).
func TestNoFloatInBDE(t *testing.T) {
	paths, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob *.go: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no se ha encontrado ningún fichero .go en internal/bde")
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
					t.Errorf("%s: aparece el identificador %s (P-07: nada de float en internal/bde)",
						fset.Position(node.Pos()), node.Name)
				}
			case *ast.BasicLit:
				if node.Kind == token.FLOAT {
					t.Errorf("%s: literal float %s (P-07: nada de float en internal/bde)",
						fset.Position(node.Pos()), node.Value)
				}
			case *ast.SelectorExpr:
				name := node.Sel.Name
				if strings.Contains(name, "Float") || strings.Contains(name, "ParseFloat") {
					t.Errorf("%s: selector %s (P-07: nada de float en internal/bde)",
						fset.Position(node.Pos()), name)
				}
			}
			return true
		})
	}
}
