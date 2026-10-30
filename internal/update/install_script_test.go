package update

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// REL-01 S2: el instalador se puede ejercitar sin red con --dry-run, y la
// detección de plataforma es determinista.
//
// Se inyecta un `uname` falso al frente del PATH en lugar de depender del
// sistema donde corre el test: así la salida es la misma en Linux y macOS, y el
// test no dice "pasa en mi máquina".
func TestInstallScriptDryRun(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("install.sh es un script de shell para Unix; en Windows la vía es Scoop")
	}
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no hay sh en el PATH")
	}

	fake := t.TempDir()
	uname := filepath.Join(fake, "uname")
	script := "#!/bin/sh\ncase \"$1\" in -s) echo Linux ;; -m) echo x86_64 ;; esac\n"
	if err := os.WriteFile(uname, []byte(script), 0o755); err != nil {
		t.Fatalf("preparando el uname falso: %v", err)
	}

	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("resolviendo la raíz: %v", err)
	}
	cmd := exec.Command(sh, filepath.Join(root, "install.sh"), "--dry-run")
	cmd.Env = append(os.Environ(),
		"PATH="+fake+string(os.PathListSeparator)+os.Getenv("PATH"),
		"HIPOTECA_VERSION=v0.1.0",
	)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("install.sh --dry-run: %v", err)
	}

	got := string(out)
	// El nombre del archivo no lleva la "v" del tag: así lo publica goreleaser.
	wantURL := "https://github.com/aam9063/hipoteca-cli/releases/download/v0.1.0/hipoteca_0.1.0_linux_amd64.tar.gz"
	if !strings.Contains(got, wantURL) {
		t.Fatalf("la URL no es la esperada\nquiero: %s\nsalida:\n%s", wantURL, got)
	}
	if !strings.Contains(got, "linux") || !strings.Contains(got, "amd64") {
		t.Fatalf("no declara la plataforma detectada:\n%s", got)
	}
	if !strings.Contains(got, "destino:") {
		t.Fatalf("no dice dónde va a instalar:\n%s", got)
	}
}

// Un sistema no soportado tiene que salir con 2 (usage) y decir cuál es la
// alternativa, no fallar con un error interno.
func TestInstallScriptRejectsUnsupportedSystem(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("install.sh es un script de shell para Unix; en Windows la vía es Scoop")
	}
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no hay sh en el PATH")
	}
	fake := t.TempDir()
	uname := filepath.Join(fake, "uname")
	if err := os.WriteFile(uname, []byte("#!/bin/sh\ncase \"$1\" in -s) echo Plan9 ;; -m) echo x86_64 ;; esac\n"), 0o755); err != nil {
		t.Fatalf("preparando el uname falso: %v", err)
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("resolviendo la raíz: %v", err)
	}
	cmd := exec.Command(sh, filepath.Join(root, "install.sh"), "--dry-run")
	cmd.Env = append(os.Environ(), "PATH="+fake+string(os.PathListSeparator)+os.Getenv("PATH"))
	out, err := cmd.CombinedOutput()

	var exitErr *exec.ExitError
	if err == nil {
		t.Fatalf("un sistema no soportado debe fallar, salida:\n%s", out)
	}
	if !ok(err, &exitErr) {
		t.Fatalf("error inesperado: %v", err)
	}
	if exitErr.ExitCode() != 2 {
		t.Fatalf("código de salida = %d, want 2\nsalida:\n%s", exitErr.ExitCode(), out)
	}
	if !strings.Contains(string(out), "Scoop") {
		t.Fatalf("debe decir cuál es la alternativa:\n%s", out)
	}
}

func ok(err error, target **exec.ExitError) bool {
	e, is := err.(*exec.ExitError)
	if is {
		*target = e
	}
	return is
}
