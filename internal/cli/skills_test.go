package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/aam9063/hipoteca-cli/internal/errs"
	"github.com/aam9063/hipoteca-cli/internal/model"
)

// AGENT-01 S1: `skills list` saca un único valor JSON en stdout con
// schema_version y las recetas con id y resumen.
func TestSkillsListOutputsSingleJSONValue(t *testing.T) {
	cmd := NewRootCmd(BuildInfo{Version: "test", Commit: "abc1234", Date: "2026-10-28"})
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"skills", "list"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("skills list debe funcionar sin configuración ni red, got %v", err)
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr debe quedarse vacío, got %q", stderr.String())
	}
	var out struct {
		SchemaVersion int `json:"schema_version"`
		Skills        []struct {
			ID      string `json:"id"`
			Title   string `json:"title"`
			Summary string `json:"summary"`
		} `json:"skills"`
	}
	dec := json.NewDecoder(&stdout)
	if err := dec.Decode(&out); err != nil {
		t.Fatalf("stdout no es un valor JSON válido: %v\n%s", err, stdout.String())
	}
	// P-01: exactamente un valor JSON: tras decodificar no queda nada.
	if _, err := dec.Token(); err != io.EOF {
		t.Errorf("stdout debe llevar UN solo valor JSON, hay resto: %v", err)
	}
	if out.SchemaVersion != model.SchemaVersion {
		t.Errorf("schema_version = %d, want %d", out.SchemaVersion, model.SchemaVersion)
	}
	if len(out.Skills) != 4 {
		t.Fatalf("skills list debe listar las cuatro recetas de §6.5, got %d", len(out.Skills))
	}
	for _, s := range out.Skills {
		if s.ID == "" || s.Summary == "" {
			t.Errorf("la entrada %+v no tiene id o resumen", s)
		}
	}
}

// AGENT-01 S2: `skills get <id>` es LA excepción text/plain del contrato:
// markdown listo para ejecutar, no JSON.
func TestSkillsGetWritesPlainTextRecipe(t *testing.T) {
	cmd := NewRootCmd(BuildInfo{Version: "test", Commit: "abc1234", Date: "2026-10-28"})
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"skills", "get", "next-revision"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("skills get debe funcionar sin configuración ni red, got %v", err)
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr debe quedarse vacío, got %q", stderr.String())
	}
	body := stdout.String()
	if strings.HasPrefix(strings.TrimSpace(body), "{") {
		t.Errorf("skills get es text/plain, no JSON: %q", body[:40])
	}
	for _, want := range []string{"hipoteca revision", "--assume", "index_not_published", "loan_incomplete"} {
		if !strings.Contains(body, want) {
			t.Errorf("la receta impresa no menciona %q", want)
		}
	}
}

// AGENT-01 S2: un id desconocido es not_found/4 con la lista de ids válidos
// en el hint, y stdout queda vacío (el sobre va a stderr, §6.4).
func TestSkillsGetUnknownIDIsNotFoundWithHint(t *testing.T) {
	cmd := NewRootCmd(BuildInfo{Version: "test", Commit: "abc1234", Date: "2026-10-28"})
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"skills", "get", "inventada"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("un id desconocido debe fallar")
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
	for _, want := range []string{"next-revision", "early-repayment", "sensitivity", "index-comparison"} {
		if !strings.Contains(e.Hint, want) {
			t.Errorf("el hint %q debe listar el id válido %q", e.Hint, want)
		}
	}
}

// AGENT-01: `skills get` sin id es un error de uso (2), no un fallo interno.
func TestSkillsGetWithoutIDIsUsage(t *testing.T) {
	cmd := NewRootCmd(BuildInfo{Version: "test", Commit: "abc1234", Date: "2026-10-28"})
	var stdout, _ bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"skills", "get"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("skills get sin id debe fallar")
	}
	var e *errs.Error
	if !errors.As(err, &e) || e.Kind != errs.KindUsage {
		t.Fatalf("kind = %v, want usage", err)
	}
}
