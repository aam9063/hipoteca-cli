package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aam9063/hipoteca-cli/internal/config"
	"github.com/aam9063/hipoteca-cli/internal/loan"
)

// configDoctor escribe una config de prueba en un directorio temporal y
// devuelve sus tres rutas resueltas dentro de ese directorio.
func configDoctor(t *testing.T) (configPath, dataPath, cacheDir string) {
	t.Helper()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	cfg := config.Default()
	cero := 0
	sinSuelo := ""
	cfg.Loans["casa"] = loan.Loan{
		Name:                  "Casa",
		Type:                  loan.TypeVariable,
		StartDate:             "2021-06-01",
		OriginalPrincipal:     "180000.00",
		TermMonths:            360,
		PaymentDay:            1,
		InitialRate:           "1.25",
		InitialPeriodMonths:   &cero,
		Index:                 "euribor-1y",
		Spread:                "0.89",
		RevisionEveryMonths:   12,
		ReferenceOffsetMonths: &cero,
		FloorRate:             &sinSuelo,
		RateRounding:          "none",
		InterestConvention:    loan.InterestConventionMonthlyNominal,
		PaymentRounding:       loan.PaymentRoundingHalfUpCents,
		EarlyRepaymentFeePct:  "0",
	}
	if err := cfg.Save(cfgPath); err != nil {
		t.Fatalf("config de prueba: %v", err)
	}
	return cfgPath, filepath.Join(dir, "hipoteca.db"), dir
}

// AGENT-01 S5: `hipoteca doctor --offline` responde en un único valor JSON:
// versión y commit, rutas con permisos, catálogo y préstamos con los campos
// que le faltan a cada incompleto. Los tests nunca tocan la red.
func TestDoctorOutputsSingleJSONContract(t *testing.T) {
	cfgPath, dataPath, cacheDir := configDoctor(t)
	cmd := NewRootCmdWithDeps(BuildInfo{Version: "9.9.9", Commit: "deadbee", Date: "2026-10-28"}, Deps{
		ConfigPath: func() (string, error) { return cfgPath, nil },
		DataPath:   func() (string, error) { return dataPath, nil },
		CacheDir:   func() (string, error) { return cacheDir, nil },
	})
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"doctor", "--offline"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("doctor --offline debe funcionar sin red, got %v", err)
	}
	var out struct {
		SchemaVersion int              `json:"schema_version"`
		Version       string           `json:"version"`
		Commit        string           `json:"commit"`
		ConfigPath    string           `json:"config_path"`
		ConfigExists  bool             `json:"config_exists"`
		ConfigMode    string           `json:"config_mode"`
		DataPath      string           `json:"data_path"`
		CacheDir      string           `json:"cache_dir"`
		Indices       []map[string]any `json:"indices"`
		Loans         []struct {
			ID            string   `json:"id"`
			Complete      bool     `json:"complete"`
			MissingFields []string `json:"missing_fields,omitempty"`
		} `json:"loans"`
		Network struct {
			Checked bool `json:"checked"`
		} `json:"network"`
		Warnings []string `json:"warnings"`
	}
	dec := json.NewDecoder(&stdout)
	if err := dec.Decode(&out); err != nil {
		t.Fatalf("stdout no es JSON: %v\n%s", err, stdout.String())
	}
	if _, err := dec.Token(); err != io.EOF {
		t.Errorf("stdout debe llevar UN solo valor JSON, hay resto: %v", err)
	}
	if out.Version != "9.9.9" || out.Commit != "deadbee" {
		t.Errorf("version/commit = %q/%q", out.Version, out.Commit)
	}
	if out.ConfigPath != cfgPath || !out.ConfigExists {
		t.Errorf("config_path = %q exists=%v", out.ConfigPath, out.ConfigExists)
	}
	if out.DataPath != dataPath || out.CacheDir != cacheDir {
		t.Errorf("rutas = %q/%q, want %q/%q", out.DataPath, out.CacheDir, dataPath, cacheDir)
	}
	if len(out.Indices) == 0 {
		t.Error("el catálogo debe listar sus índices")
	}
	if len(out.Loans) != 1 || out.Loans[0].ID != "casa" || !out.Loans[0].Complete {
		t.Errorf("loans = %+v, want el préstamo completo casa", out.Loans)
	}
	if out.Network.Checked {
		t.Errorf("con --offline la red no se comprueba: %+v", out.Network)
	}
	if len(out.Warnings) != 0 {
		t.Errorf("una instalación sana no lleva avisos, got %v", out.Warnings)
	}
}

// AGENT-01 S5: un préstamo incompleto aparece en doctor con el campo que le
// falta, para que un agente sepa qué pedir sin abrir la config.
func TestDoctorNamesMissingFieldsOfIncompleteLoans(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	cfg := config.Default()
	sinSuelo := ""
	cero := 0
	// "a medias": sin suelo ni offset de referencia.
	cfg.Loans["casa"] = loan.Loan{
		Name:                "Casa",
		Type:                loan.TypeVariable,
		StartDate:           "2021-06-01",
		OriginalPrincipal:   "180000.00",
		TermMonths:          360,
		PaymentDay:          1,
		InitialRate:         "1.25",
		InitialPeriodMonths: &cero,
		Index:               "euribor-1y",
		Spread:              "0.89",
		FloorRate:           &sinSuelo,
	}
	if err := cfg.Save(cfgPath); err != nil {
		t.Fatal(err)
	}
	cmd := NewRootCmdWithDeps(BuildInfo{Version: "test", Commit: "abc", Date: "x"}, Deps{
		ConfigPath: func() (string, error) { return cfgPath, nil },
		DataPath:   func() (string, error) { return filepath.Join(dir, "hipoteca.db"), nil },
		CacheDir:   func() (string, error) { return dir, nil },
	})
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"doctor", "--offline"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("doctor no debe fallar, got %v", err)
	}
	var out struct {
		Loans []struct {
			ID            string   `json:"id"`
			Complete      bool     `json:"complete"`
			MissingFields []string `json:"missing_fields"`
		} `json:"loans"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("stdout no es JSON: %v", err)
	}
	if len(out.Loans) != 1 || out.Loans[0].Complete {
		t.Fatalf("loans = %+v, want casa incompleto", out.Loans)
	}
	hayOffset, hayRedondeo := false, false
	for _, f := range out.Loans[0].MissingFields {
		if f == "reference_offset_months" {
			hayOffset = true
		}
		if f == "rate_rounding" {
			hayRedondeo = true
		}
	}
	if !hayOffset || !hayRedondeo {
		t.Errorf("missing_fields = %v, deben nombrar reference_offset_months y rate_rounding", out.Loans[0].MissingFields)
	}
}

// AGENT-01 S5: la configuración real no se toca: sin inyección de rutas el
// comando fallaría de forma ruidosa; con --offline y rutas inyectadas,
// ninguna prueba lee el HOME del usuario. Esta prueba es structural: doctor
// acepta la bandera --offline.
func TestDoctorAcceptsOfflineFlag(t *testing.T) {
	cfgPath, dataPath, cacheDir := configDoctor(t)
	cmd := NewRootCmdWithDeps(BuildInfo{Version: "test", Commit: "abc", Date: "x"}, Deps{
		ConfigPath: func() (string, error) { return cfgPath, nil },
		DataPath:   func() (string, error) { return dataPath, nil },
		CacheDir:   func() (string, error) { return cacheDir, nil },
	})
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"doctor", "--offline"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("got %v", err)
	}
	if !strings.Contains(stdout.String(), `"checked":false`) {
		t.Errorf("la salida debe dejar claro que la red no se comprobó: %s", stdout.String())
	}
}
