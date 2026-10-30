// Package update comprueba la última release publicada de hipoteca-cli en
// GitHub y, si el usuario lo pide, reemplaza el binario en ejecución
// (REL-01, S5/S6). La verificación del checksum y la escritura atómica no son
// opcionales: un fallo de red, un checksum que no cuadra o un binario
// corrupto dejan la instalación anterior intacta y funcionando.
//
// Ninguna prueba de este paquete toca la red: los tests levantan un
// httptest.Server local y construyen el Updater con sus URL apuntando allí
// (S8). La producción usa New, que apunta a GitHub y resuelve el binario en
// ejecución.
package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/aam9063/hipoteca-cli/internal/errs"
)

// maxDownload acota cuánto se lee de una descarga: un release roto no puede
// comerse la memoria de la máquina. El binario de este proyecto es de unos
// pocos MB; medio gigabyte es un techo generoso.
const maxDownload = 512 << 20

// Updater sustituye (o comprueba) el binario DestPath por la última release
// publicada. Los campos exportados son los puntos de inyección de los tests:
// producción usa New y no toca nada de esto.
type Updater struct {
	// APIURL es el endpoint de "última release" (JSON con tag_name).
	APIURL string
	// BaseURL es la base de descarga: <BaseURL>/<tag>/<fichero>.
	BaseURL string
	// Client es el cliente HTTP; nil usa uno con timeout propio.
	Client *http.Client
	// DestPath es el binario a reemplazar. Vacío sólo es válido para Check.
	DestPath string
	// CurrentVersion es la versión de este binario ("dev", "0.1.0", ...).
	CurrentVersion string
	// OS y Arch fuerzan la plataforma de la release a descargar; vacíos usan
	// runtime.GOOS y runtime.GOARCH. Sólo los tests los tocan.
	OS   string
	Arch string
}

// Result es la salida única de `hipoteca update` y de `hipoteca update
// --check` (S6): la versión actual, la disponible, si hay actualización y —
// sólo cuando se instaló— qué versión quedó instalada y dónde.
type Result struct {
	CurrentVersion   string `json:"current_version"`
	LatestVersion    string `json:"latest_version"`
	UpdateAvailable  bool   `json:"update_available"`
	ReleaseURL       string `json:"release_url,omitempty"`
	Updated          bool   `json:"updated"`
	InstalledVersion string `json:"installed_version,omitempty"`
	Path             string `json:"installed_path,omitempty"`
}

// New construye el Updater de producción: GitHub y el binario en ejecución.
func New(currentVersion string) (*Updater, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, errs.New(errs.KindInternal,
			"no se pudo resolver el binario en ejecución: "+err.Error(), "")
	}
	return &Updater{
		APIURL:         "https://api.github.com/repos/aam9063/hipoteca-cli/releases/latest",
		BaseURL:        "https://github.com/aam9063/hipoteca-cli/releases/download",
		CurrentVersion: currentVersion,
		DestPath:       exe,
	}, nil
}

// Check consulta la última release publicada y no toca nada más (S6).
func (u *Updater) Check(ctx context.Context) (Result, error) {
	res, _, err := u.check(ctx)
	return res, err
}

// Apply consulta la última release y, si hay una más nueva, la descarga,
// verifica su checksum y reemplaza DestPath. Cualquier fallo antes del
// reemplazo deja el binario anterior intacto: nada se escribe hasta que el
// checksum de la descarga cuadra con el checksums.txt del release.
func (u *Updater) Apply(ctx context.Context) (Result, error) {
	res, tag, err := u.check(ctx)
	if err != nil {
		return Result{}, err
	}
	if !res.UpdateAvailable {
		return res, nil
	}
	if u.DestPath == "" {
		return Result{}, errs.New(errs.KindInternal,
			"no se sabe qué binario reemplazar: DestPath está vacío", "")
	}

	archiveName := u.archiveName(res.LatestVersion)
	checksums, err := u.fetch(ctx, u.downloadURL(tag, "checksums.txt"))
	if err != nil {
		return Result{}, err
	}
	archive, err := u.fetch(ctx, u.downloadURL(tag, archiveName))
	if err != nil {
		return Result{}, err
	}

	// Verificación del checksum ANTES de escribir nada: es la garantía de
	// que una descarga corrupta no toca la instalación anterior (S5, L3).
	expected, ok := parseChecksums(checksums)[archiveName]
	if !ok {
		return Result{}, errs.New(errs.KindAPIChanged,
			fmt.Sprintf("el checksums.txt de la release %s no contiene %s: el release no trae la descarga de esta plataforma", tag, archiveName),
			"reporta el problema en https://github.com/aam9063/hipoteca-cli/issues")
	}
	sum := sha256.Sum256(archive)
	if !strings.EqualFold(hex.EncodeToString(sum[:]), expected) {
		return Result{}, errs.New(errs.KindUpstreamUnavailable,
			fmt.Sprintf("el checksum de %s no coincide con checksums.txt: la descarga está corrupta y no se ha instalado nada", archiveName),
			"reintenta más tarde; si vuelve a fallar, el release está mal publicado")
	}

	bin, err := extractBinary(archive, u.goos())
	if err != nil {
		return Result{}, err
	}
	if err := u.install(bin); err != nil {
		return Result{}, err
	}
	res.Updated = true
	res.InstalledVersion = res.LatestVersion
	res.Path = u.DestPath
	return res, nil
}

// check es la consulta común: devuelve el resultado y el tag crudo de la
// release ("v0.2.0"), que Apply necesita para construir las URL de descarga.
func (u *Updater) check(ctx context.Context) (Result, string, error) {
	body, err := u.fetch(ctx, u.APIURL)
	if err != nil {
		return Result{}, "", err
	}
	var release struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
	}
	if err := json.Unmarshal(body, &release); err != nil {
		return Result{}, "", errs.New(errs.KindAPIChanged,
			"la respuesta de última release de GitHub no es el JSON esperado: la API ha cambiado", "")
	}
	if release.TagName == "" {
		return Result{}, "", errs.New(errs.KindAPIChanged,
			"la respuesta de última release de GitHub no trae tag_name: la API ha cambiado", "")
	}

	latest := strings.TrimPrefix(release.TagName, "v")
	res := Result{
		CurrentVersion:  u.CurrentVersion,
		LatestVersion:   latest,
		ReleaseURL:      release.HTMLURL,
		UpdateAvailable: u.shouldUpdate(latest),
	}
	return res, release.TagName, nil
}

// shouldUpdate decide si la release publicada es una actualización de la
// versión actual: más nueva que la actual, sí; igual o más vieja, no; una
// versión actual que no es semver ("dev", un commit de construcción) ofrece
// siempre la release publicada.
func (u *Updater) shouldUpdate(latest string) bool {
	cmp, ok := compareVersions(u.CurrentVersion, latest)
	if !ok {
		return true
	}
	return cmp < 0
}

// archiveName reconstruye el nombre del archivo de la plataforma con el
// patrón de .goreleaser.yaml: hipoteca_<versión>_<os>_<arch>.<ext>.
func (u *Updater) archiveName(version string) string {
	ext := "tar.gz"
	if u.goos() == "windows" {
		ext = "zip"
	}
	return fmt.Sprintf("hipoteca_%s_%s_%s.%s", version, u.goos(), u.goarch(), ext)
}

func (u *Updater) downloadURL(tag, name string) string {
	return u.BaseURL + "/" + tag + "/" + name
}

func (u *Updater) goos() string {
	if u.OS != "" {
		return u.OS
	}
	return runtime.GOOS
}

func (u *Updater) goarch() string {
	if u.Arch != "" {
		return u.Arch
	}
	return runtime.GOARCH
}

func (u *Updater) client() *http.Client {
	if u.Client != nil {
		return u.Client
	}
	return &http.Client{Timeout: 2 * time.Minute}
}

// fetch hace el GET y traduce los problemas al contrato (§6.4): sin
// conexión, network; 404, not_found; 403, rate_limited; el resto de estados,
// upstream_unavailable. Nunca devuelve un error que no lleve kind.
func (u *Updater) fetch(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, errs.New(errs.KindInternal, "construyendo la petición: "+err.Error(), "")
	}
	req.Header.Set("User-Agent", "hipoteca-cli/"+u.CurrentVersion)
	resp, err := u.client().Do(req)
	if err != nil {
		return nil, errs.New(errs.KindNetwork,
			"no se pudo conectar con GitHub: "+err.Error(),
			"comprueba tu conexión; la actualización necesita descargar la release")
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated:
	case http.StatusNotFound:
		return nil, errs.New(errs.KindNotFound,
			fmt.Sprintf("GitHub responde 404 para %s: la release o el fichero no existen", url),
			"comprueba que el binario y la plataforma tienen release publicada")
	case http.StatusForbidden:
		return nil, errs.New(errs.KindRateLimited,
			fmt.Sprintf("GitHub responde 403 para %s: petición limitada por tarifa o sin permiso", url),
			"reintenta más tarde")
	default:
		return nil, errs.New(errs.KindUpstreamUnavailable,
			fmt.Sprintf("GitHub responde %d para %s: el upstream no está sirviendo la release", resp.StatusCode, url),
			"reintenta más tarde")
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxDownload))
	if err != nil {
		return nil, errs.New(errs.KindNetwork,
			"la descarga se cortó: "+err.Error(),
			"reintenta; la instalación anterior no ha sido tocada")
	}
	return body, nil
}

// install escribe el binario nuevo en un temporal del MISMO directorio que el
// destino (para que el rename sea atómico) y lo renombra encima. Nada llega
// al destino hasta aquí: todos los fallos anteriores borran el temporal y
// dejan la instalación anterior tal estaba.
func (u *Updater) install(bin []byte) error {
	dir := filepath.Dir(u.DestPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return errs.New(errs.KindInternal,
			"no se pudo preparar el directorio "+dir+": "+err.Error(), "")
	}
	tmp, err := os.CreateTemp(dir, ".hipoteca.update-*.tmp")
	if err != nil {
		return errs.New(errs.KindInternal,
			"no se pudo crear el temporal en "+dir+": "+err.Error(), "")
	}
	tmpName := tmp.Name()
	// Si algo falla, el temporal se borra y el destino no se ha tocado.
	defer func() {
		if tmpName != "" {
			_ = os.Remove(tmpName)
		}
	}()
	if err := tmp.Chmod(0o755); err != nil {
		_ = tmp.Close()
		return errs.New(errs.KindInternal, "no se pudo fijar el permiso del binario nuevo: "+err.Error(), "")
	}
	if _, err := tmp.Write(bin); err != nil {
		_ = tmp.Close()
		return errs.New(errs.KindInternal, "no se pudo escribir el binario nuevo: "+err.Error(), "")
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return errs.New(errs.KindInternal, "no se pudo sincronizar el binario nuevo: "+err.Error(), "")
	}
	if err := tmp.Close(); err != nil {
		return errs.New(errs.KindInternal, "no se pudo cerrar el binario nuevo: "+err.Error(), "")
	}
	if err := swapBinary(tmpName, u.DestPath); err != nil {
		return errs.New(errs.KindInternal,
			"no se pudo reemplazar "+u.DestPath+": "+err.Error(),
			"comprueba los permisos del directorio del binario")
	}
	tmpName = "" // movido a su sitio: el defer ya no debe borrarlo
	return nil
}

// swapBinary mueve newPath encima de dest. En Unix es un rename atómico: el
// proceso en ejecución sigue usando el inode antiguo y el próximo arranque
// usa el nuevo. En Windows el sistema no permite renombrar sobre un .exe en
// ejecución, así que el viejo se aparta como <dest>.old y se borra lo antes
// posible (el borrado puede fallar mientras el proceso vive; no es un error
// de la instalación). Si el segundo rename falla, el viejo vuelve a su sitio.
func swapBinary(newPath, dest string) error {
	if runtime.GOOS != "windows" {
		return os.Rename(newPath, dest)
	}
	old := dest + ".old"
	_ = os.Remove(old)
	if err := os.Rename(dest, old); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Rename(newPath, dest); err != nil {
		if rerr := os.Rename(old, dest); rerr != nil {
			return fmt.Errorf("%v (y la reversión falló: %v)", err, rerr)
		}
		return err
	}
	_ = os.Remove(old)
	return nil
}

// parseChecksums lee un checksums.txt con formato sha256sum: "<hex>  <nombre>"
// (dos espacios) o "<hex> *<nombre>" (modo binario). Devuelve nombre → hex.
func parseChecksums(data []byte) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) != 2 {
			continue
		}
		name := strings.TrimPrefix(parts[1], "*")
		out[name] = parts[0]
	}
	return out
}

// extractBinary saca el binario del archivo del release: tar.gz en Unix y
// zip en Windows, con la entrada que goreleaser publica ("hipoteca" o
// "hipoteca.exe" en la raíz). Un release que no la trae es un cambio del
// upstream (api_changed), no un panic.
func extractBinary(archive []byte, goos string) ([]byte, error) {
	want := "hipoteca"
	if goos == "windows" {
		want = "hipoteca.exe"
	}
	if goos == "windows" {
		zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
		if err != nil {
			return nil, errs.New(errs.KindUpstreamUnavailable,
				"el archivo de la release no es un zip válido: está corrupto y no se ha instalado nada",
				"reintenta más tarde; si vuelve a fallar, el release está mal publicado")
		}
		for _, f := range zr.File {
			if filepath.Base(f.Name) != want {
				continue
			}
			rc, err := f.Open()
			if err != nil {
				return nil, errs.New(errs.KindUpstreamUnavailable,
					"no se pudo leer "+f.Name+" dentro del zip: "+err.Error(), "")
			}
			defer rc.Close()
			bin, err := io.ReadAll(io.LimitReader(rc, maxDownload))
			if err != nil {
				return nil, errs.New(errs.KindUpstreamUnavailable,
					"no se pudo leer "+f.Name+" dentro del zip: "+err.Error(), "")
			}
			return bin, nil
		}
		return nil, errs.New(errs.KindAPIChanged,
			"el zip de la release no contiene "+want+": el formato del release ha cambiado",
			"reporta el problema en https://github.com/aam9063/hipoteca-cli/issues")
	}

	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, errs.New(errs.KindUpstreamUnavailable,
			"el archivo de la release no es un tar.gz válido: está corrupto y no se ha instalado nada",
			"reintenta más tarde; si vuelve a fallar, el release está mal publicado")
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, errs.New(errs.KindUpstreamUnavailable,
				"el tar.gz de la release está corrupto y no se ha instalado nada: "+err.Error(),
				"reintenta más tarde; si vuelve a fallar, el release está mal publicado")
		}
		if filepath.Base(header.Name) != want {
			continue
		}
		bin, err := io.ReadAll(io.LimitReader(tr, maxDownload))
		if err != nil {
			return nil, errs.New(errs.KindUpstreamUnavailable,
				"no se pudo leer "+header.Name+" dentro del tar.gz: "+err.Error(), "")
		}
		return bin, nil
	}
	return nil, errs.New(errs.KindAPIChanged,
		"el tar.gz de la release no contiene "+want+": el formato del release ha cambiado",
		"reporta el problema en https://github.com/aam9063/hipoteca-cli/issues")
}

// compareVersions compara dos versiones semver ("0.1.0", "v0.2.0", "0.3").
// Devuelve -1/0/1 y true; si alguna no es semver, (0, false) y la llamada
// decide qué hacer (shouldUpdate trata ese caso como "ofrecer actualización").
func compareVersions(a, b string) (int, bool) {
	pa, oka := versionParts(a)
	pb, okb := versionParts(b)
	if !oka || !okb {
		return 0, false
	}
	n := len(pa)
	if len(pb) > n {
		n = len(pb)
	}
	for i := 0; i < n; i++ {
		x, y := 0, 0
		if i < len(pa) {
			x = pa[i]
		}
		if i < len(pb) {
			y = pb[i]
		}
		if x != y {
			if x > y {
				return 1, true
			}
			return -1, true
		}
	}
	return 0, true
}

// versionParts parte una versión en sus componentes numéricos. Un prefijo v
// se ignora; cualquier componente no numérico invalida la versión.
func versionParts(v string) ([]int, bool) {
	v = strings.TrimPrefix(strings.TrimPrefix(v, "v"), "V")
	if v == "" {
		return nil, false
	}
	parts := strings.Split(v, ".")
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return nil, false
		}
		out = append(out, n)
	}
	return out, true
}
