package bde

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/aam9063/hipoteca-cli/internal/model"
)

const (
	// cacheFormatVersion es la versión del formato de los ficheros de caché.
	// Una entrada con otra versión es de otro formato: se ignora y se vuelve a
	// pedir (decisión 4).
	cacheFormatVersion = 1
	// latestTTL es la vida del último valor y de todo lo que puede cambiar al
	// publicarse un mes nuevo (docs/series.md §7: el mes en curso y el último
	// valor, 24 h).
	latestTTL = 24 * time.Hour
)

const (
	kindLatest  = "latest"
	kindHistory = "history"
)

// Cache guarda las respuestas del Banco bajo el directorio dado
// (config.CacheDir()). Es descartable (datos públicos regenerables): se escribe
// sin maquinaria atómica pero se valida al leer, y un fichero corrupto, de otro
// formato o de otra versión se ignora y se vuelve a pedir a la red. Nunca un
// error duro por caché (decisión 4).
//
// Política:
//   - Último valor (favoritas): TTL de 24 h.
//   - Histórico (listaSeries): los meses ya publicados no cambian. Una entrada
//     de un año cerrado —el último mes guardado es el diciembre del año
//     pedido— no caduca nunca. Los rangos abiertos (30M, 60M, MAX) y los años
//     aún abiertos alcanzan meses posteriores al último guardado, así que
//     caducan con la misma TTL de 24 h y se refrescan.
//
// Now es el reloj, inyectable para que los tests adelanten el tiempo sin dormir.
type Cache struct {
	dir string
	Now func() time.Time
}

// NewCache construye la caché sobre dir. El directorio se crea al escribir,
// con la mejor voluntad: si no se puede, la caché simplemente no existe.
func NewCache(dir string) *Cache {
	return &Cache{dir: dir, Now: time.Now}
}

// cacheObservation es una observación serializada: el valor viaja como texto
// decimal exacto, que es como model.Decimal lo renderiza, y nunca como número
// binario.
type cacheObservation struct {
	Code  string    `json:"code,omitempty"`
	Month time.Time `json:"month"`
	Value string    `json:"value"`
}

// cacheSeries son los metadatos de publicación que el histórico debe
// conservar al servirse de caché.
type cacheSeries struct {
	Code             string             `json:"code"`
	Description      string             `json:"description"`
	DescriptionShort string             `json:"description_short"`
	Frequency        string             `json:"frequency"`
	Decimals         int                `json:"decimals"`
	Unit             string             `json:"unit"`
	Start            time.Time          `json:"start"`
	End              time.Time          `json:"end"`
	Observations     []cacheObservation `json:"observations"`
}

// cacheEntry es el sobre de toda entrada: versión de formato, tipo, clave
// canónica de la petición, momento de guardado y carga útil.
type cacheEntry struct {
	FormatVersion int                `json:"format_version"`
	Kind          string             `json:"kind"`
	Key           string             `json:"key"`
	StoredAt      time.Time          `json:"stored_at"`
	Rango         string             `json:"rango,omitempty"`
	Observations  []cacheObservation `json:"observations,omitempty"`
	Series        *cacheSeries       `json:"series,omitempty"`
}

// GetLatest lee la última respuesta de favoritas si sigue viva (24 h).
func (c *Cache) GetLatest(key string) ([]Observation, bool) {
	e, ok := c.read(kindLatest, key)
	if !ok || len(e.Observations) == 0 {
		return nil, false
	}
	if e.StoredAt.Add(latestTTL).Before(c.Now()) {
		return nil, false
	}
	obs, err := observationsFromCache(e.Observations)
	if err != nil {
		return nil, false // valor no convertible: entrada corrupta, se ignora
	}
	return obs, true
}

// PutLatest guarda la última respuesta de favoritas. Es la mejor voluntad: un
// fallo de escritura se ignora (la caché es descartable, decisión 4).
func (c *Cache) PutLatest(key string, obs []Observation) {
	c.write(cacheEntry{
		FormatVersion: cacheFormatVersion,
		Kind:          kindLatest,
		Key:           key,
		StoredAt:      c.Now(),
		Observations:  observationsToCache(obs),
	})
}

// GetHistory lee el histórico guardado para la clave dada. Sirve sin límite de
// edad si el rango es un año cerrado (ver closedYear); en otro caso rige la
// TTL de 24 h.
func (c *Cache) GetHistory(key string, r Rango) (Series, bool) {
	e, ok := c.read(kindHistory, key)
	if !ok || e.Series == nil || e.Rango != r.String() {
		return Series{}, false
	}
	if !closedYear(r, e.Series.Observations) && e.StoredAt.Add(latestTTL).Before(c.Now()) {
		return Series{}, false
	}
	s, err := seriesFromCache(e.Series)
	if err != nil {
		return Series{}, false // entrada corrupta: se ignora y se vuelve a pedir
	}
	return s, true
}

// PutHistory guarda el histórico con su rango y metadatos.
func (c *Cache) PutHistory(key string, r Rango, s Series) {
	c.write(cacheEntry{
		FormatVersion: cacheFormatVersion,
		Kind:          kindHistory,
		Key:           key,
		StoredAt:      c.Now(),
		Rango:         r.String(),
		Series:        seriesToCache(s),
	})
}

// closedYear informa de si el rango pedido es un año cuyos doce meses ya están
// guardados (el último mes almacenado es el diciembre de ese año). Los meses
// publicados no cambian: esa entrada no caduca nunca; sólo se refresca cuando
// el rango pedido alcanza un mes posterior al último guardado.
func closedYear(r Rango, obs []cacheObservation) bool {
	y, ok := r.year()
	if !ok || len(obs) == 0 {
		return false
	}
	last := obs[len(obs)-1].Month // orden ascendente: el último es el más reciente
	return last.Year() == y && last.Month() == time.December
}

// path es el nombre canónico del fichero: kind + hash de la clave de la
// petición. La clave canónica (códigos en el orden pedido, o código|rango) va
// dentro y se verifica al leer.
func (c *Cache) path(kind, key string) string {
	sum := sha256.Sum256([]byte(kind + "|" + key))
	return filepath.Join(c.dir, kind+"-"+hex.EncodeToString(sum[:])+".json")
}

// read lee y valida el sobre. Cualquier desviación —no existe, JSON roto, otra
// versión de formato, otro tipo, otra clave— es un fallo silencioso: la entrada
// se trata como inexistente y se vuelve a pedir (decisión 4).
func (c *Cache) read(kind, key string) (cacheEntry, bool) {
	var e cacheEntry
	data, err := os.ReadFile(c.path(kind, key))
	if err != nil {
		return e, false
	}
	if err := json.Unmarshal(data, &e); err != nil {
		return e, false
	}
	if e.FormatVersion != cacheFormatVersion || e.Kind != kind || e.Key != key {
		return e, false
	}
	return e, true
}

// write serializa y escribe la entrada con la mejor voluntad: fallar no es
// opción, porque perder la petición por un problema de caché sería peor que no
// tener caché.
func (c *Cache) write(e cacheEntry) {
	data, err := json.Marshal(e)
	if err != nil {
		return
	}
	if err := os.MkdirAll(c.dir, 0o700); err != nil {
		return
	}
	_ = os.WriteFile(c.path(e.Kind, e.Key), data, 0o600)
}

func observationsToCache(obs []Observation) []cacheObservation {
	out := make([]cacheObservation, 0, len(obs))
	for _, o := range obs {
		out = append(out, cacheObservation{Code: o.Code, Month: o.Month, Value: o.Value.String()})
	}
	return out
}

func observationsFromCache(in []cacheObservation) ([]Observation, error) {
	obs := make([]Observation, 0, len(in))
	for _, o := range in {
		v, err := model.NewDecimal(o.Value)
		if err != nil {
			return nil, err
		}
		obs = append(obs, Observation{Code: o.Code, Month: o.Month, Value: v})
	}
	return obs, nil
}

func seriesToCache(s Series) *cacheSeries {
	return &cacheSeries{
		Code:             s.Code,
		Description:      s.Description,
		DescriptionShort: s.DescriptionShort,
		Frequency:        s.Frequency,
		Decimals:         s.Decimals,
		Unit:             s.Unit,
		Start:            s.Start,
		End:              s.End,
		Observations:     observationsToCache(s.Observations),
	}
}

func seriesFromCache(cs *cacheSeries) (Series, error) {
	obs, err := observationsFromCache(cs.Observations)
	if err != nil {
		return Series{}, err
	}
	return Series{
		Code:             cs.Code,
		Description:      cs.Description,
		DescriptionShort: cs.DescriptionShort,
		Frequency:        cs.Frequency,
		Decimals:         cs.Decimals,
		Unit:             cs.Unit,
		Start:            cs.Start,
		End:              cs.End,
		Observations:     obs,
	}, nil
}
