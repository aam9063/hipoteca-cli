package finance

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// TestNoFloatInFinance es el adelanto mínimo de OPS-02 acordado en FIN-01.6:
// ningún float64 ni float32 en los ficheros no-test de internal/finance (P-07).
// No es el linter de OPS-02; sólo un test de AST sobre este paquete.
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
			id, ok := n.(*ast.Ident)
			if ok && (id.Name == "float64" || id.Name == "float32") {
				t.Errorf("%s: aparece %s (P-07: nada de float en internal/finance)",
					fset.Position(id.Pos()), id.Name)
			}
			return true
		})
	}
}
