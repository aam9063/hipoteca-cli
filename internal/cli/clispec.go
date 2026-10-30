package cli

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/aam9063/hipoteca-cli/internal/model"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// Este fichero genera docs/cli-spec.md (AGENT-02, S1/S5/S6). Es LA fuente del
// documento: el árbol de comandos y banderas sale de la misma construcción de
// cobra que ejecuta el binario, y el contrato de salida del mismo esquema que
// describe `hipoteca schema`. El fichero del repositorio no se edita a mano:
// el test de no-desfase (clispec_test.go) regenera y compara, y falla si
// alguien edita el Markdown sin pasar por aquí.
//
// Determinismo (S6): la salida es byte-idéntica en Linux, macOS y Windows.
// No hay rutas, ni marcas de tiempo, ni iteración sobre mapas; cobra ordena
// los subcomandos por nombre y pflag las banderas en orden lexicográfico.
// Un texto de ayuda con contenido que depende del reloj (el año en curso)
// rompería esto en cada 1 de enero, así que index.go lo redacta de forma
// estática. Si algún día un texto no puede ser estático, se deja fuera del
// documento antes que dejar que el test desfasifique por calendario.

// specDefaultVersion es la marca de los comandos disponibles en la versión
// de corte actual (S5): las Olas 2 a 3 más los comandos de agente.
const specDefaultVersion = "v0.1.0 (disponible)"

// specVersionByCommand marca las excepciones: comandos que están en el árbol
// (porque la ayuda y el contrato del binario son la realidad) pero cuya
// entrega se ha aplazado. Un comando aquí se documenta como previsto, nunca
// como disponible (S5).
var specVersionByCommand = map[string]string{
	"hipoteca watch": "v0.2.0 (previsto: todavía no disponible)",
}

// specHeader encabeza el documento: cómo se regenera y por qué no se edita.
// Va como cadena interpretada concatenada: el texto lleva backticks de
// Markdown y no puede ser un raw string.
const specHeader = "# Referencia de la CLI\n\n" +
	"> **Este documento se genera desde el binario; no se edita a mano.** Describe el árbol\n" +
	"> de comandos, las banderas y el contrato de salida tal y como los conoce `hipoteca`.\n" +
	"> Para regenerarlo tras cambiar un comando:\n" +
	">\n" +
	">     go run ./scripts/generate-cli-spec\n" +
	">\n" +
	"> Un test del paquete `internal/cli` falla si este fichero difiere de lo que genera el\n" +
	"> binario (AGENT-02, S2): una referencia que se desfasa del binario miente con autoridad.\n\n" +
	"Salvo que se indique lo contrario, todos los comandos están disponibles en `v0.1.0`.\n" +
	"`watch` queda para `v0.2.0` y se marca como previsto: no está disponible y no debe\n" +
	"usarse todavía.\n\n"

// specContractIntro encabeza la sección del contrato de salida (S1, T2): es
// el mismo contenido que `hipoteca schema` describe, volcado a Markdown.
const specContractIntro = "## Contrato de salida\n\n" +
	"Todos los comandos escriben **un único valor JSON** en stdout, con un salto final\n" +
	"(decisión 7): sin envoltorio, sin prosa mezclada. `skills get` y la ayuda explícita\n" +
	"(`--help`) son las únicas salidas en texto plano. Los importes y los tipos van como\n" +
	"cadenas decimales exactas (`\"152341.18\"`), nunca como float. `schema_version` es la\n" +
	"versión del contrato: %d.\n\n" +
	"En caso de error no hay resultado: el sobre de error va a **stderr** con un código de\n" +
	"salida estable (§6.4). La tabla de abajo es la misma que produce `hipoteca schema`;\n" +
	"el 0 de éxito no aparece en ella porque no es un error.\n\n" +
	"### Códigos de salida\n\n" +
	"| Código | Tipos del contrato |\n|---|---|\n%s\n"

// RenderCLISpec devuelve el contenido completo de docs/cli-spec.md. Es pura:
// no toca ficheros, ni red, ni el reloj.
func RenderCLISpec() string {
	root := NewRootCmd(BuildInfo{})
	var sb strings.Builder
	sb.WriteString(specHeader)
	sb.WriteString("## Comandos\n\n")
	renderSpecCommand(&sb, root, 2, specDefaultVersion)
	renderSpecContract(&sb)
	return sb.String()
}

// renderSpecCommand vuelca un comando y, en profundidad, sus subcomandos.
// depth es el nivel de encabezado Markdown de este comando; parentVersion es
// la marca heredada del padre: la de un grupo aplazado (watch → v0.2.0)
// baja a todos sus subcomandos, que no pueden salir como disponibles porque
// su padre no lo está.
func renderSpecCommand(sb *strings.Builder, cmd *cobra.Command, depth int, parentVersion string) {
	heading := strings.Repeat("#", min(depth, 6))
	fmt.Fprintf(sb, "%s `%s`\n\n", heading, cmd.CommandPath())

	version, marked := specVersionByCommand[cmd.CommandPath()]
	if !marked {
		version = parentVersion
	}
	fmt.Fprintf(sb, "**Estado:** %s.\n\n", version)

	if cmd.Short != "" {
		fmt.Fprintf(sb, "%s\n\n", cmd.Short)
	}
	if long := strings.TrimSpace(cmd.Long); long != "" {
		fmt.Fprintf(sb, "%s\n\n", long)
	}
	fmt.Fprintf(sb, "Uso: `%s`\n\n", cmd.UseLine())

	renderSpecFlags(sb, cmd)

	// cobra ordena los subcomandos por nombre (EnableCommandSorting) y salta
	// los ocultos con Commands(): el orden no depende del orden de AddCommand.
	for _, sub := range cmd.Commands() {
		if sub.Hidden {
			continue
		}
		renderSpecCommand(sb, sub, depth+1, version)
	}
}

// renderSpecFlags vuelca la tabla de banderas locales del comando. pflag
// recorre las banderas en orden lexicográfico (SortFlags), así que la tabla
// es estable. La bandera help la añade cobra al ejecutar: no es parte del
// contrato declarado y cada comando la lleva igual.
func renderSpecFlags(sb *strings.Builder, cmd *cobra.Command) {
	flags := cmd.LocalFlags()
	if !flags.HasAvailableFlags() {
		return
	}
	sb.WriteString("| Bandera | Tipo | Valor por defecto | Descripción |\n")
	sb.WriteString("|---|---|---|---|\n")
	flags.VisitAll(func(f *pflag.Flag) {
		if f.Name == "help" {
			return
		}
		def := f.DefValue
		if def == "" {
			def = "*(vacío)*"
		}
		fmt.Fprintf(sb, "| `--%s` | `%s` | `%s` | %s |\n",
			f.Name, f.Value.Type(), def, escapeSpecCell(f.Usage))
	})
	sb.WriteString("\n")
}

// escapeSpecCell impide que una barra vertical o un salto de línea rompan la
// tabla Markdown. Los textos de ayuda son de una línea; el recorte es por
// si algún día deja de serlo.
func escapeSpecCell(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	return strings.ReplaceAll(s, "\n", " ")
}

// renderSpecContract vuelca el contrato de salida del mismo esquema que
// describe `hipoteca schema` (T2): descripciones, campos por reflexión y
// códigos de salida desde internal/errs.
func renderSpecContract(sb *strings.Builder) {
	exitRows := &strings.Builder{}
	for _, ec := range exitCodeTable() {
		types := make([]string, 0, len(ec.Types))
		for _, t := range ec.Types {
			types = append(types, "`"+t+"`")
		}
		fmt.Fprintf(exitRows, "| %d | %s |\n", ec.Code, strings.Join(types, ", "))
	}
	fmt.Fprintf(sb, specContractIntro, model.SchemaVersion, exitRows.String())

	// El orden es el de schemaTable, la misma secuencia que `hipoteca schema`
	// recorre: literal, no un mapa.
	for _, e := range schemaTable {
		fields := describeStruct(reflect.TypeOf(e.Sample), 0)
		fmt.Fprintf(sb, "### `%s`\n\n", e.Name)
		fmt.Fprintf(sb, "%s\n\n", e.Description)
		if len(fields) == 0 {
			sb.WriteString("*(sin campos)*\n\n")
			continue
		}
		sb.WriteString("| Campo | Tipo | Opcional |\n|---|---|---|\n")
		appendSpecFields(sb, fields, "")
		sb.WriteString("\n")
	}
}

// appendSpecFields vuelca los campos con su ruta completa: los objetos
// anidados con punto (`attribution.data_updated_at`) y los elementos de un
// array con corchetes (`revisions[].payment`). Es un recorrido por slices,
// nunca por mapas: el orden es el de declaración de los struct.
func appendSpecFields(sb *strings.Builder, fields []schemaField, prefix string) {
	for _, f := range fields {
		path := f.Name
		if prefix != "" {
			path = prefix + "." + path
		}
		optional := ""
		if f.Optional {
			optional = "sí"
		}
		fmt.Fprintf(sb, "| `%s` | `%s` | %s |\n", path, f.Type, optional)
		switch f.Type {
		case "object":
			appendSpecFields(sb, f.Fields, path)
		case "array":
			if f.Items == nil {
				continue
			}
			itemPath := path + "[]"
			if f.Items.Type == "object" {
				appendSpecFields(sb, f.Items.Fields, itemPath)
				continue
			}
			fmt.Fprintf(sb, "| `%s` | `%s` |  |\n", itemPath, f.Items.Type)
		}
	}
}
