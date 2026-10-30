package app

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"runtime"
	"sort"
	"time"

	"github.com/aam9063/hipoteca-cli/internal/bde"
	"github.com/aam9063/hipoteca-cli/internal/config"
	"github.com/aam9063/hipoteca-cli/internal/model"
)

// DoctorResult es la salida de `doctor` (AGENT-01, S5): el estado completo de
// la instalación en un único valor JSON — versión y commit, rutas de config y
// datos con su permiso, catálogo de índices, préstamos guardados con los
// campos que les faltan, y la comprobación de red (una sola, desactivable con
// --offline).
type DoctorResult struct {
	SchemaVersion int           `json:"schema_version"`
	Version       string        `json:"version"`
	Commit        string        `json:"commit"`
	ConfigPath    string        `json:"config_path"`
	ConfigExists  bool          `json:"config_exists"`
	ConfigMode    string        `json:"config_mode,omitempty"`
	DataPath      string        `json:"data_path"`
	DataExists    bool          `json:"data_exists"`
	DataMode      string        `json:"data_mode,omitempty"`
	CacheDir      string        `json:"cache_dir"`
	CacheExists   bool          `json:"cache_exists"`
	Indices       []DoctorIndex `json:"indices"`
	Loans         []DoctorLoan  `json:"loans"`
	Network       DoctorNetwork `json:"network"`
	Warnings      []string      `json:"warnings"`
}

// DoctorIndex es un índice del catálogo con su código de serie efectivo
// (con los overrides de configuración ya aplicados).
type DoctorIndex struct {
	ID   string `json:"id"`
	Code string `json:"code"`
	Name string `json:"name"`
}

// DoctorLoan es un préstamo guardado con su marca de completo y, si falta
// algo, el nombre exacto de cada campo de la escritura que no declara —
// lo mismo que `loan show` avisa— para que un agente sepa qué pedir sin
// abrir la config.
type DoctorLoan struct {
	ID            string   `json:"id"`
	Name          string   `json:"name,omitempty"`
	Type          string   `json:"type,omitempty"`
	Complete      bool     `json:"complete"`
	MissingFields []string `json:"missing_fields,omitempty"`
}

// DoctorNetwork es la única comprobación de red del diagnóstico. Con
// --offline no se toca nada y checked es false. La comprobación es de
// conectividad TCP contra el host que publica el catálogo (EndpointBase):
// una conexión que se abre y se cierra, sin semántica HTTP ni caché, de
// modo que la respuesta dice si la red responde AHORA y no si la caché
// tiene el dato de antes.
type DoctorNetwork struct {
	Checked bool   `json:"checked"`
	OK      bool   `json:"ok"`
	Host    string `json:"host,omitempty"`
	Error   string `json:"error,omitempty"`
}

// DoctorParams son las entradas de Doctor. Las rutas las resuelve la capa
// CLI (Deps), de modo que los tests inyectan temporales y ninguna prueba lee
// ni escribe datos del usuario. Dial es la sonda de red inyectable (los
// tests no tocan la red); nil usa la de producción.
type DoctorParams struct {
	ConfigPath string
	DataPath   string
	CacheDir   string
	Version    string
	Commit     string
	Offline    bool
	Dial       func(ctx context.Context, host string) error
}

// networkTimeout es el techo de la única comprobación de red: un doctor no
// puede quedarse colgado por un host que no responde.
const networkTimeout = 5 * time.Second

// Doctor monta el diagnóstico completo. Es un comando de diagnóstico: salvo
// un error interno inesperado, siempre responde con el estado y sus avisos
// (config corrupta, red caída, permisos incorrectos) y sale 0; el usuario
// decide qué arreglar.
func Doctor(ctx context.Context, p DoctorParams) (DoctorResult, error) {
	res := DoctorResult{
		SchemaVersion: model.SchemaVersion,
		Version:       p.Version,
		Commit:        p.Commit,
		ConfigPath:    p.ConfigPath,
		DataPath:      p.DataPath,
		CacheDir:      p.CacheDir,
		Warnings:      []string{},
	}

	// La config puede estar corrupta: es justamente lo que un doctor debe
	// reportar, no un motivo para colgarse antes de decir nada. Sin config
	// legible el catálogo se carga sin overrides y los préstamos quedan
	// vacíos, y el aviso nombra la causa y el camino de arreglo.
	var overrides map[string]string
	if cfg, err := config.Load(p.ConfigPath); err != nil {
		res.Warnings = append(res.Warnings,
			fmt.Sprintf("la configuración %s no se pudo leer: %v", p.ConfigPath, err))
	} else {
		overrides = cfg.Indices
		// Préstamos guardados, ordenados por id, con lo que les falta (S5).
		ids := make([]string, 0, len(cfg.Loans))
		for id := range cfg.Loans {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		res.Loans = make([]DoctorLoan, 0, len(ids))
		for _, id := range ids {
			l := cfg.Loans[id]
			missing := l.MissingFields()
			res.Loans = append(res.Loans, DoctorLoan{
				ID:            id,
				Name:          l.Name,
				Type:          l.Type,
				Complete:      len(missing) == 0,
				MissingFields: missing,
			})
		}
	}

	catalog, err := bde.LoadCatalog(overrides)
	if err != nil {
		// El catálogo embebido es válido por construcción; un fallo aquí
		// viene de los overrides y ya está avisado arriba. Último recurso:
		// el catálogo sin overrides.
		catalog, err = bde.LoadCatalog(nil)
		if err != nil {
			return DoctorResult{}, err
		}
	}
	res.Indices = make([]DoctorIndex, 0, len(catalog.IDs()))
	for _, id := range catalog.IDs() {
		idx, err := catalog.Lookup(id)
		if err != nil {
			return DoctorResult{}, err
		}
		res.Indices = append(res.Indices, DoctorIndex{ID: idx.ID, Code: idx.Code, Name: idx.Name})
	}

	// Rutas y permisos (S5): el modo se reporta tal cual; en los sistemas
	// con bits de Unix un modo distinto de 0600 es un aviso (en Windows los
	// bits de permiso no existen y no se juzgan).
	if mode, ok := statMode(p.ConfigPath); ok {
		res.ConfigExists = true
		res.ConfigMode = mode
		res.warnPermiso(res.ConfigPath, mode)
	}
	if mode, ok := statMode(p.DataPath); ok {
		res.DataExists = true
		res.DataMode = mode
		res.warnPermiso(p.DataPath, mode)
	}
	if info, err := os.Stat(p.CacheDir); err == nil && info.IsDir() {
		res.CacheExists = true
	}

	// La única comprobación de red (S5): off con --offline, una conexión en
	// el resto de casos.
	if p.Offline {
		res.Network = DoctorNetwork{Checked: false}
	} else {
		host := networkHost(catalog)
		res.Network = DoctorNetwork{Checked: true, Host: host}
		dial := p.Dial
		if dial == nil {
			dial = dialTCP
		}
		if err := dial(ctx, host); err != nil {
			res.Network.OK = false
			res.Network.Error = err.Error()
			res.Warnings = append(res.Warnings,
				fmt.Sprintf("el host del Banco de España %s no responde: %v", host, err))
		} else {
			res.Network.OK = true
		}
	}
	return res, nil
}

// warnPermiso añade el aviso de permiso incorrecto en sistemas con bits de
// Unix: los datos del préstamo viven en ficheros 0600 (arranque §7).
func (r *DoctorResult) warnPermiso(path, mode string) {
	if runtime.GOOS == "windows" {
		return
	}
	if mode != "0600" {
		r.Warnings = append(r.Warnings,
			fmt.Sprintf("el fichero %s tiene permiso %s; debería ser 0600", path, mode))
	}
}

// statMode devuelve el permiso del fichero formateado a cuatro dígitos
// octales ("0600"), o ok=false si no existe. Un fichero inexistente no es un
// problema para un doctor: es el estado de una instalación nueva.
func statMode(path string) (string, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return "", false
	}
	return fmt.Sprintf("%04o", info.Mode().Perm()), true
}

// networkHost deriva el host:puerto de la comprobación de red del host que
// publica el catálogo (EndpointBase, la única fuente de esa URL). Sin puerto
// explícito, el 443 de https.
func networkHost(catalog *bde.Catalog) string {
	u, err := url.Parse(catalog.EndpointBase())
	if err != nil || u.Host == "" {
		return "app.bde.es:443"
	}
	if u.Port() == "" {
		return u.Host + ":443"
	}
	return u.Host
}

// dialTCP es la sonda de red de producción: abre una conexión TCP y la
// cierra. No hay HTTP, no hay petición, no hay caché.
func dialTCP(ctx context.Context, host string) error {
	d := net.Dialer{Timeout: networkTimeout}
	conn, err := d.DialContext(ctx, "tcp", host)
	if err != nil {
		// El detalle útil es la causa (DNS, conexión rechazada, timeout);
		// net ya la trae, sin secretos.
		return fmt.Errorf("conexión a %s: %v", host, err)
	}
	_ = conn.Close()
	return nil
}
