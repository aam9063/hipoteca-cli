package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aam9063/hipoteca-cli/internal/errs"
)

// ----------------------------------------------------------------------------
// E2E de `hipoteca uninstall` (REF-03): confirmación explícita, dry-run que
// no borra nada y borrado real de config, base de datos y caché. Las rutas
// van inyectadas a un temporal: ningún test toca los datos del usuario.
// ----------------------------------------------------------------------------

// uninstallDeps construye el árbol con las tres rutas en un temporal y
// escribe en cada una el contenido indicado (vacío = no crear).
func uninstallDeps(t *testing.T) (Deps, string) {
	t.Helper()
	base := t.TempDir()
	cfgPath := filepath.Join(base, "config.toml")
	dbPath := filepath.Join(base, "hipoteca.db")
	cacheDir := filepath.Join(base, "cache")
	deps := Deps{
		ConfigPath: func() (string, error) { return cfgPath, nil },
		DataPath:   func() (string, error) { return dbPath, nil },
		CacheDir:   func() (string, error) { return cacheDir, nil },
	}
	if err := os.WriteFile(cfgPath, []byte("schema_version = 1\n"), 0o600); err != nil {
		t.Fatalf("setup config: %v", err)
	}
	if err := os.WriteFile(dbPath, []byte("estado de vigilancias"), 0o600); err != nil {
		t.Fatalf("setup db: %v", err)
	}
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		t.Fatalf("setup cache: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "euribor-1y.json"), []byte("0123456789"), 0o600); err != nil {
		t.Fatalf("setup cache file: %v", err)
	}
	return deps, base
}

// TestE2EUninstallRefusesWithoutYes: sin --yes responde confirmation_required/2
// en el sobre del contrato y no borra nada (S2).
func TestE2EUninstallRefusesWithoutYes(t *testing.T) {
	deps, base := uninstallDeps(t)

	stdout, _, err := runRoot(t, deps, "uninstall")
	e := assertContractError(t, err, errs.KindConfirmationRequired, 2)
	assertEmpty(t, "stdout", stdout)
	if !strings.Contains(e.Hint, "--yes") {
		t.Fatalf("el hint debe apuntar a --yes: %q", e.Hint)
	}
	// Nada se ha borrado: la config con los préstamos sigue en su sitio.
	if _, statErr := os.Stat(filepath.Join(base, "config.toml")); statErr != nil {
		t.Fatalf("la config debía seguir existiendo: %v", statErr)
	}
}

// TestE2EUninstallDryRunListsWithoutDeleting: --dry-run emite un único JSON
// con las tres rutas, sus tamaños y dry_run true, y no borra nada (S3).
func TestE2EUninstallDryRunListsWithoutDeleting(t *testing.T) {
	deps, base := uninstallDeps(t)

	stdout, _, err := runRoot(t, deps, "uninstall", "--dry-run")
	if err != nil {
		t.Fatalf("uninstall --dry-run: %v", err)
	}
	assertSingleJSON(t, stdout)
	var res struct {
		SchemaVersion int  `json:"schema_version"`
		DryRun        bool `json:"dry_run"`
		Targets       []struct {
			Kind      string `json:"kind"`
			Path      string `json:"path"`
			Exists    bool   `json:"exists"`
			SizeBytes int64  `json:"size_bytes"`
			Removed   bool   `json:"removed"`
		} `json:"targets"`
	}
	if err := json.Unmarshal([]byte(stdout), &res); err != nil {
		t.Fatalf("la salida no es JSON válido: %v\n%s", err, stdout)
	}
	if !res.DryRun || len(res.Targets) != 3 {
		t.Fatalf("dry_run = %v, targets = %d; salida: %s", res.DryRun, len(res.Targets), stdout)
	}
	kinds := map[string]bool{}
	for _, target := range res.Targets {
		kinds[target.Kind] = true
		if !target.Exists || target.Removed || target.SizeBytes == 0 {
			t.Fatalf("objetivo %s = %+v: debía existir, medir y no borrarse", target.Kind, target)
		}
	}
	for _, kind := range []string{"config", "data", "cache"} {
		if !kinds[kind] {
			t.Fatalf("falta el objetivo %q en la salida: %s", kind, stdout)
		}
	}
	for _, name := range []string{"config.toml", "hipoteca.db", "cache"} {
		if _, statErr := os.Stat(filepath.Join(base, name)); statErr != nil {
			t.Fatalf("%s debía seguir existiendo tras el dry-run: %v", name, statErr)
		}
	}
}

// TestE2EUninstallWithYesDeletes: con --yes borra las tres rutas y la salida
// lo declara removed true (S1).
func TestE2EUninstallWithYesDeletes(t *testing.T) {
	deps, base := uninstallDeps(t)

	stdout, _, err := runRoot(t, deps, "uninstall", "--yes")
	if err != nil {
		t.Fatalf("uninstall --yes: %v", err)
	}
	assertSingleJSON(t, stdout)
	if !strings.Contains(stdout, `"removed":true`) {
		t.Fatalf("la salida debe declarar el borrado: %s", stdout)
	}
	for _, name := range []string{"config.toml", "hipoteca.db", "cache"} {
		if _, statErr := os.Stat(filepath.Join(base, name)); !os.IsNotExist(statErr) {
			t.Fatalf("%s debía estar borrado (stat: %v)", name, statErr)
		}
	}
}

// TestE2EUninstallHelpSaysBinaryIsNotRemoved (S4): la ayuda dice que el
// binario no se toca y a quién pertenece su retirada.
func TestE2EUninstallHelpSaysBinaryIsNotRemoved(t *testing.T) {
	deps, _ := uninstallDeps(t)
	stdout, _, err := runRoot(t, deps, "uninstall", "--help")
	if err != nil {
		t.Fatalf("uninstall --help: %v", err)
	}
	if !strings.Contains(stdout, "binario") || !strings.Contains(stdout, "gestor de paquetes") {
		t.Fatalf("la ayuda debe decir que el binario lo retira el gestor de paquetes:\n%s", stdout)
	}
}
