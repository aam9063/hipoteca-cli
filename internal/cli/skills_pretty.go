package cli

import (
	"strings"

	"github.com/aam9063/hipoteca-cli/internal/skills"
)

// prettyWidth es el ancho al que se envuelve la descripción. No se lee del
// terminal a propósito: el resultado tiene que ser el mismo en cualquier
// máquina y en los tests, sin simular un TTY.
const prettyWidth = 72

// renderSkillsTable devuelve la tabla de recetas: id a la izquierda, resumen a
// la derecha. Es la vista para humanos de lo mismo que `skills list` publica en
// JSON (D-01 del arranque: `pretty` en español).
func renderSkillsTable(list []skills.Summary) string {
	if len(list) == 0 {
		return "No hay ninguna receta disponible.\n"
	}

	const (
		headerID    = "RECETA"
		headerTitle = "QUÉ RESUELVE"
	)
	idWidth := len(headerID)
	for _, s := range list {
		if len(s.ID) > idWidth {
			idWidth = len(s.ID)
		}
	}
	const gap = 3

	var b strings.Builder
	b.WriteString("  " + padRight(headerID, idWidth) + strings.Repeat(" ", gap) + headerTitle + "\n")
	for _, s := range list {
		lines := wrapLines(s.Summary, prettyWidth-idWidth-gap)
		if len(lines) == 0 {
			lines = []string{""}
		}
		for i, line := range lines {
			b.WriteString("  ")
			if i == 0 {
				b.WriteString(padRight(s.ID, idWidth))
			} else {
				b.WriteString(strings.Repeat(" ", idWidth))
			}
			b.WriteString(strings.Repeat(" ", gap))
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	return b.String()
}

// padRight rellena con espacios hasta el ancho indicado. Cuenta runas, no
// bytes: los ids son ASCII, pero un acento en el futuro no debe descuadrar la
// tabla.
func padRight(s string, width int) string {
	n := len([]rune(s))
	if n >= width {
		return s
	}
	return s + strings.Repeat(" ", width-n)
}

// wrapLines parte texto en líneas de como mucho width runas, cortando por
// palabras. Una palabra más larga que width queda sola en su línea en vez de
// partirse: no hay guiones que insertar sin inventar.
func wrapLines(text string, width int) []string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return nil
	}
	if width < 1 {
		width = 1
	}
	var lines []string
	current := words[0]
	for _, w := range words[1:] {
		if len([]rune(current))+1+len([]rune(w)) <= width {
			current += " " + w
			continue
		}
		lines = append(lines, current)
		current = w
	}
	return append(lines, current)
}
