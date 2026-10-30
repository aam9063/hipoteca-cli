package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/aam9063/hipoteca-cli/internal/bde"
	"github.com/aam9063/hipoteca-cli/internal/config"
	"github.com/aam9063/hipoteca-cli/internal/loan"
)

// escribeConfig crea una config temporal con los préstamos dados (o ninguna)
// y devuelve la ruta; usa config.Save, la escritura atómica de producción.
func escribeConfig(t *testing.T, loans map[string]loan.Loan) string {
	t.Helper()
	cfg := config.Default()
	for id, l := range loans {
		cfg.Loans[id] = l
	}
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := cfg.Save(path); err != nil {
		t.Fatalf("no se pudo escribir la config de prueba: %v", err)
	}
	return path
}

// doctorPrestamoCompleto es un préstamo variable con todos los campos de
// escritura declarados (docs/ARRANQUE.md §4.1); no reutiliza el helper de
// loan_test.go para que estas pruebas sigan leyéndose solas.
func doctorPrestamoCompleto() loan.Loan {
	cero := 0
	sinSuelo := ""
	return loan.Loan{
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
}

// AGENT-01 S5: doctor responde el estado completo: rutas, catálogo,
// préstamos guardados y qué campo le falta a cada incompleto.
func TestDoctorReportsPathsCatalogAndLoans(t *testing.T) {
	incompleto := doctorPrestamoCompleto()
	incompleto.FloorRate = nil // sin declarar el suelo: préstamo incompleto
	cfgPath := escribeConfig(t, map[string]loan.Loan{"casa": incompleto})
	dir := t.TempDir()

	dialHosts := []string{}
	res, err := Doctor(context.Background(), DoctorParams{
		ConfigPath: cfgPath,
		DataPath:   filepath.Join(dir, "hipoteca.db"),
		CacheDir:   dir,
		Version:    "1.2.3",
		Commit:     "abc1234",
		Dial: func(_ context.Context, host string) error {
			dialHosts = append(dialHosts, host)
			return nil
		},
	})
	if err != nil {
		t.Fatalf("Doctor no debe fallar, got %v", err)
	}
	if res.Version != "1.2.3" || res.Commit != "abc1234" {
		t.Errorf("version/commit = %q/%q, want 1.2.3/abc1234", res.Version, res.Commit)
	}
	if res.ConfigPath != cfgPath || !res.ConfigExists {
		t.Errorf("config_path = %q (exists=%v), want %q true", res.ConfigPath, res.ConfigExists, cfgPath)
	}
	// Permiso correcto: config.Save escribe 0600; fuera de Windows (que no
	// lleva los bits de Unix) el modo debe llegar "0600".
	if runtime.GOOS != "windows" {
		if res.ConfigMode != "0600" {
			t.Errorf("config_mode = %q, want 0600", res.ConfigMode)
		}
	}
	if res.DataPath == "" || res.CacheDir == "" {
		t.Errorf("data_path y cache_dir deben venir siempre, got %q/%q", res.DataPath, res.CacheDir)
	}

	// Catálogo completo, con id y código de cada índice.
	catalog, err := bde.LoadCatalog(nil)
	if err != nil {
		t.Fatalf("catálogo embebido no válido: %v", err)
	}
	if len(res.Indices) != len(catalog.IDs()) {
		t.Fatalf("doctor lista %d índices, want %d", len(res.Indices), len(catalog.IDs()))
	}
	for i, idx := range res.Indices {
		if idx.ID == "" || idx.Code == "" {
			t.Errorf("índice %d sin id o código: %+v", i, idx)
		}
	}

	// Préstamos: el incompleto nombra el campo que le falta (S5).
	byID := map[string]DoctorLoan{}
	for _, l := range res.Loans {
		byID[l.ID] = l
	}
	got, ok := byID["casa"]
	if !ok {
		t.Fatalf("doctor no lista el préstamo casa: %+v", res.Loans)
	}
	if got.Complete {
		t.Error("el préstamo sin floor_rate no puede salir como completo")
	}
	found := false
	for _, campo := range got.MissingFields {
		if campo == "floor_rate" {
			found = true
		}
	}
	if !found {
		t.Errorf("missing_fields = %v, debe nombrar floor_rate", got.MissingFields)
	}

	// Red: una sola comprobación, contra el host del catálogo.
	if !res.Network.Checked || !res.Network.OK {
		t.Errorf("network = %+v, want checked y ok", res.Network)
	}
	if len(dialHosts) != 1 {
		t.Fatalf("la comprobación de red debe ser UNA, hubo %d (%v)", len(dialHosts), dialHosts)
	}
	if !strings.Contains(dialHosts[0], "app.bde.es") {
		t.Errorf("la comprobación debe apuntar al host del catálogo, got %q", dialHosts[0])
	}
	if res.Network.Host == "" {
		t.Error("el host comprobado debe salir en la respuesta")
	}
}

// AGENT-01 S5: con --offline no se toca la red: ni una conexión.
func TestDoctorOfflineNeverDials(t *testing.T) {
	cfgPath := escribeConfig(t, nil)
	called := false
	res, err := Doctor(context.Background(), DoctorParams{
		ConfigPath: cfgPath,
		DataPath:   filepath.Join(t.TempDir(), "hipoteca.db"),
		CacheDir:   t.TempDir(),
		Offline:    true,
		Dial: func(context.Context, string) error {
			called = true
			return nil
		},
	})
	if err != nil {
		t.Fatalf("Doctor --offline no debe fallar, got %v", err)
	}
	if called {
		t.Error("con --offline no se puede marcar a nadie")
	}
	if res.Network.Checked {
		t.Errorf("network = %+v, want checked=false", res.Network)
	}
}

// AGENT-01 S5: la red caída se informa en la respuesta; doctor sigue siendo
// un diagnóstico que funciona (no es un fallo del comando).
func TestDoctorReportsUnreachableNetwork(t *testing.T) {
	cfgPath := escribeConfig(t, nil)
	res, err := Doctor(context.Background(), DoctorParams{
		ConfigPath: cfgPath,
		DataPath:   filepath.Join(t.TempDir(), "hipoteca.db"),
		CacheDir:   t.TempDir(),
		Dial: func(context.Context, string) error {
			return errors.New("connection refused")
		},
	})
	if err != nil {
		t.Fatalf("Doctor no debe fallar aunque la red esté caída, got %v", err)
	}
	if res.Network.OK {
		t.Errorf("network.ok = true con la red caída: %+v", res.Network)
	}
	if res.Network.Error == "" {
		t.Error("la causa debe salir en network.error")
	}
}

// AGENT-01 S5: una config corrupta no tira el diagnóstico: avisa y sigue.
func TestDoctorWarnsOnCorruptConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("esto no es toml"), 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := Doctor(context.Background(), DoctorParams{
		ConfigPath: path,
		DataPath:   filepath.Join(dir, "hipoteca.db"),
		CacheDir:   dir,
		Offline:    true,
	})
	if err != nil {
		t.Fatalf("Doctor debe sobrevivir a una config corrupta, got %v", err)
	}
	if len(res.Warnings) == 0 {
		t.Error("la config corrupta debe aparecer en warnings")
	}
	if len(res.Loans) != 0 {
		t.Errorf("sin config legible no hay préstamos que listar, got %+v", res.Loans)
	}
}

// AGENT-01 S7: un permiso incorrecto en la config se avisa (sólo tiene
// sentido en sistemas con bits de permiso de Unix).
func TestDoctorWarnsOnWrongConfigPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows no lleva los bits de permiso de Unix")
	}
	cfgPath := escribeConfig(t, nil)
	if err := os.Chmod(cfgPath, 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := Doctor(context.Background(), DoctorParams{
		ConfigPath: cfgPath,
		DataPath:   filepath.Join(t.TempDir(), "hipoteca.db"),
		CacheDir:   t.TempDir(),
		Offline:    true,
	})
	if err != nil {
		t.Fatalf("Doctor no debe fallar, got %v", err)
	}
	hay := false
	for _, w := range res.Warnings {
		if strings.Contains(w, "0600") && strings.Contains(w, "0644") {
			hay = true
		}
	}
	if !hay {
		t.Errorf("warnings = %v, debe avisar del permiso 0644 frente a 0600", res.Warnings)
	}
	if res.ConfigMode != "0644" {
		t.Errorf("config_mode = %q, want 0644", res.ConfigMode)
	}
}
