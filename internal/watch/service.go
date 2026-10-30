package watch

import (
	"errors"
	"fmt"
	"path"
	"strings"
	"time"
)

// ErrServiceUnsupported: el sistema operativo no tiene un servicio de usuario
// soportado. Se responde como usage, no como error interno: no es que el
// programa se rompa, es que ese sistema no ofrece dónde engancharlo.
var ErrServiceUnsupported = errors.New("watch: este sistema no tiene servicio de usuario soportado")

// ErrServiceNoWatches: instalar un servicio que no vigila nada sería una
// trampa silenciosa, así que se rechaza antes de escribir la unidad.
var ErrServiceNoWatches = errors.New("watch: no hay ninguna vigilancia que el servicio pueda atender")

// ServiceUnit es la unidad del servicio del usuario: qué gestor la controla,
// dónde vive, cómo se llama y qué contiene. Se calcula sin tocar el disco.
type ServiceUnit struct {
	Manager string // "systemd" o "launchd"
	Name    string // nombre lógico que entiende el gestor
	Path    string // ruta absoluta del fichero de la unidad
	Content string // contenido exacto del fichero
}

// ServiceIntervalText da el intervalo en la forma más corta que el gestor
// entiende: 12h y no 12h0m0s.
func ServiceIntervalText(interval time.Duration) string {
	if interval%time.Hour == 0 {
		return fmt.Sprintf("%dh", int(interval.Hours()))
	}
	if interval%time.Minute == 0 {
		return fmt.Sprintf("%dm", int(interval.Minutes()))
	}
	return interval.String()
}

// ServiceUnitFor calcula la unidad del servicio para un sistema operativo.
// execPath debe ser la ruta absoluta del ejecutable (os.Executable): un
// servicio no hereda el PATH del usuario, así que "hipoteca" a secas
// funcionaría hasta el primer reinicio y después moriría en silencio.
// dataPath se fija en el entorno de la unidad por la misma razón: el servicio
// debe encontrar la misma base de datos que vio el usuario al configurarlo.
func ServiceUnitFor(goos, home, execPath, dataPath string, interval time.Duration) (ServiceUnit, error) {
	switch goos {
	case "linux", "darwin":
	default:
		return ServiceUnit{}, ErrServiceUnsupported
	}
	if interval < time.Hour {
		return ServiceUnit{}, fmt.Errorf("watch: el intervalo del servicio debe ser de al menos una hora, no %s", interval)
	}
	// La ruta se valida con la semántica del sistema DESTINO, no la del que
	// compila: generar una unidad de Linux desde Windows es legítimo y
	// filepath.IsAbs diría que /opt/hipoteca no es absoluta.
	if !strings.HasPrefix(execPath, "/") {
		return ServiceUnit{}, fmt.Errorf("watch: el ejecutable debe ser una ruta absoluta, no %q", execPath)
	}
	if home == "" {
		return ServiceUnit{}, errors.New("watch: no se pudo determinar el directorio del usuario")
	}

	name := "hipoteca-watch"
	label := "com.aam9063.hipoteca.watch"
	every := ServiceIntervalText(interval)

	switch goos {
	case "linux":
		content := strings.Join([]string{
			"[Unit]",
			"Description=hipoteca-cli: aviso de índice publicado",
			"After=network-online.target",
			"",
			"[Service]",
			"Type=simple",
			"Environment=XDG_DATA_HOME=" + dataPath,
			"ExecStart=" + execPath + " watch run --interval " + every,
			"Restart=on-failure",
			"RestartSec=1h",
			"",
			"[Install]",
			"WantedBy=default.target",
			"",
		}, "\n")
		return ServiceUnit{
			Manager: "systemd",
			Name:    name,
			Path:    path.Join(home, ".config", "systemd", "user", name+".service"),
			Content: content,
		}, nil
	case "darwin":
		content := strings.Join([]string{
			`<?xml version="1.0" encoding="UTF-8"?>`,
			`<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">`,
			`<plist version="1.0">`,
			`<dict>`,
			`	<key>Label</key>`,
			`	<string>` + label + `</string>`,
			`	<key>ProgramArguments</key>`,
			`	<array>`,
			`		<string>` + xmlEscape(execPath) + `</string>`,
			`		<string>watch</string>`,
			`		<string>run</string>`,
			`		<string>--interval</string>`,
			`		<string>` + xmlEscape(every) + `</string>`,
			`	</array>`,
			`	<key>EnvironmentVariables</key>`,
			`	<dict>`,
			`		<key>XDG_DATA_HOME</key>`,
			`		<string>` + xmlEscape(dataPath) + `</string>`,
			`	</dict>`,
			`	<key>RunAtLoad</key>`,
			`	<true/>`,
			`	<key>KeepAlive</key>`,
			`	<true/>`,
			`</dict>`,
			`</plist>`,
			``,
		}, "\n")
		return ServiceUnit{
			Manager: "launchd",
			Name:    label,
			Path:    path.Join(home, "Library", "LaunchAgents", label+".plist"),
			Content: content,
		}, nil
	}
	return ServiceUnit{}, ErrServiceUnsupported
}

// xmlEscape protege los valores que van dentro de un plist: una ruta con & o
// < rompería el XML y launchd rechazaría el agente sin decir por qué.
func xmlEscape(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&apos;",
	)
	return r.Replace(s)
}
