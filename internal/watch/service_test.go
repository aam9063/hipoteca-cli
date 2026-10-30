package watch

import (
	"errors"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const execAbs = "/opt/hipoteca/bin/hipoteca"

// TestServiceUnitLinuxUnRutaAbsoluta garantiza que la unidad no dependa del
// PATH: un servicio no lo hereda, así que "hipoteca" a secas funcionaría hasta
// el primer reinicio y después moriría sin decir nada.
func TestServiceUnitLinuxUnRutaAbsoluta(t *testing.T) {
	unit, err := ServiceUnitFor("linux", "/home/ana", execAbs, "/home/ana/.local/share", 12*time.Hour)
	if err != nil {
		t.Fatalf("ServiceUnitFor: %v", err)
	}
	if unit.Manager != "systemd" {
		t.Fatalf("gestor = %q, se esperaba systemd", unit.Manager)
	}
	if !strings.Contains(unit.Content, "ExecStart="+execAbs+" watch run --interval 12h") {
		t.Fatalf("ExecStart no lleva la ruta absoluta:\n%s", unit.Content)
	}
	if !strings.Contains(unit.Content, "Environment=XDG_DATA_HOME=/home/ana/.local/share") {
		t.Fatalf("la unidad no fija el directorio de datos:\n%s", unit.Content)
	}
	want := path.Join("/home/ana", ".config", "systemd", "user", "hipoteca-watch.service")
	if unit.Path != want {
		t.Fatalf("ruta = %q, se esperaba %q", unit.Path, want)
	}
}

func TestServiceUnitDarwinEsUnAgenteDeLaunchd(t *testing.T) {
	unit, err := ServiceUnitFor("darwin", "/Users/ana", "/usr/local/bin/hipoteca", "/Users/ana/Library/Application Support", 6*time.Hour)
	if err != nil {
		t.Fatalf("ServiceUnitFor: %v", err)
	}
	if unit.Manager != "launchd" {
		t.Fatalf("gestor = %q, se esperaba launchd", unit.Manager)
	}
	if !strings.Contains(unit.Content, "<string>/usr/local/bin/hipoteca</string>") {
		t.Fatalf("el plist no lleva la ruta absoluta:\n%s", unit.Content)
	}
	if !strings.Contains(unit.Content, "<key>Label</key>") {
		t.Fatalf("el plist no declara Label:\n%s", unit.Content)
	}
	if !strings.Contains(unit.Content, "<string>6h</string>") {
		t.Fatalf("el plist no lleva el intervalo:\n%s", unit.Content)
	}
	if !strings.HasSuffix(unit.Path, path.Join("Library", "LaunchAgents", "com.aam9063.hipoteca.watch.plist")) {
		t.Fatalf("ruta inesperada %q", unit.Path)
	}
}

// TestServiceUnitRechazaLoQueRompeElServicio cubre las tres entradas que
// producirían un servicio roto en silencio.
func TestServiceUnitRechazaLoQueRompeElServicio(t *testing.T) {
	if _, err := ServiceUnitFor("linux", "/home/ana", "hipoteca", "/d", time.Hour); err == nil {
		t.Fatal("una ruta relativa del ejecutable debe rechazarse")
	}
	if _, err := ServiceUnitFor("linux", "/home/ana", execAbs, "/d", 30*time.Minute); err == nil {
		t.Fatal("un intervalo menor de una hora debe rechazarse")
	}
	if _, err := ServiceUnitFor("windows", `C:\Users\ana`, `C:\hipoteca.exe`, `C:\d`, time.Hour); !errors.Is(err, ErrServiceUnsupported) {
		t.Fatalf("Windows debe dar ErrServiceUnsupported, dio %v", err)
	}
}

func fakeRunner(calls *[]string) ServiceRunner {
	return func(name string, args ...string) error {
		*calls = append(*calls, name+" "+strings.Join(args, " "))
		return nil
	}
}

// TestServiceInstallEsIdempotente instala dos veces y comprueba que no acumula
// unidades ni deja el gestor en un estado raro.
func TestServiceInstallEsIdempotente(t *testing.T) {
	unit := hostUnit(t)
	var calls []string
	run := fakeRunner(&calls)

	first, err := ServiceInstall(unit, run)
	if err != nil {
		t.Fatalf("primera instalación: %v", err)
	}
	second, err := ServiceInstall(unit, run)
	if err != nil {
		t.Fatalf("segunda instalación: %v", err)
	}
	if !first.Installed || !second.Installed {
		t.Fatal("instalar debe dejar el servicio instalado las dos veces")
	}
	content, err := os.ReadFile(unit.Path)
	if err != nil {
		t.Fatalf("leyendo la unidad: %v", err)
	}
	if string(content) != unit.Content {
		t.Fatal("la unidad escrita no coincide con la generada")
	}
	if matches, _ := filepath.Glob(filepath.Join(filepath.Dir(unit.Path), "*.service")); len(matches) != 1 {
		t.Fatalf("hay %d unidades, se esperaba exactamente una: %v", len(matches), matches)
	}
	var enables int
	for _, c := range calls {
		if strings.Contains(c, "enable") {
			enables++
		}
	}
	if enables != 2 {
		t.Fatalf("se esperaba un enable por instalación, hubo %d", enables)
	}
}

// TestServiceUninstallNoDejaRestos es el criterio de aceptación de la tarea:
// fuera el archivo, fuera el directorio, y el estado de systemd desactivado
// (que vive fuera del archivo y sobreviviría a borrarlo).
func TestServiceUninstallNoDejaRestos(t *testing.T) {
	unit := hostUnit(t)
	var calls []string
	run := fakeRunner(&calls)
	if _, err := ServiceInstall(unit, run); err != nil {
		t.Fatalf("instalando: %v", err)
	}

	state, err := ServiceUninstall(unit, run)
	if err != nil {
		t.Fatalf("desinstalando: %v", err)
	}
	if state.Installed {
		t.Fatal("el estado debe quedar como no instalado")
	}
	if _, err := os.Stat(unit.Path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("la unidad sigue ahí: %v", err)
	}
	if _, err := os.Stat(filepath.Dir(unit.Path)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("el directorio creado por install sigue ahí: %v", err)
	}
	var disabled bool
	for _, c := range calls {
		if strings.Contains(c, "disable") {
			disabled = true
		}
	}
	if !disabled {
		t.Fatalf("no se desactivó el servicio antes de borrar la unidad: %v", calls)
	}
}

func TestServiceUninstallDosVecesNoFalla(t *testing.T) {
	unit := hostUnit(t)
	var calls []string
	run := fakeRunner(&calls)
	second, err := ServiceUninstall(unit, run)
	if err != nil {
		t.Fatalf("desinstalar algo que no estaba debe funcionar: %v", err)
	}
	if second.Note == "" {
		t.Fatal("conviene decir que no había nada que retirar")
	}
}

func TestServiceStatusReflejaElArchivo(t *testing.T) {
	unit := hostUnit(t)
	before, err := ServiceStatusOf(unit)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if before.Installed {
		t.Fatal("no debería estar instalado antes de instalar")
	}
	var calls []string
	if _, err := ServiceInstall(unit, fakeRunner(&calls)); err != nil {
		t.Fatalf("instalando: %v", err)
	}
	after, err := ServiceStatusOf(unit)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if !after.Installed {
		t.Fatal("debería estar instalado después de instalar")
	}
}

// hostUnit arma una unidad sobre el sistema de archivos real del test. Los
// tests de generación usan ServiceUnitFor con el SO destino; los de escritura
// no, para no mezclar "qué contenido toca" con "dónde vive este archivo aquí".
func hostUnit(t *testing.T) ServiceUnit {
	t.Helper()
	return ServiceUnit{
		Manager: "systemd",
		Name:    "hipoteca-watch",
		Path:    filepath.Join(t.TempDir(), "hipoteca-watch.service"),
		Content: "[Unit]\nDescription=prueba\n",
	}
}
