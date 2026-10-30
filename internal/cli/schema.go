package cli

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/aam9063/hipoteca-cli/internal/app"
	"github.com/aam9063/hipoteca-cli/internal/errs"
	"github.com/aam9063/hipoteca-cli/internal/model"
	"github.com/aam9063/hipoteca-cli/internal/output"
	"github.com/spf13/cobra"
)

// `schema` describe el contrato de salida (AGENT-01, S4). La tabla de abajo
// NO es una segunda fuente de verdad: cada entrada apunta al MISMO tipo Go
// que el comando escribe por stdout con output.WriteJSON, y el describor
// reflection lee los json tags de ese tipo. Añadir un campo a una salida
// actualiza `schema` sin tocar nada aquí; hacer un comando nuevo sin
// registrarlo en la tabla es lo único que puede desfasar, y el test de
// contrato lo cubre con la salida real del binario.

// schemaField es un campo del contrato: nombre JSON, tipo del contrato
// ("string", "integer", "boolean", "decimal", "object", "array"), si puede
// faltar (puntero u omitempty), y los campos anidados de los objetos o el
// elemento de los arrays.
type schemaField struct {
	Name     string        `json:"name"`
	Type     string        `json:"type"`
	Optional bool          `json:"optional,omitempty"`
	Fields   []schemaField `json:"fields,omitempty"`
	Items    *schemaField  `json:"items,omitempty"`
}

// schemaExitCode es una fila de la tabla de códigos de salida (§6.4): el
// código y los tipos del contrato que lo producen.
type schemaExitCode struct {
	Code  int      `json:"code"`
	Types []string `json:"types"`
}

// schemaCommand es la descripción de la salida de un comando.
type schemaCommand struct {
	Command     string           `json:"command"`
	Description string           `json:"description"`
	ExitCodes   []schemaExitCode `json:"exit_codes"`
	Fields      []schemaField    `json:"fields"`
}

// schemaOutput es la salida de `hipoteca schema` y `hipoteca schema <cmd>`:
// siempre la misma forma, con uno o todos los comandos descritos.
type schemaOutput struct {
	SchemaVersion int             `json:"schema_version"`
	Commands      []schemaCommand `json:"commands"`
}

// schemaEntry es una fila de la tabla: el nombre del comando tal cual se
// invoca y el tipo de su salida.
type schemaEntry struct {
	Name        string
	Description string
	Sample      any
}

// schemaTable es LA tabla: cada comando de las Olas 2 a 3 (más los nuevos de
// AGENT-01 que también tienen salida) con el tipo exacto que escribe.
var schemaTable = []schemaEntry{
	{"index list", "Todos los índices del catálogo con su último valor publicado", app.IndexValues{}},
	{"index show", "Histórico mensual de un índice", app.IndexHistoryResult{}},
	{"index latest", "Último valor publicado de uno o varios índices", app.IndexValues{}},
	{"calc", "Cuota, intereses y total de un cálculo suelto", app.CalcResult{}},
	{"revision", "La revisión de un préstamo guardado", app.RevisionResult{}},
	{"schedule", "El cuadro de amortización de un préstamo guardado", app.ScheduleResult{}},
	{"amortize", "El efecto de una amortización anticipada", app.AmortizeResult{}},
	{"scenario", "Cuota y coste total con valores del índice supuestos", app.ScenarioResult{}},
	{"compare", "El préstamo recalculado con otro índice", app.CompareResult{}},
	{"loan add", "El préstamo guardado, tal cual quedó", app.LoanShowResult{}},
	{"loan edit", "El préstamo tras la edición", app.LoanShowResult{}},
	{"loan sync", "El préstamo tras guardar el recibo", app.LoanShowResult{}},
	{"loan list", "Los préstamos guardados con su marca de completo", app.LoanListResult{}},
	{"loan show", "Un préstamo guardado con lo que le falta", app.LoanShowResult{}},
	{"loan remove", "La confirmación del borrado", removedResult{}},
	{"skills list", "Las recetas embebidas para agentes", skillsListOutput{}},
	{"doctor", "El estado de la instalación", app.DoctorResult{}},
	{"config show", "La configuración completa, con los sinks redactados", app.ConfigShowResult{}},
	{"config get", "El valor de una clave de la config", app.ConfigValueResult{}},
	{"config set", "La clave escrita en la config", app.ConfigValueResult{}},
}

// newSchemaCmd: `hipoteca schema [comando]`.
func newSchemaCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "schema [comando]",
		Short: "El contrato de salida de los comandos: campos, tipos y códigos de salida",
		Long: `Describe el contrato de salida de los comandos: sus campos y tipos, y los
códigos de salida del contrato (§6.4). Se genera de los mismos tipos que los
comandos escriben en stdout, así que no puede desfasar del binario.

Sin argumento describe todos los comandos; con un nombre ("hipoteca schema
revision") describe sólo ese.`,
		Args: maxArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var entries []schemaEntry
			if len(args) == 1 {
				entry, ok := lookupSchemaEntry(args[0])
				if !ok {
					return errs.New(errs.KindNotFound,
						fmt.Sprintf("no hay ningún comando llamado %q en el contrato", args[0]),
						"comandos descritos: "+schemaNames())
				}
				entries = []schemaEntry{entry}
			} else {
				entries = schemaTable
			}
			out := schemaOutput{
				SchemaVersion: model.SchemaVersion,
				Commands:      describeAll(entries),
			}
			if err := output.WriteJSON(cmd.OutOrStdout(), out); err != nil {
				return fmt.Errorf("escribiendo el resultado: %w", err)
			}
			return nil
		},
	}
}

// lookupSchemaEntry resuelve "index list" o "revision" en la tabla.
func lookupSchemaEntry(name string) (schemaEntry, bool) {
	for _, e := range schemaTable {
		if e.Name == name {
			return e, true
		}
	}
	return schemaEntry{}, false
}

// schemaNames lista los nombres de la tabla para el hint.
func schemaNames() string {
	names := make([]string, 0, len(schemaTable))
	for _, e := range schemaTable {
		names = append(names, e.Name)
	}
	return strings.Join(names, ", ")
}

// describeAll describe cada entrada: campos por reflexión sobre el tipo de
// su salida, y la tabla de códigos de salida del contrato.
func describeAll(entries []schemaEntry) []schemaCommand {
	exitCodes := exitCodeTable()
	out := make([]schemaCommand, 0, len(entries))
	for _, e := range entries {
		out = append(out, schemaCommand{
			Command:     e.Name,
			Description: e.Description,
			ExitCodes:   exitCodes,
			Fields:      describeStruct(reflect.TypeOf(e.Sample), 0),
		})
	}
	return out
}

// exitCodeTable construye la tabla código → tipos desde internal/errs, la
// única fuente de la correspondencia (§6.4): ningún mapeo propio.
func exitCodeTable() []schemaExitCode {
	kinds := []errs.Kind{
		errs.KindInternal,
		errs.KindUsage,
		errs.KindLoanIncomplete,
		errs.KindCancelled,
		errs.KindConfirmationRequired,
		errs.KindNotFound,
		errs.KindIndexNotPublished,
		errs.KindUpstreamUnavailable,
		errs.KindRateLimited,
		errs.KindNetwork,
		errs.KindAPIChanged,
	}
	byCode := map[int][]string{}
	for _, k := range kinds {
		code := errs.ExitCode(k)
		byCode[code] = append(byCode[code], string(k))
	}
	codes := make([]int, 0, len(byCode))
	for code := range byCode {
		codes = append(codes, code)
	}
	sort.Ints(codes)
	out := make([]schemaExitCode, 0, len(codes))
	for _, code := range codes {
		out = append(out, schemaExitCode{Code: code, Types: byCode[code]})
	}
	return out
}

// maxDepth para la reflexión: los tipos del contrato no tienen ciclos, pero
// un límite explícito evita que un cambio descuidado cuelgue el comando.
const maxDepth = 8

// describeStruct recorre los campos json de un struct del contrato. Los
// struct anónimos embebidos (p. ej. LoanView sobre loan.Loan) se aplanan,
// igual que hace encoding/json.
func describeStruct(t reflect.Type, depth int) []schemaField {
	if t == nil || depth > maxDepth {
		return nil
	}
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil
	}
	fields := make([]schemaField, 0, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		// Un struct embebido sin json tag se aplanan: sus campos suben.
		if f.Anonymous && f.Type.Kind() == reflect.Struct && !isDecimal(f.Type) {
			fields = append(fields, describeStruct(f.Type, depth+1)...)
			continue
		}
		name := f.Name
		optional := false
		tag := f.Tag.Get("json")
		if tag != "" {
			parts := strings.Split(tag, ",")
			if parts[0] != "" {
				name = parts[0]
			}
			for _, opt := range parts[1:] {
				if opt == "omitempty" {
					optional = true
				}
			}
		}
		if f.Type.Kind() == reflect.Ptr {
			optional = true
		}
		field := schemaField{
			Name:     name,
			Type:     contractType(f.Type),
			Optional: optional,
		}
		if field.Type == "object" && f.Type.Kind() == reflect.Struct {
			field.Fields = describeStruct(f.Type, depth+1)
		}
		if field.Type == "array" {
			item := describeElem(f.Type.Elem(), depth+1)
			field.Items = &item
		}
		fields = append(fields, field)
	}
	return fields
}

// describeElem describe el elemento de un array: objeto con campos, o
// escalar del contrato.
func describeElem(t reflect.Type, depth int) schemaField {
	field := schemaField{Name: "", Type: contractType(t)}
	if field.Type == "object" && t.Kind() == reflect.Struct {
		field.Fields = describeStruct(t, depth)
	}
	return field
}

// isDecimal informa de si el tipo es model.Decimal (o su puntero).
func isDecimal(t reflect.Type) bool {
	return t == reflect.TypeOf(model.Decimal{}) || t == reflect.TypeOf(&model.Decimal{})
}

// contractType traduce un tipo Go al tipo del contrato: los importes y los
// tipos van como cadenas decimales exactas (model.Decimal → "decimal"), los
// meses como enteros, las fechas ISO como strings.
func contractType(t reflect.Type) string {
	if isDecimal(t) {
		return "decimal"
	}
	switch t.Kind() {
	case reflect.String:
		return "string"
	case reflect.Int, reflect.Int64:
		return "integer"
	case reflect.Bool:
		return "boolean"
	case reflect.Slice:
		return "array"
	case reflect.Map:
		return "object"
	case reflect.Ptr:
		return contractType(t.Elem())
	case reflect.Struct:
		return "object"
	}
	return t.Kind().String()
}
