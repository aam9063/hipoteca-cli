package cli

import (
	"strings"

	"github.com/aam9063/hipoteca-cli/internal/skills"
)

// prettyWidth, padRight y wrapLines viven en pretty.go: un solo sitio para
// el ancho fijo y los primitivos de las tablas --pretty (T1 de PRETTY-01).

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

// padRight y wrapLines viven en pretty.go, compartidas por todas las tablas
// --pretty.
