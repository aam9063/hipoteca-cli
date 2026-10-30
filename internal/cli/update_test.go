package cli

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aam9063/hipoteca-cli/internal/errs"
)

// Las pruebas de `hipoteca update` corren contra un servidor local
// (httptest): ninguna toca la red real (REL-01, S8).

// updateServer levanta un servidor local con una release falsa v0.2.0 y
// devuelve su URL. El binario de la release es un tar.gz con "hipoteca" dentro
// y checksums.txt lista su sha256, como la publica goreleaser.
//
// Sirve **todas** las plataformas con el mismo contenido y el mismo hash: el
// test no debe asumir en qué sistema corre, porque el CI lo ejecuta en los
// tres (mismo error que ya apareció en WATCH-02 con las rutas).
func updateServer(t *testing.T, binary []byte) *httptest.Server {
	t.Helper()
	version := "0.2.0"
	archive := buildFakeArchive(t, binary)
	zipArchive := buildFakeZip(t, binary)
	sum := sha256.Sum256(archive)
	digest := hex.EncodeToString(sum[:])
	zipSum := sha256.Sum256(zipArchive)
	zipDigest := hex.EncodeToString(zipSum[:])

	mux := http.NewServeMux()
	mux.HandleFunc("/api/latest", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"tag_name":"v%s","html_url":"https://github.com/aam9063/hipoteca-cli/releases/tag/v%s"}`, version, version)
	})
	mux.HandleFunc("/download/v"+version+"/checksums.txt", func(w http.ResponseWriter, _ *http.Request) {
		for _, os := range []string{"linux", "darwin", "windows"} {
			for _, arch := range []string{"amd64", "arm64"} {
				for _, ext := range []string{"tar.gz", "zip"} {
					hash := digest
					if ext == "zip" {
						hash = zipDigest
					}
					fmt.Fprintf(w, "%s  hipoteca_%s_%s_%s.%s\n", hash, version, os, arch, ext)
				}
			}
		}
	})
	mux.HandleFunc("/download/v"+version+"/", func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(filepath.Base(r.URL.Path), "hipoteca_") {
			http.NotFound(w, r)
			return
		}
		if strings.HasSuffix(r.URL.Path, ".zip") {
			_, _ = w.Write(zipArchive)
			return
		}
		_, _ = w.Write(archive)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// buildFakeArchive monta un tar.gz mínimo con "hipoteca" dentro.
func buildFakeArchive(t *testing.T, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: "hipoteca", Mode: 0o755, Size: int64(len(content))}); err != nil {
		t.Fatalf("cabecera tar: %v", err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatalf("contenido tar: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("cerrar tar: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("cerrar gzip: %v", err)
	}
	return buf.Bytes()
}

// updateCommand monta el árbol con las dependencias de update apuntando al
// servidor local y con el destino en un temporal del test.
func updateCommand(t *testing.T, srvURL string) (*bytes.Buffer, string) {
	t.Helper()
	dest := filepath.Join(t.TempDir(), "hipoteca")
	cmd := NewRootCmdWithDeps(BuildInfo{Version: "0.1.0", Commit: "deadbee", Date: "2026-10-28"}, Deps{
		UpdateAPIURL:   srvURL + "/api/latest",
		UpdateBaseURL:  srvURL + "/download",
		UpdateDestPath: dest,
	})
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stdout)
	cmd.SetArgs([]string{"update"})
	return &stdout, dest
}

// runUpdate ejecuta `hipoteca update` con los argumentos dados, tras escribir
// oldBinary en el destino (la "instalación anterior"). Devuelve el stdout.
func runUpdate(t *testing.T, srvURL string, oldBinary []byte, args ...string) (string, string) {
	t.Helper()
	stdout, dest := updateCommand(t, srvURL)
	if oldBinary != nil {
		if err := os.WriteFile(dest, oldBinary, 0o755); err != nil {
			t.Fatalf("preparar la instalación anterior: %v", err)
		}
	}
	cmdArgs := append([]string{"update"}, args...)
	if err := updateCommandArgs(t, stdout, dest, srvURL, cmdArgs); err != nil {
		t.Fatalf("%v falló: %v", cmdArgs, err)
	}
	return stdout.String(), dest
}

// updateCommandArgs reconstruye el árbol con argumentos concretos (los tests
// que esperan fallo lo capturan en lugar de fatal).
func updateCommandArgs(t *testing.T, stdout *bytes.Buffer, dest, srvURL string, args []string) error {
	t.Helper()
	cmd := NewRootCmdWithDeps(BuildInfo{Version: "0.1.0", Commit: "deadbee", Date: "2026-10-28"}, Deps{
		UpdateAPIURL:   srvURL + "/api/latest",
		UpdateBaseURL:  srvURL + "/download",
		UpdateDestPath: dest,
	})
	cmd.SetOut(stdout)
	cmd.SetErr(stdout)
	cmd.SetArgs(args)
	return cmd.Execute()
}

// REL-01 S6: `hipoteca update --check` no modifica nada y responde en JSON
// con la versión actual, la disponible y si hay actualización.
func TestUpdateCheckAnswersJSONAndTouchesNothing(t *testing.T) {
	srv := updateServer(t, []byte("binario-nuevo"))
	stdout, dest := updateCommand(t, srv.URL)
	if err := updateCommandArgs(t, stdout, dest, srv.URL, []string{"update", "--check"}); err != nil {
		t.Fatalf("update --check debe funcionar contra el servidor local, got %v", err)
	}

	dec := json.NewDecoder(bytes.NewReader(stdout.Bytes()))
	var out struct {
		SchemaVersion   int    `json:"schema_version"`
		CurrentVersion  string `json:"current_version"`
		LatestVersion   string `json:"latest_version"`
		UpdateAvailable bool   `json:"update_available"`
		ReleaseURL      string `json:"release_url"`
		Updated         bool   `json:"updated"`
	}
	if err := dec.Decode(&out); err != nil {
		t.Fatalf("stdout no es JSON: %v\n%s", err, stdout)
	}
	if dec.More() {
		t.Errorf("stdout debe llevar UN solo valor JSON, hay resto: %q", stdout)
	}
	if out.SchemaVersion != 1 {
		t.Errorf("schema_version = %d, want 1", out.SchemaVersion)
	}
	if out.CurrentVersion != "0.1.0" || out.LatestVersion != "0.2.0" {
		t.Errorf("versiones = %q/%q, want 0.1.0/0.2.0", out.CurrentVersion, out.LatestVersion)
	}
	if !out.UpdateAvailable {
		t.Error("update_available = false, want true")
	}
	if out.Updated {
		t.Error("--check no actualiza: updated debe ser false")
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Errorf("--check no debe escribir nada, pero %s existe", dest)
	}
}

// REL-01 S5: `hipoteca update` reemplaza el binario del destino.
func TestUpdateAppliesFromLocalServer(t *testing.T) {
	srv := updateServer(t, []byte("binario-nuevo"))
	stdout, dest := runUpdate(t, srv.URL, []byte("binario-viejo"))

	var out struct {
		SchemaVersion    int    `json:"schema_version"`
		Updated          bool   `json:"updated"`
		InstalledVersion string `json:"installed_version"`
		InstalledPath    string `json:"installed_path"`
	}
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("stdout no es JSON: %v\n%s", err, stdout)
	}
	if !out.Updated || out.InstalledVersion != "0.2.0" || out.InstalledPath != dest {
		t.Errorf("resultado = %+v, want updated a 0.2.0 en %s", out, dest)
	}
	b, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("leer el binario instalado: %v", err)
	}
	if !strings.Contains(string(b), "binario-nuevo") {
		t.Errorf("el binario del destino no es el de la release: %q", b)
	}
}

// REL-01 S5/L3: un checksum que no cuadra deja el binario anterior intacto y
// sale con el tipo del contrato (upstream_unavailable, código 5), con stdout
// vacío.
func TestUpdateChecksumMismatchLeavesPreviousBinaryIntact(t *testing.T) {
	// El servidor declara un checksum que no corresponde con el archivo que
	// sirve: la descarga "está corrupta".
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/latest":
			_, _ = w.Write([]byte(`{"tag_name":"v0.2.0","html_url":"u"}`))
		case strings.HasSuffix(r.URL.Path, "checksums.txt"):
			// Todos los nombres posibles, para que el test no dependa de la
			// plataforma donde corre: el CI lo ejecuta en los tres sistemas.
			for _, os := range []string{"linux", "darwin", "windows"} {
				for _, arch := range []string{"amd64", "arm64"} {
					for _, ext := range []string{"tar.gz", "zip"} {
						_, _ = io.WriteString(w, "0000000000000000000000000000000000000000000000000000000000000000  hipoteca_0.2.0_"+os+"_"+arch+"."+ext+"\n")
					}
				}
			}
		case strings.HasSuffix(r.URL.Path, ".tar.gz"), strings.HasSuffix(r.URL.Path, ".zip"):
			// El checksum no cuadra a propósito, así que el contenido es
			// indiferente: la verificación ocurre antes de descomprimir.
			_, _ = w.Write(buildFakeArchive(t, []byte("binario-corrupto")))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	stdout, dest := updateCommand(t, srv.URL)
	if err := os.WriteFile(dest, []byte("binario-viejo"), 0o755); err != nil {
		t.Fatalf("preparar la instalación anterior: %v", err)
	}
	err := updateCommandArgs(t, stdout, dest, srv.URL, []string{"update"})
	if err == nil {
		t.Fatal("un checksum que no cuadra debe fallar")
	}
	env, ok := errs.From(err)
	if !ok || env.Error.Type != string(errs.KindUpstreamUnavailable) {
		t.Fatalf("kind = %+v, want upstream_unavailable", env)
	}
	if errs.ExitCode(errs.Kind(env.Error.Type)) != 5 {
		t.Errorf("código de salida = %d, want 5", errs.ExitCode(errs.Kind(env.Error.Type)))
	}
	if stdout.Len() != 0 {
		t.Errorf("en fallo stdout debe quedar vacío (el sobre va a stderr), got %q", stdout)
	}
	b, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("el binario anterior debe seguir ahí: %v", err)
	}
	if string(b) != "binario-viejo" {
		t.Errorf("la instalación anterior NO sobrevivió: %q", b)
	}
	entries, err := os.ReadDir(filepath.Dir(dest))
	if err != nil {
		t.Fatalf("listar el destino: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "hipoteca" {
		t.Errorf("quedan restos de la actualización fallida: %v", entries)
	}
}

// `update` rechaza argumentos con un error de uso del contrato.
func TestUpdateRejectsArgumentsAsUsage(t *testing.T) {
	srv := updateServer(t, []byte("x"))
	stdout, dest := updateCommand(t, srv.URL)
	err := updateCommandArgs(t, stdout, dest, srv.URL, []string{"update", "ahora"})
	env, ok := errs.From(err)
	if !ok || env.Error.Type != string(errs.KindUsage) {
		t.Fatalf("kind = %+v, want usage", env)
	}
}

// El comando `update` está en el árbol real: la ayuda lo lista, sin tocar la
// red ni el binario en ejecución.
func TestUpdateIsInTheCommandTree(t *testing.T) {
	cmd := NewRootCmd(BuildInfo{Version: "test", Commit: "x", Date: "y"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"update", "--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("update --help: %v", err)
	}
	if !strings.Contains(out.String(), "--check") {
		t.Errorf("la ayuda de update no menciona --check:\n%s", out.String())
	}
}

// buildFakeZip monta un zip mínimo con "hipoteca" dentro, que es lo que
// goreleaser publica para Windows. Sin esto el test del camino feliz pasaría
// en Linux y macOS y fallaría en Windows, que es exactamente el defecto que
// este archivo ya tuvo.
func buildFakeZip(t *testing.T, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	fw, err := zw.CreateHeader(&zip.FileHeader{Name: "hipoteca.exe", Method: zip.Deflate})
	if err != nil {
		t.Fatalf("cabecera zip: %v", err)
	}
	if _, err := fw.Write(content); err != nil {
		t.Fatalf("contenido zip: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("cerrar zip: %v", err)
	}
	return buf.Bytes()
}
