package bde

import (
	_ "embed"
	"fmt"

	"github.com/aam9063/hipoteca-cli/internal/errs"
	"gopkg.in/yaml.v3"
)

// catalogSchemaVersion es la única schema_version de series.yaml que este
// paquete sabe leer; una Embedded con otra versión es un error, nunca una
// suposición.
const catalogSchemaVersion = 1

//go:embed series.yaml
var seriesYAML []byte

// Index es una entrada del catálogo de índices de series.yaml. `id` es el
// identificador estable que aceptan los comandos (renombrarlo es un cambio de
// contrato, P-09); `code` es el código de serie del Banco de España y es lo
// único que se puede sobrescribir desde la configuración (arranque §7).
type Index struct {
	ID             string `yaml:"id"`
	Code           string `yaml:"code"`
	Name           string `yaml:"name"`
	Family         string `yaml:"family"`
	Tenor          string `yaml:"tenor"`
	Frequency      string `yaml:"frequency"`
	Decimals       int    `yaml:"decimals"`
	Unit           string `yaml:"unit"`
	Description    string `yaml:"description"`
	APIDescription string `yaml:"api_description"`
	FirstValue     string `yaml:"first_value"`
}

// auxSeries es una serie del Banco que no es un índice hipotecario
// (docs/series.md §1.4): existe en la API y en el catálogo, pero nunca se
// ofrece como --index ni entra en ningún cálculo de cuota.
type auxSeries struct {
	Code     string `yaml:"code"`
	Name     string `yaml:"name"`
	Role     string `yaml:"role"`
	Unit     string `yaml:"unit"`
	Decimals int    `yaml:"decimals"`
}

// catalogFile es la forma de series.yaml tal cual está escrita. Las etiquetas
// yaml reemplazan al parser a mano: el mapeo campo a campo y el descifrado de
// escalares (comillas, escapes, `:` y `#` dentro de valores) los hace
// gopkg.in/yaml.v3.
type catalogFile struct {
	SchemaVersion int         `yaml:"schema_version"`
	VerifiedAt    string      `yaml:"verified_at"`
	EndpointBase  string      `yaml:"endpoint_base"`
	Language      string      `yaml:"language"`
	Indices       []Index     `yaml:"indices"`
	Auxiliary     []auxSeries `yaml:"auxiliary"`
}

// Catalog es el catálogo de series embebido (series.yaml, go:embed) con los
// overrides de la configuración ya aplicados. Una instancia construida nunca
// cambia: los overrides se aplican al cargar.
type Catalog struct {
	byID          map[string]Index
	ids           []string
	byCode        map[string]string
	aux           map[string]auxSeries
	schemaVersion int
	verifiedAt    string
	endpointBase  string
	language      string
}

// LoadCatalog parsea series.yaml embebido y aplica los overrides de
// Config.Indices (id → código del Banco). Un id que no existe en el catálogo
// es un error de configuración que nombra el id: los overrides reemplazan el
// `code` de un índice existente, nunca crean índices nuevos (decisión 3).
func LoadCatalog(overrides map[string]string) (*Catalog, error) {
	cat, err := parseCatalog(seriesYAML)
	if err != nil {
		return nil, errs.New(errs.KindInternal,
			"el catálogo de series embebido no es válido: "+err.Error(), "")
	}
	for id, code := range overrides {
		idx, ok := cat.byID[id]
		if !ok {
			return nil, errs.New(errs.KindInternal,
				fmt.Sprintf("la configuración sobrescribe el id %q, que no existe en el catálogo de series del Banco de España", id),
				"usa un id del catálogo o elimina esa entrada de [indices] en config.toml")
		}
		idx.Code = code
		cat.byID[id] = idx
	}
	cat.byCode = make(map[string]string, len(cat.ids))
	for _, id := range cat.ids {
		cat.byCode[cat.byID[id].Code] = id
	}
	return cat, nil
}

// Lookup resuelve un id del catálogo. Un id desconocido es not_found (código 4)
// y nombra el id pedido.
func (c *Catalog) Lookup(id string) (Index, error) {
	idx, ok := c.byID[id]
	if !ok {
		return Index{}, errs.New(errs.KindNotFound,
			fmt.Sprintf("el índice %q no existe en el catálogo de series", id),
			"usa la lista de índices del catálogo para ver los disponibles")
	}
	return idx, nil
}

// IDs devuelve los id del catálogo, en el orden de series.yaml.
func (c *Catalog) IDs() []string {
	out := make([]string, len(c.ids))
	copy(out, c.ids)
	return out
}

// EndpointBase es la base de la API según el catálogo embebido: la única
// fuente de esa URL.
func (c *Catalog) EndpointBase() string { return c.endpointBase }

// hasCode informa de si un código de serie del Banco sale del catálogo (índices
// con override aplicado, o series auxiliares). La clasificación de errores de
// docs/series.md §6 lo usa: un errNum sobre un código del catálogo es
// api_changed, sobre uno forzado por el usuario es not_found.
func (c *Catalog) hasCode(code string) bool {
	if _, ok := c.byCode[code]; ok {
		return true
	}
	_, ok := c.aux[code]
	return ok
}

// parseCatalog decodifica series.yaml con gopkg.in/yaml.v3. Una clave
// desconocida NO aborta: el catálogo es un dato que este proyecto genera,
// versiona y embebe, no un contrato con un tercero, así que un campo nuevo
// añadido a series.yaml debe ser ignorado por el binario actual hasta que el
// código lo empiece a usar. Las comprobaciones de contrato sí son estrictas y
// están aquí abajo: schema_version soportada, endpoint_base y language
// presentes, al menos un índice, y id y code únicos y presentes.
func parseCatalog(data []byte) (*Catalog, error) {
	var file catalogFile
	if err := yaml.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("series.yaml no es un YAML válido: %w", err)
	}
	if file.SchemaVersion != catalogSchemaVersion {
		return nil, fmt.Errorf("schema_version %d no soportada; se espera la %d", file.SchemaVersion, catalogSchemaVersion)
	}
	if file.EndpointBase == "" || file.Language == "" {
		return nil, fmt.Errorf("faltan endpoint_base o language en el catálogo")
	}
	if len(file.Indices) == 0 {
		return nil, fmt.Errorf("el catálogo no tiene ningún índice")
	}
	cat := &Catalog{
		byID:          map[string]Index{},
		byCode:        map[string]string{},
		aux:           map[string]auxSeries{},
		schemaVersion: file.SchemaVersion,
		verifiedAt:    file.VerifiedAt,
		endpointBase:  file.EndpointBase,
		language:      file.Language,
	}
	seenID := map[string]bool{}
	seenCode := map[string]bool{}
	for _, idx := range file.Indices {
		if idx.ID == "" || idx.Code == "" {
			return nil, fmt.Errorf("un índice no tiene id o code")
		}
		if seenID[idx.ID] {
			return nil, fmt.Errorf("id duplicado %q", idx.ID)
		}
		if seenCode[idx.Code] {
			return nil, fmt.Errorf("código duplicado %q", idx.Code)
		}
		seenID[idx.ID] = true
		seenCode[idx.Code] = true
		cat.byID[idx.ID] = idx
		cat.ids = append(cat.ids, idx.ID)
		cat.byCode[idx.Code] = idx.ID
	}
	// Como antes del parser a mano: si hubiera un código auxiliar repetido,
	// gana el último (no es un contrato comprobado, solo se conserva).
	for _, a := range file.Auxiliary {
		cat.aux[a.Code] = a
	}
	return cat, nil
}
