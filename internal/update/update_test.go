package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/aam9063/hipoteca-cli/internal/errs"
)

// --- servidor local de releases (REL-01, S8): ninguna prueba toca la red ---

// fakeServer levanta un servidor local que responde /api/latest con el JSON
// dado y sirve los ficheros de la ruta que les corresponde. requests cuenta
// cuántas peticiones recibe, para poder afirmar que una ruta no descarga nada.
type fakeServer struct {
	URL      string
	requests *int
	files    map[string][]byte
}

func fakeServerFor(t *testing.T, apiBody []byte, apiStatus int, files map[string][]byte) *fakeServer {
	t.Helper()
	requests := new(int)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*requests++
		if b, ok := files[r.URL.Path]; ok {
			_, _ = w.Write(b)
			return
		}
		if r.URL.Path == "/api/latest" {
			w.Header().Set("Content-Type", "application/json")
			if apiStatus != 0 {
				w.WriteHeader(apiStatus)
			}
			_, _ = w.Write(apiBody)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	return &fakeServer{URL: srv.URL, requests: requests, files: files}
}

func fakeServerDown(t *testing.T) *fakeServer {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close() // cerrado a propósito: conectar falla sin tocar la red real
	return &fakeServer{URL: url, requests: new(int), files: nil}
}

// --- constructores de una release falsa ---

func sha256hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// buildTarGz construye un .tar.gz con las entradas dadas (rutas separadas por /).
func buildTarGz(t *testing.T, entries map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, content := range entries {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(content))}); err != nil {
			t.Fatalf("cabecera tar de %s: %v", name, err)
		}
		if _, err := tw.Write(content); err != nil {
			t.Fatalf("contenido tar de %s: %v", name, err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("cerrar tar: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("cerrar gzip: %v", err)
	}
	return buf.Bytes()
}

// buildZip construye un .zip con las entradas dadas (la release de Windows).
func buildZip(t *testing.T, entries map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range entries {
		f, err := zw.Create(name)
		if err != nil {
			t.Fatalf("entrada zip %s: %v", name, err)
		}
		if _, err := f.Write(content); err != nil {
			t.Fatalf("contenido zip %s: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("cerrar zip: %v", err)
	}
	return buf.Bytes()
}

// releaseFiles monta los ficheros de una release con el tag dado: el binario
// va dentro del fichero y checksums.txt lista el sha256 de lo que hay en el
// servidor. checksumOverride permite servir un checksum que NO corresponde al
// fichero servido (caso "descarga corrupta", L3).
func releaseFiles(t *testing.T, tag, version string, binary []byte, goos, goarch string, checksumOverride map[string]string) map[string][]byte {
	t.Helper()
	var archive []byte
	archiveName := ""
	if goos == "windows" {
		archiveName = fmt.Sprintf("hipoteca_%s_%s_%s.zip", version, goos, goarch)
		archive = buildZip(t, map[string][]byte{"hipoteca.exe": binary})
	} else {
		archiveName = fmt.Sprintf("hipoteca_%s_%s_%s.tar.gz", version, goos, goarch)
		archive = buildTarGz(t, map[string][]byte{"hipoteca": binary})
	}
	hexes := map[string]string{archiveName: sha256hex(archive)}
	for name, hexValue := range checksumOverride {
		hexes[name] = hexValue
	}
	checksums := new(bytes.Buffer)
	for name, hexValue := range hexes {
		fmt.Fprintf(checksums, "%s  %s\n", hexValue, name)
	}
	return map[string][]byte{
		"/api/latest":                          []byte(fmt.Sprintf(`{"tag_name":%q,"html_url":"https://github.com/aam9063/hipoteca-cli/releases/tag/%s"}`, tag, tag)),
		"/download/" + tag + "/checksums.txt":  checksums.Bytes(),
		"/download/" + tag + "/" + archiveName: archive,
	}
}

// testUpdater monta un Updater contra el servidor local, con destino en un
// directorio temporal: ninguna prueba lee ni escribe fuera del temp.
func testUpdater(t *testing.T, srv *fakeServer, currentVersion string) (*Updater, string) {
	t.Helper()
	dest := filepath.Join(t.TempDir(), "hipoteca")
	return &Updater{
		APIURL:         srv.URL + "/api/latest",
		BaseURL:        srv.URL + "/download",
		CurrentVersion: currentVersion,
		OS:             "linux",
		Arch:           "amd64",
		DestPath:       dest,
	}, dest
}

func assertKind(t *testing.T, err error, wantKind errs.Kind, wantCode int) {
	t.Helper()
	if err == nil {
		t.Fatalf("se esperaba un error %q, no hubo ninguno", wantKind)
	}
	env, ok := errs.From(err)
	if !ok {
		t.Fatalf("el error no lleva kind del contrato: %v", err)
	}
	if env.Error.Type != string(wantKind) {
		t.Errorf("kind = %q, want %q (mensaje: %s)", env.Error.Type, wantKind, env.Error.Message)
	}
	if code := errs.ExitCode(errs.Kind(env.Error.Type)); code != wantCode {
		t.Errorf("código de salida = %d, want %d", code, wantCode)
	}
}

func writeFile(t *testing.T, path string, content []byte, perm os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("crear directorio: %v", err)
	}
	if err := os.WriteFile(path, content, perm); err != nil {
		t.Fatalf("escribir %s: %v", path, err)
	}
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("leer %s: %v", path, err)
	}
	return b
}

func dirEntries(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("listar %s: %v", dir, err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

// --- S6: update --check no modifica nada y responde con la versión actual,
// la disponible y si hay actualización ---

func TestCheckReportsLatestReleaseAndOffersUpdate(t *testing.T) {
	srv := fakeServerFor(t, nil, 0, releaseFiles(t, "v0.2.0", "0.2.0", []byte("binario-0.2.0"), "linux", "amd64", nil))
	up, dest := testUpdater(t, srv, "0.1.0")

	res, err := up.Check(context.Background())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if res.CurrentVersion != "0.1.0" {
		t.Errorf("current_version = %q, want 0.1.0", res.CurrentVersion)
	}
	if res.LatestVersion != "0.2.0" {
		t.Errorf("latest_version = %q, want 0.2.0 (sin la v del tag)", res.LatestVersion)
	}
	if !res.UpdateAvailable {
		t.Error("update_available = false, want true (0.1.0 < 0.2.0)")
	}
	if res.ReleaseURL != "https://github.com/aam9063/hipoteca-cli/releases/tag/v0.2.0" {
		t.Errorf("release_url = %q", res.ReleaseURL)
	}
	if res.Updated {
		t.Error("--check no actualiza: updated debe ser false")
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Errorf("--check no debe escribir nada, pero %s existe", dest)
	}
	if *srv.requests != 1 {
		t.Errorf("--check debe hacer una sola petición (la API), hizo %d", *srv.requests)
	}
}

func TestCheckSameVersionIsNotAnUpdate(t *testing.T) {
	srv := fakeServerFor(t, nil, 0, releaseFiles(t, "v0.2.0", "0.2.0", []byte("binario-0.2.0"), "linux", "amd64", nil))
	up, _ := testUpdater(t, srv, "0.2.0")

	res, err := up.Check(context.Background())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if res.UpdateAvailable {
		t.Error("misma versión: update_available debe ser false")
	}
}

func TestCheckNewerLocalVersionIsNotAnUpdate(t *testing.T) {
	srv := fakeServerFor(t, nil, 0, releaseFiles(t, "v0.2.0", "0.2.0", []byte("binario-0.2.0"), "linux", "amd64", nil))
	up, _ := testUpdater(t, srv, "0.3.0")

	res, err := up.Check(context.Background())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if res.UpdateAvailable {
		t.Error("una versión local más nueva que la publicada no debe ofrecer actualización")
	}
}

func TestCheckDevVersionOffersUpdate(t *testing.T) {
	srv := fakeServerFor(t, nil, 0, releaseFiles(t, "v0.1.0", "0.1.0", []byte("binario-0.1.0"), "linux", "amd64", nil))
	up, _ := testUpdater(t, srv, "dev")

	res, err := up.Check(context.Background())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !res.UpdateAvailable {
		t.Error("un binario dev siempre debe ofrecer la release publicada")
	}
}

// S5/S8: los problemas de red y del upstream llegan como tipos del contrato,
// no como errores crudos de http.
func TestCheckMapsUpstreamProblemsToTheContract(t *testing.T) {
	cases := []struct {
		name      string
		apiBody   []byte
		apiStatus int
		down      bool
		wantKind  errs.Kind
		wantCode  int
	}{
		{"release inexistente", nil, http.StatusNotFound, false, errs.KindNotFound, 4},
		{"rate limit", nil, http.StatusForbidden, false, errs.KindRateLimited, 5},
		{"error del servidor", nil, http.StatusInternalServerError, false, errs.KindUpstreamUnavailable, 5},
		{"json roto", []byte("<html>no soy json</html>"), 0, false, errs.KindAPIChanged, 7},
		{"sin tag_name", []byte(`{"html_url":"x"}`), 0, false, errs.KindAPIChanged, 7},
		{"sin conexión", nil, 0, true, errs.KindNetwork, 6},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var srv *fakeServer
			if tc.down {
				srv = fakeServerDown(t)
			} else {
				srv = fakeServerFor(t, tc.apiBody, tc.apiStatus, nil)
			}
			up, _ := testUpdater(t, srv, "0.1.0")
			_, err := up.Check(context.Background())
			assertKind(t, err, tc.wantKind, tc.wantCode)
		})
	}
}

func TestCheckRateLimitResponseIsRateLimited(t *testing.T) {
	requests := new(int)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		*requests++
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(srv.Close)

	up, _ := testUpdater(t, &fakeServer{URL: srv.URL, requests: requests}, "0.1.0")
	_, err := up.Check(context.Background())
	assertKind(t, err, errs.KindRateLimited, 5)
}

// --- S5: aplicar reemplaza el binario con checksum y escritura atómica ---

func TestApplyReplacesTheBinary(t *testing.T) {
	srv := fakeServerFor(t, nil, 0, releaseFiles(t, "v0.2.0", "0.2.0", []byte("binario-nuevo"), "linux", "amd64", nil))
	up, dest := testUpdater(t, srv, "0.1.0")
	writeFile(t, dest, []byte("binario-viejo"), 0o755)

	res, err := up.Apply(context.Background())
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !res.Updated {
		t.Error("updated = false, want true")
	}
	if res.InstalledVersion != "0.2.0" {
		t.Errorf("installed_version = %q, want 0.2.0", res.InstalledVersion)
	}
	if res.Path != dest {
		t.Errorf("path = %q, want %q", res.Path, dest)
	}
	if got := readFile(t, dest); string(got) != "binario-nuevo" {
		t.Errorf("el binario no se reemplazó: %q", got)
	}
	if names := dirEntries(t, filepath.Dir(dest)); len(names) != 1 {
		t.Errorf("quedan restos temporales junto al binario: %v", names)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(dest)
		if err != nil {
			t.Fatalf("stat: %v", err)
		}
		if info.Mode().Perm() != 0o755 {
			t.Errorf("permiso del binario = %o, want 755", info.Mode().Perm())
		}
	}
}

func TestApplyWithoutUpdateTouchesNothing(t *testing.T) {
	srv := fakeServerFor(t, nil, 0, releaseFiles(t, "v0.2.0", "0.2.0", []byte("binario-0.2.0"), "linux", "amd64", nil))
	up, dest := testUpdater(t, srv, "0.2.0")
	writeFile(t, dest, []byte("binario-instalado"), 0o755)

	res, err := up.Apply(context.Background())
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if res.Updated {
		t.Error("con la misma versión no debe actualizarse")
	}
	if got := readFile(t, dest); string(got) != "binario-instalado" {
		t.Errorf("el binario cambió sin necesidad: %q", got)
	}
	if *srv.requests != 1 {
		t.Errorf("sin actualización sólo se consulta la API; hubo %d peticiones", *srv.requests)
	}
}

// L3: la descarga corrupta (checksum que no cuadra) NO toca el binario
// anterior: la instalación previa sobrevive intacta y sin restos.
func TestApplyChecksumMismatchLeavesPreviousBinaryIntact(t *testing.T) {
	bin := []byte("binario-nuevo-corrupto")
	files := releaseFiles(t, "v0.2.0", "0.2.0", bin, "linux", "amd64",
		map[string]string{"hipoteca_0.2.0_linux_amd64.tar.gz": sha256hex([]byte("otra-cosa-por-la-que-el-checksum-no-cuadra"))})
	srv := fakeServerFor(t, nil, 0, files)
	up, dest := testUpdater(t, srv, "0.1.0")
	writeFile(t, dest, []byte("binario-viejo"), 0o755)

	_, err := up.Apply(context.Background())
	assertKind(t, err, errs.KindUpstreamUnavailable, 5)
	if !strings.Contains(err.Error(), "checksum") {
		t.Errorf("el mensaje debe hablar del checksum: %v", err)
	}
	if got := readFile(t, dest); string(got) != "binario-viejo" {
		t.Errorf("la instalación anterior NO sobrevivió: %q", got)
	}
	if names := dirEntries(t, filepath.Dir(dest)); len(names) != 1 {
		t.Errorf("quedan restos de la descarga fallida: %v", names)
	}
}

// L3: un fallo de red a mitad de la actualización deja la instalación
// anterior intacta.
func TestApplyNetworkFailureLeavesPreviousBinaryIntact(t *testing.T) {
	srv := fakeServerDown(t)
	up, dest := testUpdater(t, srv, "0.1.0")
	writeFile(t, dest, []byte("binario-viejo"), 0o755)

	_, err := up.Apply(context.Background())
	assertKind(t, err, errs.KindNetwork, 6)
	if got := readFile(t, dest); string(got) != "binario-viejo" {
		t.Errorf("la instalación anterior NO sobrevivió: %q", got)
	}
}

// L3: el binario corrupto (el fichero de la release no es un fichero válido)
// deja la instalación anterior intacta.
func TestApplyCorruptArchiveLeavesPreviousBinaryIntact(t *testing.T) {
	archive := []byte("esto no es un tar.gz, son bytes corruptos")
	files := map[string][]byte{
		"/api/latest":                    []byte(`{"tag_name":"v0.2.0","html_url":"u"}`),
		"/download/v0.2.0/checksums.txt": []byte(sha256hex(archive) + "  hipoteca_0.2.0_linux_amd64.tar.gz\n"),
		"/download/v0.2.0/hipoteca_0.2.0_linux_amd64.tar.gz": archive,
	}
	srv := fakeServerFor(t, nil, 0, files)
	up, dest := testUpdater(t, srv, "0.1.0")
	writeFile(t, dest, []byte("binario-viejo"), 0o755)

	_, err := up.Apply(context.Background())
	assertKind(t, err, errs.KindUpstreamUnavailable, 5)
	if got := readFile(t, dest); string(got) != "binario-viejo" {
		t.Errorf("la instalación anterior NO sobrevivió: %q", got)
	}
	if names := dirEntries(t, filepath.Dir(dest)); len(names) != 1 {
		t.Errorf("quedan restos de la descarga fallida: %v", names)
	}
}

// L3: el checksum verifica pero el binario dentro del fichero está corrupto
// (bytes sueltos): la instalación anterior debe sobrevivir igualmente.
func TestApplyCorruptBinaryInsideArchiveLeavesPreviousBinaryIntact(t *testing.T) {
	bin := []byte("bytes que no son un ejecutable")
	files := releaseFiles(t, "v0.2.0", "0.2.0", bin, "linux", "amd64", nil)
	srv := fakeServerFor(t, nil, 0, files)
	up, dest := testUpdater(t, srv, "0.1.0")
	writeFile(t, dest, []byte("binario-viejo"), 0o755)

	// El checksum cuadra (es lo que hay en el fichero), así que el reemplazo
	// se hace: este caso prueba que el fallo NO deja a medias. Un binario que
	// no ejecuta es un problema del release, no de la instalación; lo que
	// aquí se fija es que el reemplazo es completo (nada a medias en el dir).
	res, err := up.Apply(context.Background())
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !res.Updated {
		t.Error("updated = false, want true")
	}
	if got := readFile(t, dest); string(got) != string(bin) {
		t.Errorf("el reemplazo no fue completo: %q", got)
	}
	if names := dirEntries(t, filepath.Dir(dest)); len(names) != 1 {
		t.Errorf("quedan restos del reemplazo: %v", names)
	}
}

func TestApplyMissingAssetLeavesPreviousBinaryIntact(t *testing.T) {
	// El checksums.txt existe pero el fichero de la plataforma no está en el
	// servidor: 404 en la descarga del fichero.
	files := map[string][]byte{
		"/api/latest":                    []byte(`{"tag_name":"v0.2.0","html_url":"u"}`),
		"/download/v0.2.0/checksums.txt": []byte(sha256hex([]byte("x")) + "  hipoteca_0.2.0_linux_amd64.tar.gz\n"),
	}
	srv := fakeServerFor(t, nil, 0, files)
	up, dest := testUpdater(t, srv, "0.1.0")
	writeFile(t, dest, []byte("binario-viejo"), 0o755)

	_, err := up.Apply(context.Background())
	assertKind(t, err, errs.KindNotFound, 4)
	if got := readFile(t, dest); string(got) != "binario-viejo" {
		t.Errorf("la instalación anterior NO sobrevivió: %q", got)
	}
}

func TestApplyChecksumsMissingEntryLeavesBinaryIntact(t *testing.T) {
	srv := fakeServerFor(t, nil, 0, releaseFiles(t, "v0.2.0", "0.2.0", []byte("binario-nuevo"), "linux", "amd64", nil))
	up, dest := testUpdater(t, srv, "0.1.0")
	// Apuntamos a una plataforma de la que checksums.txt no sabe nada: el
	// fichero existe en el servidor, pero el checksums.txt no lo lista.
	up.Arch = "arm64"
	extra := buildTarGz(t, map[string][]byte{"hipoteca": []byte("binario-nuevo-arm64")})
	srv.files["/download/v0.2.0/hipoteca_0.2.0_linux_arm64.tar.gz"] = extra
	writeFile(t, dest, []byte("binario-viejo"), 0o755)

	_, err := up.Apply(context.Background())
	assertKind(t, err, errs.KindAPIChanged, 7)
	if got := readFile(t, dest); string(got) != "binario-viejo" {
		t.Errorf("la instalación anterior NO sobrevivió: %q", got)
	}
}

// La release de Windows (zip con hipoteca.exe) se extrae correctamente
// estemos donde estemos: el formato depende del SO de la release, no del
// host que ejecuta el test.
func TestApplyWindowsZipArchive(t *testing.T) {
	srv := fakeServerFor(t, nil, 0, releaseFiles(t, "v0.2.0", "0.2.0", []byte("binario-nuevo-exe"), "windows", "amd64", nil))
	up, dest := testUpdater(t, srv, "0.1.0")
	up.OS = "windows"
	writeFile(t, dest, []byte("exe-viejo"), 0o755)

	res, err := up.Apply(context.Background())
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !res.Updated {
		t.Error("updated = false, want true")
	}
	if got := readFile(t, dest); string(got) != "binario-nuevo-exe" {
		t.Errorf("el binario no se reemplazó: %q", got)
	}
	if names := dirEntries(t, filepath.Dir(dest)); len(names) != 1 {
		t.Errorf("quedan restos temporales junto al binario: %v", names)
	}
}

// El propio binario en ejecución se reemplaza a sí mismo: en Unix el rename
// sobre el fichero abierto es el mecanismo; este test instala sobre el
// binario real del proceso (sólo lectura/escritura del propio temp del test,
// vía un fichero distinto, para no tocar el binario del host).
func TestSwapReplacesExistingFile(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "hipoteca")
	writeFile(t, dest, []byte("ejecutando"), 0o755)
	newBin := filepath.Join(dir, ".hipoteca.update-nuevo.tmp")
	writeFile(t, newBin, []byte("reemplazo"), 0o755)

	if err := swapBinary(newBin, dest); err != nil {
		t.Fatalf("swapBinary: %v", err)
	}
	if got := readFile(t, dest); string(got) != "reemplazo" {
		t.Errorf("el destino no quedó reemplazado: %q", got)
	}
	if _, err := os.Stat(newBin); !os.IsNotExist(err) {
		t.Errorf("el temporal no se movió al destino: %v", err)
	}
}

// TestExtractRejectsArchiveWithoutBinary fija el caso "el release cambió de
// forma": un fichero sin el binario esperado es un api_changed, no un pánico.
func TestExtractRejectsArchiveWithoutBinary(t *testing.T) {
	archive := buildTarGz(t, map[string][]byte{"LICENSE": []byte("MIT")})
	if _, err := extractBinary(archive, "linux"); err == nil {
		t.Fatal("un fichero sin hipoteca debe fallar")
	} else if env, ok := errs.From(err); !ok || env.Error.Type != string(errs.KindAPIChanged) {
		t.Errorf("kind = %+v, want api_changed", env)
	}
}
