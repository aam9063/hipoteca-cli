// Command generate-cli-spec regenera docs/cli-spec.md desde el binario
// (AGENT-02, S1/S6). Es el único camino de escritura del documento: el árbol
// de comandos, las banderas y el contrato de salida salen del mismo código
// que ejecuta la CLI, y el test de no-desfase de internal/cli compara el
// resultado con lo que está en el repositorio.
//
// Uso:
//
//	go run ./scripts/generate-cli-spec            # escribe docs/cli-spec.md del repositorio
//	go run ./scripts/generate-cli-spec ruta.md    # escribe en otra ruta (para pruebas)
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/aam9063/hipoteca-cli/internal/cli"
)

func main() {
	target := defaultTarget()
	if len(os.Args) == 2 {
		target = os.Args[1]
	} else if len(os.Args) > 2 {
		fmt.Fprintln(os.Stderr, "uso: generate-cli-spec [ruta-de-salida]")
		os.Exit(2)
	}
	content := []byte(cli.RenderCLISpec())
	if err := os.WriteFile(target, content, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "escribiendo %s: %v\n", target, err)
		os.Exit(1)
	}
	fmt.Printf("generado %s (%d bytes)\n", target, len(content))
}

// defaultTarget resuelve docs/cli-spec.md del repositorio a partir de la
// posición de este fichero, no del directorio en curso: da igual desde dónde
// se invoque. La ruta vive aquí, nunca dentro del documento generado, que
// debe ser idéntico en las tres plataformas (S6).
func defaultTarget() string {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		fmt.Fprintln(os.Stderr, "no se ha podido resolver la raíz del repositorio; pasa la ruta de salida como argumento")
		os.Exit(1)
	}
	root := filepath.Dir(filepath.Dir(filepath.Dir(thisFile)))
	return filepath.Join(root, "docs", "cli-spec.md")
}
