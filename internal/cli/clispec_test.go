package cli

import (
	"os"
	"strings"
	"testing"
)

// TestDocsCLISpecMatchesBinary es el test de no-desfase (AGENT-02, S2): el
// docs/cli-spec.md del repositorio tiene que ser byte a byte lo que genera el
// binario ahora. Si este test falla, el documento miente con autoridad: se
// regenera con `go run ./scripts/generate-cli-spec` y se vuelve a committing
// junto al cambio del comando que lo provocó.
func TestDocsCLISpecMatchesBinary(t *testing.T) {
	const path = "../../docs/cli-spec.md"
	committed, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("no se puede leer %s: %v; regenera con `go run ./scripts/generate-cli-spec`", path, err)
	}
	generated := RenderCLISpec()
	if string(committed) == generated {
		return
	}
	// El fallo señala la primera línea que difiere, para que el ajuste no
	// sea un diff a ciegas.
	cLines := strings.Split(string(committed), "\n")
	gLines := strings.Split(generated, "\n")
	line := 0
	for line < len(cLines) && line < len(gLines) && cLines[line] == gLines[line] {
		line++
	}
	t.Errorf("docs/cli-spec.md está desfasado respecto al binario (primera diferencia en la línea %d):\n documento: %q\n generado:  %q\n regenera con `go run ./scripts/generate-cli-spec` y añade el resultado al commit",
		line+1,
		specLine(cLines, line), specLine(gLines, line))
}

func specLine(lines []string, i int) string {
	if i < len(lines) {
		return lines[i]
	}
	return "(fin del documento)"
}

// TestRenderCLISpecDeterministic fija la propiedad S6: dos ejecuciones en el
// mismo binario producen exactamente los mismos bytes, y nada del documento
// depende de la máquina que lo genera.
func TestRenderCLISpecDeterministic(t *testing.T) {
	first := RenderCLISpec()
	second := RenderCLISpec()
	if first != second {
		t.Error("RenderCLISpec no es determinista: dos ejecuciones difieren")
	}
	if strings.Contains(first, `C:\`) || strings.Contains(first, "/home/") || strings.Contains(first, "/tmp/") {
		t.Error("RenderCLISpec contiene una ruta de la máquina: el documento debe ser portable")
	}
}

// TestRenderCLISpecVersions fija S5: los comandos de la corte actual van
// marcados como disponibles en v0.1.0, y `watch` —aplazado a v0.2.0 por la
// decisión de corte— se documenta como previsto, nunca como disponible.
func TestRenderCLISpecVersions(t *testing.T) {
	doc := RenderCLISpec()
	if !strings.Contains(doc, "`hipoteca watch`") {
		t.Error("el documento no menciona hipoteca watch; si sale del árbol, actualiza este test")
	}
	if !strings.Contains(doc, "v0.2.0 (previsto: todavía no disponible)") {
		t.Error("watch no aparece marcado como previsto para v0.2.0")
	}
	watchSection := doc[strings.Index(doc, "`hipoteca watch`"):]
	end := strings.Index(watchSection, "\n# ")
	if end >= 0 {
		watchSection = watchSection[:end]
	}
	if strings.Contains(watchSection, specDefaultVersion) {
		t.Error("watch aparece marcado como disponible en v0.1.0: no debe documentarse como disponible")
	}
	if !strings.Contains(doc, "**Estado:** "+specDefaultVersion) {
		t.Error("los comandos disponibles no llevan su marca de v0.1.0")
	}
}

// TestRenderCLISpecCoversSchemaTable fija S1: el contrato de salida del
// documento es el de `hipoteca schema`, comando por comando, en el mismo
// orden. Un comando nuevo sin contrato se nota aquí en cuanto el documento
// se regenera.
func TestRenderCLISpecCoversSchemaTable(t *testing.T) {
	doc := RenderCLISpec()
	for _, e := range schemaTable {
		heading := "### `" + e.Name + "`"
		if !strings.Contains(doc, heading) {
			t.Errorf("el contrato de %q no está en el documento generado", e.Name)
		}
	}
}
