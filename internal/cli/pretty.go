package cli

import (
	"strings"

	"github.com/aam9063/hipoteca-cli/internal/model"
)

// prettyWidth es el ancho fijo de las vistas --pretty. No se lee del terminal
// a propósito: el resultado tiene que ser el mismo en cualquier máquina y en
// los tests, sin simular un TTY (S3 de PRETTY-01, D-01 del arranque).
const prettyWidth = 72

// prettyIndent es la sangría común de todas las líneas de una tabla --pretty.
const prettyIndent = "  "

// prettyGap es el espacio entre columnas de una tabla --pretty.
const prettyGap = 2

// prettyCol describe una columna de una tabla --pretty: cabecera en
// mayúsculas y ancho fijo. Las columnas de texto libre (wrap) envuelven con
// wrapLines sin partir palabras; el resto trunca con una elipsis si la celda
// no cabe, en vez de empujar la columna (S6 de PRETTY-01).
type prettyCol struct {
	header string
	width  int
	wrap   bool
}

// renderPrettyTable dibuja una tabla --pretty: cabecera en mayúsculas,
// columnas alineadas a la izquierda (S1) y, debajo, las líneas de resumen que
// hacen la tabla interpretable por sí sola (S5). Devuelve el bloque con un
// salto final.
func renderPrettyTable(cols []prettyCol, rows [][]string, summaries []string) string {
	var b strings.Builder
	b.WriteString(prettyIndent)
	for i, c := range cols {
		if i > 0 {
			b.WriteString(strings.Repeat(" ", prettyGap))
		}
		b.WriteString(padRight(truncateCell(c.header, c.width), c.width))
	}
	b.WriteString("\n")
	for _, row := range rows {
		for _, line := range prettyRowLines(cols, row) {
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	for _, s := range summaries {
		for _, l := range wrapLines(s, prettyWidth-len(prettyIndent)) {
			b.WriteString(prettyIndent)
			b.WriteString(l)
			b.WriteString("\n")
		}
	}
	return b.String()
}

// prettyRowLines vuelca una fila lógica a sus líneas físicas: las columnas de
// texto libre pueden envolver a varias líneas y las demás quedan en blanco en
// las de continuación, igual que hace la tabla de `skills list`.
func prettyRowLines(cols []prettyCol, row []string) []string {
	cells := make([][]string, len(cols))
	height := 1
	for i, c := range cols {
		text := ""
		if i < len(row) {
			text = row[i]
		}
		if c.wrap {
			cells[i] = wrapLines(text, c.width)
			if len(cells[i]) == 0 {
				cells[i] = []string{""}
			}
		} else {
			cells[i] = []string{truncateCell(text, c.width)}
		}
		if len(cells[i]) > height {
			height = len(cells[i])
		}
	}
	out := make([]string, 0, height)
	for li := 0; li < height; li++ {
		var b strings.Builder
		b.WriteString(prettyIndent)
		for i, c := range cols {
			if i > 0 {
				b.WriteString(strings.Repeat(" ", prettyGap))
			}
			text := ""
			if li < len(cells[i]) {
				text = cells[i][li]
			}
			b.WriteString(padRight(text, c.width))
		}
		out = append(out, b.String())
	}
	return out
}

// truncateCell recorta la celda al ancho dejando una elipsis final: la celda
// nunca empuja la columna fuera del ancho fijo (S6 de PRETTY-01).
func truncateCell(s string, width int) string {
	r := []rune(s)
	if len(r) <= width {
		return s
	}
	if width <= 1 {
		return "…"
	}
	return string(r[:width-1]) + "…"
}

// prettyMoney formatea un importe del contrato en es-ES (S4, arranque D-01):
// coma decimal y símbolo, p. ej. 742,18 €. Parte del string decimal exacto
// del contrato: nunca de un float.
func prettyMoney(d model.Decimal) string {
	return strings.Replace(d.Decimal().StringFixed(2), ".", ",", 1) + " €"
}

// prettyPct formatea un tipo en es-ES (S4): coma decimal y símbolo, p. ej.
// 3,101 %. La escala es la que lleva el propio valor del contrato.
func prettyPct(d model.Decimal) string {
	return strings.Replace(d.String(), ".", ",", 1) + " %"
}

// prettyAttributionLine es la línea de atribución de las tablas (P-05): el
// texto exacto del contrato con su fecha de actualización de datos.
func prettyAttributionLine(a model.Attribution) string {
	return a.Text + " (datos actualizados: " + a.DataUpdatedAt + ")"
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
