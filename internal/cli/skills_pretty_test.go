package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/aam9063/hipoteca-cli/internal/skills"
)

// TestSkillsListPrettyIsATable: --pretty es la vista para humanos, con la
// receta a la izquierda y su resumen a la derecha.
func TestSkillsListPrettyIsATable(t *testing.T) {
	cmd := newSkillsCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"list", "--pretty"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("skills list --pretty: %v", err)
	}

	got := out.String()
	if strings.HasPrefix(strings.TrimSpace(got), "{") {
		t.Fatalf("--pretty no debe devolver JSON:\n%s", got)
	}
	if !strings.Contains(got, "RECETA") || !strings.Contains(got, "QUÉ RESUELVE") {
		t.Fatalf("faltan las cabeceras de la tabla:\n%s", got)
	}
	// Cada receta aparece en su propia línea, alineada a la izquierda.
	for _, s := range skills.List() {
		found := false
		for _, line := range strings.Split(got, "\n") {
			if strings.HasPrefix(line, "  "+s.ID) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("la receta %q no encabeza ninguna línea:\n%s", s.ID, got)
		}
	}
	// Ninguna línea se pasa del ancho fijado: la tabla no depende del terminal.
	for _, line := range strings.Split(strings.TrimRight(got, "\n"), "\n") {
		if n := len([]rune(line)); n > prettyWidth+2 {
			t.Fatalf("línea de %d caracteres, el máximo es %d: %q", n, prettyWidth+2, line)
		}
	}
}

// TestSkillsListDefaultStaysJSON: sin --pretty el contrato no cambia. Es el
// valor que consumen los agentes, y el que documenta `hipoteca schema`.
func TestSkillsListDefaultStaysJSON(t *testing.T) {
	cmd := newSkillsCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"list"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("skills list: %v", err)
	}
	var decoded skillsListOutput
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("la salida por defecto debe ser un único valor JSON: %v\n%s", err, out.String())
	}
	if len(decoded.Skills) == 0 {
		t.Fatal("el JSON debe traer las recetas")
	}
}

// TestWrapLinesNoRompeUnId: parte por palabras y una palabra más larga que el
// ancho queda entera en su línea en vez de trocearse.
func TestWrapLinesNoRompeUnId(t *testing.T) {
	// Ancho 10: "corto palabras" no cabe en una línea y se parte en dos,
	// cada una dentro del límite.
	lines := wrapLines("corto palabras", 10)
	if len(lines) != 2 {
		t.Fatalf("se esperaban dos líneas, salieron %d: %q", len(lines), lines)
	}
	for _, l := range lines {
		if len([]rune(l)) > 10 {
			t.Fatalf("línea demasiado larga: %q", l)
		}
	}
	// Una palabra más ancha que el límite queda entera, sola en su línea: no
	// hay guiones que insertar sin inventar dónde va la sílaba.
	long := "extraordinariamentelarga"
	lines = wrapLines("x "+long, 10)
	if lines[len(lines)-1] != long {
		t.Fatalf("la palabra larga debe quedar entera, quedó %q", lines)
	}
}
