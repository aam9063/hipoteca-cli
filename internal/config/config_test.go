package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/aam9063/hipoteca-cli/internal/loan"
)

// writeBytes writes b to a new file under dir and returns its path.
func writeBytes(t *testing.T, dir, name string, b []byte) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, b, 0o600); err != nil {
		t.Fatalf("setup: os.WriteFile(%q): %v", p, err)
	}
	return p
}

const validTOML = `schema_version = 1

[indices]
euribor1y = "SECUENCIA_TIPO_INTERES_1"
`

// TestLoadMissingFileReturnsDefaults: a config that does not exist is not an
// error; Load returns the defaults (CORE-02.2).
func TestLoadMissingFileReturnsDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "config.toml")

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load() on missing file: error = %v, want defaults and no error", err)
	}
	want := Default()
	if got.SchemaVersion != want.SchemaVersion {
		t.Fatalf("Load() SchemaVersion = %d, want %d", got.SchemaVersion, want.SchemaVersion)
	}
	if len(got.Indices) != 0 {
		t.Fatalf("Load() Indices = %v, want empty", got.Indices)
	}
}

// TestLoadParsesFile checks that an existing valid config is read back.
func TestLoadParsesFile(t *testing.T) {
	path := writeBytes(t, t.TempDir(), "config.toml", []byte(validTOML))

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.SchemaVersion != 1 {
		t.Fatalf("SchemaVersion = %d, want 1", got.SchemaVersion)
	}
	if got.Indices["euribor1y"] != "SECUENCIA_TIPO_INTERES_1" {
		t.Fatalf("Indices = %v, want the override for euribor1y", got.Indices)
	}
}

// TestLoadCorruptFileErrorsAndNeverOverwrites: a corrupt file is an explicit
// error carrying the path, and the file's bytes are untouched afterwards —
// Load never "repairs" user data (decision 5 of the task).
func TestLoadCorruptFileErrorsAndNeverOverwrites(t *testing.T) {
	corrupt := []byte("schema_version = 1\n[indices\n")
	path := writeBytes(t, t.TempDir(), "config.toml", corrupt)

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() on corrupt file: error = nil, want an error")
	}
	if !strings.Contains(err.Error(), path) {
		t.Fatalf("Load() error %q does not mention the path %q", err, path)
	}
	after, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("reading config after failed Load: %v", readErr)
	}
	if string(after) != string(corrupt) {
		t.Fatalf("corrupt config was modified:\nbefore: %q\nafter:  %q", corrupt, after)
	}
}

// TestLoadUnsupportedSchemaVersion: an unsupported schema_version is rejected
// with a clear message; the file is not touched.
func TestLoadUnsupportedSchemaVersion(t *testing.T) {
	content := []byte("schema_version = 99\n")
	path := writeBytes(t, t.TempDir(), "config.toml", content)

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() with schema_version 99: error = nil, want an error")
	}
	if !strings.Contains(err.Error(), "99") {
		t.Fatalf("error %q does not mention the unsupported version 99", err)
	}
	after, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("reading config after failed Load: %v", readErr)
	}
	if string(after) != string(content) {
		t.Fatal("config was modified by a rejected Load")
	}
}

// TestSaveLoadRoundTrip: save then load returns the same SchemaVersion and the
// same Indices map, including an empty map (CORE-02.4).
func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "config.toml") // parent must be created

	for name, cfg := range map[string]Config{
		"with overrides": {
			SchemaVersion: 1,
			Indices: map[string]string{
				"euribor1y": "SECUENCIA_TIPO_INTERES_1",
				"euribor3m": "SECUENCIA_TIPO_INTERES_2",
			},
		},
		"empty indices": {
			SchemaVersion: 1,
			Indices:       map[string]string{},
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := cfg.Save(path); err != nil {
				t.Fatalf("Save() error = %v", err)
			}
			got, err := Load(path)
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if got.SchemaVersion != cfg.SchemaVersion {
				t.Fatalf("SchemaVersion = %d, want %d", got.SchemaVersion, cfg.SchemaVersion)
			}
			if len(got.Indices) != len(cfg.Indices) {
				t.Fatalf("Indices = %v, want %v", got.Indices, cfg.Indices)
			}
			for k, v := range cfg.Indices {
				if got.Indices[k] != v {
					t.Fatalf("Indices[%q] = %q, want %q", k, got.Indices[k], v)
				}
			}
		})
	}
}

// TestSaveRejectsUnsupportedSchemaVersion: Save never writes a config that
// Load would refuse to read back.
func TestSaveRejectsUnsupportedSchemaVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	cfg := Config{SchemaVersion: 99, Indices: map[string]string{}}

	if err := cfg.Save(path); err == nil {
		t.Fatal("Save() with schema_version 99: error = nil, want an error")
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatal("Save() with an unsupported version left a file behind")
	}
}

// ----------------------------------------------------------------------------
// Sección [loans.<id>] (LOAN-01.2): la misma escritura atómica de CORE-02,
// con el modelo de internal/loan.
// ----------------------------------------------------------------------------

// prestamoCasa devuelve el préstamo del ejemplo de docs/ARRANQUE.md §4.1,
// completo, para las pruebas de ida y vuelta.
func prestamoCasa() loan.Loan {
	initialPeriod := 12
	offset := 2
	sinSuelo := ""
	return loan.Loan{
		Name:                  "Casa",
		Type:                  loan.TypeVariable,
		StartDate:             "2021-06-01",
		OriginalPrincipal:     "180000.00",
		TermMonths:            360,
		PaymentDay:            1,
		InitialRate:           "1.25",
		InitialPeriodMonths:   &initialPeriod,
		Index:                 "euribor-1y",
		Spread:                "0.89",
		SpreadUnbonified:      "1.39",
		RevisionEveryMonths:   12,
		ReferenceOffsetMonths: &offset,
		FloorRate:             &sinSuelo,
		RateRounding:          "none",
		InterestConvention:    loan.InterestConventionMonthlyNominal,
		PaymentRounding:       loan.PaymentRoundingHalfUpCents,
		EarlyRepaymentFeePct:  "0",
	}
}

// equalLoans compara dos préstamos campo a campo, incluidos los punteros.
func equalLoans(t *testing.T, want, got loan.Loan) {
	t.Helper()
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("el préstamo cambió en la ida y vuelta:\nwant %+v\ngot  %+v", want, got)
	}
}

// TestLoadParsesLoansSection: un TOML escrito a mano con la forma del
// arranque (§4.1, [loans.<id>] con sync anidado) se lee en cfg.Loans. Fija el
// formato del archivo: claves snake_case, floor_rate vacío = sin suelo y
// sync como tabla anidada.
func TestLoadParsesLoansSection(t *testing.T) {
	tomlText := `schema_version = 1

[indices]
euribor1y = "SECUENCIA_TIPO_INTERES_1"

[loans.casa]
name = "Casa"
type = "variable"
start_date = "2021-06-01"
original_principal = "180000.00"
term_months = 360
payment_day = 1
initial_rate = "1.25"
initial_period_months = 12
index = "euribor-1y"
spread = "0.89"
spread_unbonified = "1.39"
revision_every_months = 12
reference_offset_months = 2
floor_rate = ""
rate_rounding = "none"
interest_convention = "monthly-nominal"
payment_rounding = "half-up-cents"
early_repayment_fee_pct = "0"

[loans.casa.sync]
date = "2026-10-01"
outstanding_principal = "152341.18"
current_rate = "3.12"
`
	path := writeBytes(t, t.TempDir(), "config.toml", []byte(tomlText))

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(got.Indices) != 1 {
		t.Fatalf("Indices = %v: la sección loans no debe pisar las demás", got.Indices)
	}
	if len(got.Loans) != 1 {
		t.Fatalf("Loans = %v, want un préstamo casa", got.Loans)
	}
	l, ok := got.Loans["casa"]
	if !ok {
		t.Fatalf("no está el préstamo casa: %v", got.Loans)
	}
	want := prestamoCasa()
	want.Sync = &loan.Sync{Date: "2026-10-01", OutstandingPrincipal: "152341.18", CurrentRate: "3.12"}
	equalLoans(t, want, l)
}

// TestSaveLoadRoundTripLoans: guardar y volver a leer conserva la sección de
// préstamos campo a campo, incluidos floor_rate = "" (sin suelo explícito),
// los punteros a cero (offset 0) y un préstamo sin sync (LOAN-01.2).
func TestSaveLoadRoundTripLoans(t *testing.T) {
	offsetCero := 0
	tramoCero := 0
	for name, l := range map[string]loan.Loan{
		"completo con sync": func() loan.Loan {
			l := prestamoCasa()
			l.Sync = &loan.Sync{Date: "2026-10-01", OutstandingPrincipal: "152341.18", CurrentRate: "3.12"}
			return l
		}(),
		"offset cero y sin suelo": func() loan.Loan {
			l := prestamoCasa()
			l.ReferenceOffsetMonths = &offsetCero
			l.InitialPeriodMonths = &tramoCero
			return l
		}(),
		"sin sync": prestamoCasa(),
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.toml")
			cfg := Default()
			cfg.Loans = map[string]loan.Loan{"casa": l}
			if err := cfg.Save(path); err != nil {
				t.Fatalf("Save() error = %v", err)
			}
			got, err := Load(path)
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if len(got.Loans) != 1 {
				t.Fatalf("Loans = %v, want un préstamo", got.Loans)
			}
			equalLoans(t, l, got.Loans["casa"])
		})
	}
}

// TestLoadSinLoansSection: una config sin sección loans carga el mapa vacío,
// nunca nil (el resto del programa puede recorrerlo sin comprobar).
func TestLoadSinLoansSection(t *testing.T) {
	path := writeBytes(t, t.TempDir(), "config.toml", []byte(validTOML))
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.Loans == nil || len(got.Loans) != 0 {
		t.Fatalf("Loans = %#v, want mapa vacío no nil", got.Loans)
	}
}
