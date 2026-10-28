package bde

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// newTestCache crea una caché en un directorio temporal con un reloj que el
// test puede adelantar sin dormir.
func newTestCache(t *testing.T) (*Cache, *time.Time) {
	t.Helper()
	c := NewCache(t.TempDir())
	now := time.Now()
	c.Now = func() time.Time { return now }
	return c, &now
}

func TestCacheUltimoValorCaducaA24h(t *testing.T) {
	ts, c := newFixtureClient(t, fixture(t, "favoritas-14-indices.json"), 0)
	cache, now := newTestCache(t)
	c.cache = cache

	_, err := c.Latest([]string{"euribor-1y"})
	assertNoErr(t, err)
	_, err = c.Latest([]string{"euribor-1y"})
	assertNoErr(t, err)
	if got := ts.hits.Load(); got != 1 {
		t.Fatalf("la segunda llamada dentro de la TTL no debe tocar el servidor; peticiones = %d", got)
	}

	*now = now.Add(25 * time.Hour)
	_, err = c.Latest([]string{"euribor-1y"})
	assertNoErr(t, err)
	if got := ts.hits.Load(); got != 2 {
		t.Fatalf("pasadas 24 h la entrada debe caducar y refrescar; peticiones = %d", got)
	}
}

func TestCacheHistoricoCerradoNoCaduca(t *testing.T) {
	ts, c := newFixtureClient(t, fixture(t, "listaSeries-euribor-1y-2025.json"), 0)
	cache, now := newTestCache(t)
	c.cache = cache

	r, err := RangoAnio(2025)
	assertNoErr(t, err)
	s1, err := c.History("euribor-1y", r)
	assertNoErr(t, err)

	*now = now.Add(1000 * time.Hour) // meses enteros: el año 2025 ya está cerrado
	s2, err := c.History("euribor-1y", r)
	assertNoErr(t, err)
	if got := ts.hits.Load(); got != 1 {
		t.Fatalf("el histórico cerrado no debe caducar nunca; peticiones = %d", got)
	}
	assertSameSeries(t, s1, s2)
}

func TestCacheHistoricoAbiertoCaduca(t *testing.T) {
	// 30M incluye meses que aún pueden publicarse: rango abierto, TTL de 24 h.
	ts, c := newFixtureClient(t, fixture(t, "listaSeries-euribor-1y-30M.json"), 0)
	cache, now := newTestCache(t)
	c.cache = cache

	_, err := c.History("euribor-1y", Rango30M)
	assertNoErr(t, err)
	_, err = c.History("euribor-1y", Rango30M)
	assertNoErr(t, err)
	if got := ts.hits.Load(); got != 1 {
		t.Fatalf("dentro de la TTL debe servirse de caché; peticiones = %d", got)
	}

	*now = now.Add(25 * time.Hour)
	_, err = c.History("euribor-1y", Rango30M)
	assertNoErr(t, err)
	if got := ts.hits.Load(); got != 2 {
		t.Fatalf("el rango abierto debe refrescar tras 24 h; peticiones = %d", got)
	}
}

// TestCacheHistoricoAnioEnCurso: el año 2026 del fixture termina en septiembre
// (no está cerrado), así que el rango sigue alcanzando meses posteriores al
// último guardado y caduca con la TTL.
func TestCacheHistoricoAnioEnCurso(t *testing.T) {
	ts, c := newFixtureClient(t, fixture(t, "listaSeries-euribor-1y-2026.json"), 0)
	cache, now := newTestCache(t)
	c.cache = cache

	r, err := RangoAnio(2026)
	assertNoErr(t, err)
	_, err = c.History("euribor-1y", r)
	assertNoErr(t, err)
	_, err = c.History("euribor-1y", r)
	assertNoErr(t, err)
	if got := ts.hits.Load(); got != 1 {
		t.Fatalf("dentro de la TTL debe servirse de caché; peticiones = %d", got)
	}

	*now = now.Add(25 * time.Hour)
	_, err = c.History("euribor-1y", r)
	assertNoErr(t, err)
	if got := ts.hits.Load(); got != 2 {
		t.Fatalf("el año aún abierto debe refrescar tras 24 h; peticiones = %d", got)
	}
}

func TestCacheCorruptoSeIgnora(t *testing.T) {
	ts, c := newFixtureClient(t, fixture(t, "favoritas-14-indices.json"), 0)
	cache, _ := newTestCache(t)
	c.cache = cache
	key := "D_1NBAF472"

	// Un archivo que no es JSON.
	if err := os.WriteFile(cache.path("latest", key), []byte("no soy json"), 0o600); err != nil {
		t.Fatalf("escribir caché corrupta: %v", err)
	}
	obs, err := c.Latest([]string{"euribor-1y"})
	assertNoErr(t, err)
	if got := obsByCode(t, obs, "D_1NBAF472").Value.String(); got != "3.247" {
		t.Fatalf("la caché corrupta debe ignorarse y refetch: %q", got)
	}
	if got := ts.hits.Load(); got != 1 {
		t.Fatalf("peticiones = %d, se esperaba 1", got)
	}

	// Un JSON válido pero de otro formato (sin format_version conocida).
	otro := []byte(`{"hola":"soy otro formato"}`)
	if err := os.WriteFile(cache.path("latest", key), otro, 0o600); err != nil {
		t.Fatalf("escribir caché extranjera: %v", err)
	}
	obs, err = c.Latest([]string{"euribor-1y"})
	assertNoErr(t, err)
	if got := obsByCode(t, obs, "D_1NBAF472").Value.String(); got != "3.247" {
		t.Fatalf("la caché de otro formato debe ignorarse: %q", got)
	}

	// Una versión de formato futura tampoco se lee.
	futura := []byte(`{"format_version":99,"kind":"latest","key":"D_1NBAF472","stored_at":"2020-01-01T00:00:00Z","observations":[{"code":"D_1NBAF472","month":"2020-01-01T00:00:00Z","value":"9.999"}]}`)
	if err := os.WriteFile(cache.path("latest", key), futura, 0o600); err != nil {
		t.Fatalf("escribir caché futura: %v", err)
	}
	obs, err = c.Latest([]string{"euribor-1y"})
	assertNoErr(t, err)
	if got := obsByCode(t, obs, "D_1NBAF472").Value.String(); got != "3.247" {
		t.Fatalf("la caché de otra versión debe ignorarse, nunca servir 9.999: %q", got)
	}
}

// TestCacheEscribeEntradaVersionada: la entrada escrita lleva su versión de
// formato y el valor exacto como texto decimal.
func TestCacheEscribeEntradaVersionada(t *testing.T) {
	_, c := newFixtureClient(t, fixture(t, "favoritas-14-indices.json"), 0)
	cache, _ := newTestCache(t)
	c.cache = cache

	_, err := c.Latest([]string{"euribor-1y"})
	assertNoErr(t, err)
	data, err := os.ReadFile(cache.path("latest", "D_1NBAF472"))
	assertNoErr(t, err)
	var entry struct {
		FormatVersion int    `json:"format_version"`
		Value         string `json:"-"`
	}
	if err := json.Unmarshal(data, &entry); err != nil {
		t.Fatalf("la entrada de caché no es JSON válido: %v", err)
	}
	if entry.FormatVersion != 1 {
		t.Fatalf("format_version = %d, se esperaba 1", entry.FormatVersion)
	}
	if !strings.Contains(string(data), `"3.247"`) {
		t.Fatalf("la caché debe guardar el valor exacto como texto decimal: %s", data)
	}
}

// TestCacheDirectorioNoEscribible: la caché es descartable; un fallo de
// escritura nunca rompe la petición (decisión 4).
func TestCacheDirectorioNoEscribible(t *testing.T) {
	ts, c := newFixtureClient(t, fixture(t, "favoritas-14-indices.json"), 0)
	bloqueo := filepath.Join(t.TempDir(), "archivo")
	if err := os.WriteFile(bloqueo, []byte("x"), 0o600); err != nil {
		t.Fatalf("preparar el bloqueo: %v", err)
	}
	c.cache = NewCache(bloqueo) // MkdirAll fallará: es un fichero, no un directorio

	obs, err := c.Latest([]string{"euribor-1y"})
	assertNoErr(t, err)
	if got := obsByCode(t, obs, "D_1NBAF472").Value.String(); got != "3.247" {
		t.Fatalf("un fallo de escritura de caché no debe afectar al resultado: %q", got)
	}
	if got := ts.hits.Load(); got != 1 {
		t.Fatalf("peticiones = %d, se esperaba 1", got)
	}
}

func assertSameSeries(t *testing.T, a, b Series) {
	t.Helper()
	if a.Code != b.Code || len(a.Observations) != len(b.Observations) {
		t.Fatalf("las series servidas de caché difieren: %+v vs %+v", a, b)
	}
	for i := range a.Observations {
		if ym(a.Observations[i].Month) != ym(b.Observations[i].Month) ||
			a.Observations[i].Value.String() != b.Observations[i].Value.String() {
			t.Fatalf("observación %d difiere en caché: %v vs %v", i, a.Observations[i], b.Observations[i])
		}
	}
}
