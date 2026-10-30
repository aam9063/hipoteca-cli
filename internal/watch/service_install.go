package watch

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// ServiceRunner ejecuta el gestor de servicios. Se inyecta para que los tests
// no toquen un systemd o un launchd reales: probar el contenido de la unidad
// es determinista, hablar con el gestor del sistema no lo es.
type ServiceRunner func(name string, args ...string) error

// ServiceState es lo que `watch service status` responde.
type ServiceState struct {
	Installed bool   `json:"installed"`
	Active    bool   `json:"active"`
	Manager   string `json:"manager"`
	Path      string `json:"path"`
	Name      string `json:"name"`
	Note      string `json:"note,omitempty"`
}

// ServiceInstall escribe la unidad y la activa. Es idempotente: volver a
// instalarla reescribe el mismo contenido sin duplicar nada, y el gestor
// recarga en lugar de acumular.
func ServiceInstall(unit ServiceUnit, run ServiceRunner) (ServiceState, error) {
	if err := os.MkdirAll(filepath.Dir(unit.Path), 0o700); err != nil {
		return ServiceState{}, fmt.Errorf("watch: creando el directorio del servicio %q: %w", filepath.Dir(unit.Path), err)
	}
	if err := os.WriteFile(unit.Path, []byte(unit.Content), 0o600); err != nil {
		return ServiceState{}, fmt.Errorf("watch: escribiendo la unidad %q: %w", unit.Path, err)
	}

	switch unit.Manager {
	case "systemd":
		if err := run("systemctl", "--user", "daemon-reload"); err != nil {
			return ServiceState{}, fmt.Errorf("watch: recargando systemd: %w", err)
		}
		if err := run("systemctl", "--user", "enable", "--now", unit.Name); err != nil {
			return ServiceState{}, fmt.Errorf("watch: activando %s: %w", unit.Name, err)
		}
	case "launchd":
		// Un agente ya cargado no se vuelve a cargar: primero se descarga para
		// que launchd lea el plist nuevo. Si no estaba cargado, el error de
		// unload es esperado y no se propaga.
		_ = run("launchctl", "unload", unit.Path)
		if err := run("launchctl", "load", "-w", unit.Path); err != nil {
			return ServiceState{}, fmt.Errorf("watch: cargando %s: %w", unit.Name, err)
		}
	default:
		return ServiceState{}, fmt.Errorf("watch: gestor de servicios desconocido %q", unit.Manager)
	}

	return ServiceState{
		Installed: true,
		Active:    true,
		Manager:   unit.Manager,
		Path:      unit.Path,
		Name:      unit.Name,
	}, nil
}

// ServiceUninstall retira todo lo que puso ServiceInstall. Sin restos: en
// systemd eso incluye el estado de "enabled", que vive fuera del archivo de la
// unidad y sobreviviría a borrarlo; en launchd, el agente tiene que estar
// descargado antes de que su plist desaparezca. Desinstalar dos veces no falla.
func ServiceUninstall(unit ServiceUnit, run ServiceRunner) (ServiceState, error) {
	existed := true
	if _, err := os.Stat(unit.Path); errors.Is(err, fs.ErrNotExist) {
		existed = false
	}

	switch unit.Manager {
	case "systemd":
		// disable y stop pueden fallar si el servicio no estaba activo: eso no
		// es un error de la desinstalación.
		_ = run("systemctl", "--user", "disable", "--now", unit.Name)
		if err := run("systemctl", "--user", "daemon-reload"); err != nil {
			return ServiceState{}, fmt.Errorf("watch: recargando systemd: %w", err)
		}
	case "launchd":
		_ = run("launchctl", "unload", "-w", unit.Path)
	default:
		return ServiceState{}, fmt.Errorf("watch: gestor de servicios desconocido %q", unit.Manager)
	}

	if err := os.Remove(unit.Path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return ServiceState{}, fmt.Errorf("watch: borrando la unidad %q: %w", unit.Path, err)
	}
	// Si el directorio lo creamos nosotros y quedó vacío, se va también: es
	// parte de "sin restos".
	_ = os.Remove(filepath.Dir(unit.Path))

	return ServiceState{
		Installed: false,
		Active:    false,
		Manager:   unit.Manager,
		Path:      unit.Path,
		Name:      unit.Name,
		Note:      noteFor(existed),
	}, nil
}

// ServiceStatusOf informa sin cambiar nada.
func ServiceStatusOf(unit ServiceUnit) (ServiceState, error) {
	_, err := os.Stat(unit.Path)
	installed := err == nil
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return ServiceState{}, fmt.Errorf("watch: consultando la unidad %q: %w", unit.Path, err)
	}
	state := ServiceState{
		Installed: installed,
		Active:    installed, // el archivo es la prueba de que el gestor la conoce
		Manager:   unit.Manager,
		Path:      unit.Path,
		Name:      unit.Name,
	}
	if !installed {
		state.Note = "el servicio no está instalado"
	}
	return state, nil
}

func noteFor(existed bool) string {
	if !existed {
		return "el servicio no estaba instalado; no había nada que retirar"
	}
	return ""
}
