package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aam9063/hipoteca-cli/internal/errs"
)

// ----------------------------------------------------------------------------
// `uninstall` en la capa de orquestación (REF-03, S1-S4): borrar la config
// (con los préstamos), la base de datos y la caché, diciendo exactamente qué
// rutas se han tocado. Sin confirmación no se borra nada; --dry-run lista
// tamaños sin tocar nada; el binario nunca entra en la lista (S4).
// ----------------------------------------------------------------------------

// uninstallFixture crea las tres rutas con contenido conocido en un HOME
// temporal: config.toml con un préstamo (el dato que importa), hipoteca.db y
// un directorio de caché con un fichero dentro.
func uninstallFixture(t *testing.T) (cfgPath, dbPath, cacheDir string) {
	t.Helper()
	base := t.TempDir()
	cfgPath = filepath.Join(base, "config.toml")
	dbPath = filepath.Join(base, "hipoteca.db")
	cacheDir = filepath.Join(base, "cache")

	cfg := "schema_version = 1\n\n[loans.casa]\nname = \"Casa\"\n"
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
		t.Fatalf("setup config: %v", err)
	}
	if err := os.WriteFile(dbPath, []byte("watch state"), 0o600); err != nil {
		t.Fatalf("setup db: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(cacheDir, "series"), 0o700); err != nil {
		t.Fatalf("setup cache: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "series", "euribor-1y.json"), []byte("0123456789"), 0o600); err != nil {
		t.Fatalf("setup cache file: %v", err)
	}
	return cfgPath, dbPath, cacheDir
}

// mustContain busca kind en el resultado y lo devuelve.
func mustContain(t *testing.T, res UninstallResult, kind string) UninstallTarget {
	t.Helper()
	for _, target := range res.Targets {
		if target.Kind == kind {
			return target
		}
	}
	t.Fatalf("el resultado no incluye ningún objetivo %q: %+v", kind, res.Targets)
	return UninstallTarget{}
}

// TestUninstallRefusesWithoutYes: sin --yes responde confirmation_required/2
// y NO borra nada (S2): ni la config con los préstamos, ni la base, ni la
// caché.
func TestUninstallRefusesWithoutYes(t *testing.T) {
	cfgPath, dbPath, cacheDir := uninstallFixture(t)

	_, err := Uninstall(cfgPath, dbPath, cacheDir, false, false)
	e, ok := errs.From(err)
	if !ok {
		t.Fatalf("Uninstall() sin --yes: error = %v, queria confirmation_required", err)
	}
	if e.Error.Type != string(errs.KindConfirmationRequired) {
		t.Fatalf("kind = %q, want confirmation_required", e.Error.Type)
	}
	if !strings.Contains(e.Error.Hint, "--yes") {
		t.Fatalf("el hint debe apuntar a --yes: %q", e.Error.Hint)
	}
	// Nada se ha borrado.
	for _, p := range []string{cfgPath, dbPath, cacheDir} {
		if _, statErr := os.Stat(p); statErr != nil {
			t.Fatalf("%s debía seguir existiendo tras el rechazo: %v", p, statErr)
		}
	}
}

// TestUninstallDryRunListsSizesWithoutDeleting: --dry-run enumera las tres
// rutas con su tamaño y no borra nada (S3). El tamaño es el del contenido
// real, también para el directorio de caché.
func TestUninstallDryRunListsSizesWithoutDeleting(t *testing.T) {
	cfgPath, dbPath, cacheDir := uninstallFixture(t)

	res, err := Uninstall(cfgPath, dbPath, cacheDir, false, true)
	if err != nil {
		t.Fatalf("Uninstall() dry-run: %v", err)
	}
	if !res.DryRun {
		t.Fatal("DryRun = false, want true")
	}
	if len(res.Targets) != 3 {
		t.Fatalf("Targets = %d entradas, want 3 (config, data, cache)", len(res.Targets))
	}
	cfgTarget := mustContain(t, res, "config")
	if cfgTarget.Path != cfgPath || !cfgTarget.Exists || cfgTarget.Removed {
		t.Fatalf("config = %+v", cfgTarget)
	}
	if cfgTarget.SizeBytes != int64(len("schema_version = 1\n\n[loans.casa]\nname = \"Casa\"\n")) {
		t.Fatalf("config size = %d, want el tamaño exacto del fichero", cfgTarget.SizeBytes)
	}
	dbTarget := mustContain(t, res, "data")
	if dbTarget.Path != dbPath || dbTarget.SizeBytes != int64(len("watch state")) {
		t.Fatalf("data = %+v", dbTarget)
	}
	cacheTarget := mustContain(t, res, "cache")
	if cacheTarget.Path != cacheDir || cacheTarget.SizeBytes != 10 {
		t.Fatalf("cache = %+v, want el tamaño sumado de su contenido (10 bytes)", cacheTarget)
	}
	// Nada se ha borrado.
	for _, p := range []string{cfgPath, dbPath, cacheDir} {
		if _, statErr := os.Stat(p); statErr != nil {
			t.Fatalf("%s debía seguir existiendo tras el dry-run: %v", p, statErr)
		}
	}
}

// TestUninstallDryRunListsMissingPaths: una ruta que no existe se lista con
// exists false y tamaño 0, sin error: es el estado de una instalación nueva.
func TestUninstallDryRunListsMissingPaths(t *testing.T) {
	base := t.TempDir()
	res, err := Uninstall(
		filepath.Join(base, "config.toml"),
		filepath.Join(base, "hipoteca.db"),
		filepath.Join(base, "cache"),
		false, true,
	)
	if err != nil {
		t.Fatalf("Uninstall() dry-run sin nada instalado: %v", err)
	}
	for _, target := range res.Targets {
		if target.Exists || target.SizeBytes != 0 || target.Removed {
			t.Fatalf("objetivo %s = %+v, want exists false, size 0, removed false", target.Kind, target)
		}
	}
}

// TestUninstallRemovesEverythingWithYes: con --yes borra las tres rutas y el
// resultado dice exactamente qué ha eliminado (S1).
func TestUninstallRemovesEverythingWithYes(t *testing.T) {
	cfgPath, dbPath, cacheDir := uninstallFixture(t)

	res, err := Uninstall(cfgPath, dbPath, cacheDir, true, false)
	if err != nil {
		t.Fatalf("Uninstall() --yes: %v", err)
	}
	if res.DryRun {
		t.Fatal("DryRun = true, want false")
	}
	for _, target := range res.Targets {
		if !target.Exists || !target.Removed {
			t.Fatalf("objetivo %s = %+v, want exists y removed a true", target.Kind, target)
		}
	}
	for _, p := range []string{cfgPath, dbPath, cacheDir} {
		if _, statErr := os.Stat(p); !os.IsNotExist(statErr) {
			t.Fatalf("%s debía estar borrado (stat: %v)", p, statErr)
		}
	}
}

// TestUninstallWithYesWhenNothingExists: borrar una instalación que no ha
// guardado nada no es un error; el resultado lo declara.
func TestUninstallWithYesWhenNothingExists(t *testing.T) {
	base := t.TempDir()
	res, err := Uninstall(
		filepath.Join(base, "config.toml"),
		filepath.Join(base, "hipoteca.db"),
		filepath.Join(base, "cache"),
		true, false,
	)
	if err != nil {
		t.Fatalf("Uninstall() --yes sin nada instalado: %v", err)
	}
	for _, target := range res.Targets {
		if target.Exists || target.Removed {
			t.Fatalf("objetivo %s = %+v, want exists y removed a false", target.Kind, target)
		}
	}
}

// TestUninstallNeverTouchesSiblingFiles: el borrado es exacto: los tres
// objetivos son rutas concretas y un fichero hermano del binario o de la
// config no se toca (S4: el binario nunca lo borra uninstall).
func TestUninstallNeverTouchesSiblingFiles(t *testing.T) {
	cfgPath, dbPath, cacheDir := uninstallFixture(t)
	sibling := filepath.Join(filepath.Dir(cfgPath), "hipoteca.exe")
	if err := os.WriteFile(sibling, []byte("binary"), 0o600); err != nil {
		t.Fatalf("setup binario hermano: %v", err)
	}

	if _, err := Uninstall(cfgPath, dbPath, cacheDir, true, false); err != nil {
		t.Fatalf("Uninstall() --yes: %v", err)
	}
	if _, statErr := os.Stat(sibling); statErr != nil {
		t.Fatalf("el binario hermano debía seguir existiendo: %v", statErr)
	}
}
