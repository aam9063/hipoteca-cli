package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/aam9063/hipoteca-cli/internal/errs"
)

// runSchema ejecuta `hipoteca schema ...` en el árbol real y devuelve la
// salida JSON decodificada.
func runSchema(t *testing.T, args []string) map[string]any {
	t.Helper()
	cmd := NewRootCmd(BuildInfo{Version: "test", Commit: "abc1234", Date: "2026-10-28"})
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("%v debe funcionar sin red ni config, got %v", args, err)
	}
	var out map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("la salida de %v no es JSON: %v\n%s", args, err, stdout.String())
	}
	return out
}

// AGENT-01 S4: `hipoteca schema` describe el contrato de salida de los
// comandos de las Olas 2 a 3, generado de los mismos tipos que escriben los
// comandos, con los códigos de salida del contrato (§6.4).
func TestSchemaDescribesTheWelle23Commands(t *testing.T) {
	out := runSchema(t, []string{"schema"})
	wantCommands := []string{
		"index list", "index show", "index latest",
		"calc", "revision", "schedule",
		"amortize", "scenario", "compare",
		"loan add", "loan edit", "loan sync", "loan list", "loan show", "loan remove",
	}
	byName := map[string]map[string]any{}
	for _, c := range out["commands"].([]any) {
		m := c.(map[string]any)
		byName[m["command"].(string)] = m
	}
	for _, name := range wantCommands {
		c, ok := byName[name]
		if !ok {
			t.Errorf("schema no describe el comando %q", name)
			continue
		}
		fields, ok := c["fields"].([]any)
		if !ok || len(fields) == 0 {
			t.Errorf("el comando %q no declara campos", name)
		}
		if _, ok := c["exit_codes"].([]any); !ok {
			t.Errorf("el comando %q no declara códigos de salida", name)
		}
	}
}

// AGENT-01 S4/T6: el schema no puede desfasar del binario porque se genera
// de los mismos tipos que escriben los comandos. Prueba de contrato: la
// salida REAL de `calc` (que no necesita red ni config) valida campo a campo
// contra lo que `schema calc` declara.
func TestSchemaMatchesTheRealOutputOfCalc(t *testing.T) {
	schema := runSchema(t, []string{"schema", "calc"})
	var wire struct {
		Commands []struct {
			Command string           `json:"command"`
			Fields  []schemaWireTest `json:"fields"`
		} `json:"commands"`
	}
	b, _ := json.Marshal(schema)
	if err := json.Unmarshal(b, &wire); err != nil {
		t.Fatalf("schema calc no es JSON válido: %v", err)
	}
	commands := wire.Commands
	if len(commands) != 1 || commands[0].Command != "calc" {
		t.Fatalf("schema calc debe describir sólo calc, got %+v", commands)
	}

	// Salida real del comando.
	cmd := NewRootCmd(BuildInfo{Version: "test", Commit: "abc1234", Date: "2026-10-28"})
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"calc", "--principal", "150000", "--rate", "3.1", "--months", "300"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("calc debe funcionar sin red, got %v", err)
	}
	var real map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &real); err != nil {
		t.Fatalf("la salida de calc no es JSON: %v", err)
	}
	compareFieldsToJSON(t, "calc", commands[0].Fields, real)
}

// schemaWireTest es la forma wire de un campo del schema para las pruebas.
type schemaWireTest struct {
	Name   string           `json:"name"`
	Type   string           `json:"type"`
	Fields []schemaWireTest `json:"fields,omitempty"`
	Items  *schemaWireTest  `json:"items,omitempty"`
}

func compareFieldsToJSON(t *testing.T, path string, fields []schemaWireTest, real map[string]any) {
	t.Helper()
	declared := map[string]bool{}
	for _, f := range fields {
		declared[f.Name] = true
		v, present := real[f.Name]
		if !present {
			// Un campo con omitempty puede faltar en una salida concreta: no
			// es desvío. Pero no lo podemos comparar: seguimos.
			continue
		}
		if f.Fields != nil {
			obj, ok := v.(map[string]any)
			if !ok {
				t.Errorf("%s.%s se declara objeto pero la salida real trae %T", path, f.Name, v)
				continue
			}
			compareFieldsToJSON(t, path+"."+f.Name, f.Fields, obj)
		}
		if f.Items != nil && f.Items.Fields != nil {
			arr, ok := v.([]any)
			if !ok || len(arr) == 0 {
				continue
			}
			obj, ok := arr[0].(map[string]any)
			if !ok {
				t.Errorf("%s.%s se declara array de objetos pero la salida real trae %T", path, f.Name, v)
				continue
			}
			compareFieldsToJSON(t, path+"."+f.Name+"[]", f.Items.Fields, obj)
		}
	}
	for k := range real {
		if !declared[k] {
			t.Errorf("la salida real de %s trae el campo %q que schema no declara: el schema se ha quedado anticuado", path, k)
		}
	}
}

// AGENT-01 S4: los códigos de salida declarados vienen de la tabla de
// internal/errs (§6.4), no de un mapa propio.
func TestSchemaExitCodesComeFromTheContractTable(t *testing.T) {
	out := runSchema(t, []string{"schema", "revision"})
	b, _ := json.Marshal(out)
	var wire struct {
		Commands []struct {
			ExitCodes []struct {
				Code  int      `json:"code"`
				Types []string `json:"types"`
			} `json:"exit_codes"`
		} `json:"commands"`
	}
	if err := json.Unmarshal(b, &wire); err != nil {
		t.Fatalf("schema revision no es JSON válido: %v", err)
	}
	commands := wire.Commands
	all := map[int][]string{}
	for _, ec := range commands[0].ExitCodes {
		for _, typ := range ec.Types {
			if errs.ExitCode(errs.Kind(typ)) != ec.Code {
				t.Errorf("el tipo %q se declara con código %d, pero errs dice %d", typ, ec.Code, errs.ExitCode(errs.Kind(typ)))
			}
			all[ec.Code] = append(all[ec.Code], typ)
		}
	}
	// El contrato de `revision`: usage/loan_incomplete (2) e
	// index_not_published (4) tienen que estar.
	found2, found4 := false, false
	for _, typ := range all[2] {
		if typ == "usage" || typ == "loan_incomplete" {
			found2 = true
		}
	}
	for _, typ := range all[4] {
		if typ == "index_not_published" {
			found4 = true
		}
	}
	if !found2 || !found4 {
		t.Errorf("schema revision debe declarar los códigos 2 y 4 del contrato, got %+v", all)
	}
}

// AGENT-01 S4: un comando desconocido es not_found/4 con los nombres válidos
// en el hint y stdout vacío.
func TestSchemaUnknownCommandIsNotFoundWithHint(t *testing.T) {
	cmd := NewRootCmd(BuildInfo{Version: "test", Commit: "abc1234", Date: "2026-10-28"})
	var stdout, _ bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"schema", "inventado"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("schema de un comando desconocido debe fallar")
	}
	var e *errs.Error
	if !errors.As(err, &e) {
		t.Fatalf("el error debe llevar kind del contrato, got %v", err)
	}
	if e.Kind != errs.KindNotFound {
		t.Errorf("kind = %q, want not_found", e.Kind)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout debe quedar vacío en fallo, got %q", stdout.String())
	}
	if !strings.Contains(e.Hint, "revision") {
		t.Errorf("el hint %q debe listar comandos válidos", e.Hint)
	}
}
