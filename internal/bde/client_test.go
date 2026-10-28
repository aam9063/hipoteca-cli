package bde

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aam9063/hipoteca-cli/internal/errs"
)

// fixture lee una captura real de testdata/bde. Ningún test toca la red: el
// httptest sirve exactamente esos bytes.
func fixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "bde", name))
	if err != nil {
		t.Fatalf("leer fixture %s: %v", name, err)
	}
	return data
}

// testServer sirve un cuerpo fijo y registra las peticiones recibidas, para
// contar cachés y reintentos y para afirmar los invariantes de la petición.
type testServer struct {
	hits  atomic.Int64
	mu    sync.Mutex
	query url.Values
	agent string
}

// httptestNewServer levanta un servidor local para el test y lo cierra al
// terminar; ningún test de internal/bde toca la red real.
func httptestNewServer(t *testing.T, h http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}

func newTestClient(t *testing.T, baseURL string) *HTTPClient {
	t.Helper()
	cat, err := LoadCatalog(nil)
	if err != nil {
		t.Fatalf("cargar catálogo: %v", err)
	}
	c := NewHTTPClient(cat)
	c.baseURL = baseURL
	c.retryWait = time.Millisecond
	return c
}

func newFixtureClient(t *testing.T, body []byte, status int) (*testServer, *HTTPClient) {
	t.Helper()
	ts := &testServer{}
	srv := httptestNewServer(t, func(w http.ResponseWriter, r *http.Request) {
		ts.hits.Add(1)
		ts.mu.Lock()
		ts.query = r.URL.Query()
		ts.agent = r.Header.Get("User-Agent")
		ts.mu.Unlock()
		if status != 0 {
			w.WriteHeader(status)
		}
		_, _ = w.Write(body)
	})
	return ts, newTestClient(t, srv.URL)
}

func assertErr(t *testing.T, err error, kind errs.Kind, code int) {
	t.Helper()
	if err == nil {
		t.Fatal("se esperaba un error, no hubo")
	}
	var e *errs.Error
	if !errors.As(err, &e) {
		t.Fatalf("el error no lleva el sobre del contrato (errs.Error): %v", err)
	}
	if e.Kind != kind {
		t.Fatalf("kind = %q, se esperaba %q (mensaje: %s)", e.Kind, kind, e.Message)
	}
	if got := errs.ExitCode(e.Kind); got != code {
		t.Fatalf("código de salida = %d, se esperaba %d", got, code)
	}
}

func assertNoErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
}

func asErrs(err error, target **errs.Error) bool { return errors.As(err, target) }

// obsByCode devuelve la observación de un código en la respuesta; el fixture de
// 14 índices contesta con todos los elementos del array, no sólo el pedido.
func obsByCode(t *testing.T, obs []Observation, code string) Observation {
	t.Helper()
	for _, o := range obs {
		if o.Code == code {
			return o
		}
	}
	t.Fatalf("la respuesta no trae la serie %s", code)
	return Observation{}
}

// ym devuelve el año-mes de un instante: lo único significativo de las fechas
// de la API (docs/series.md §5).
func ym(t time.Time) string { return t.Format("2006-01") }

// idsCatorce son los 14 id del catálogo en el orden de series.yaml; el orden
// del array de favoritas sigue el orden de los códigos pedidos.
var idsCatorce = []string{
	"euribor-1w", "euribor-1m", "euribor-3m", "euribor-6m", "euribor-1y",
	"irph", "irs-5y", "deuda-publica-2a6", "hipotecario-ue-1a5",
	"estr-1w", "estr-1m", "estr-3m", "estr-6m", "estr-1y",
}

var codigosCatorce = []string{
	"D_1NBAS972", "D_1NBAC972", "D_1NBAD972", "D_1NBAE972", "D_1NBAF472",
	"D_1T9H0000", "D_1T9H0011", "D_1T9H0004", "D_1T9H0010",
	"D_1NBAS572", "D_1NBAC572", "D_1NBAD572", "D_1NBAE572", "D_1NBAF572",
}

// valoresCatorce son los valores exactos del fixture favoritas-14-indices.json,
// como texto decimal: 3.45 y 2.44 conservan su escala original.
var valoresCatorce = map[string]string{
	"D_1NBAS972": "2.289",
	"D_1NBAC972": "2.414",
	"D_1NBAD972": "2.635",
	"D_1NBAE972": "2.922",
	"D_1NBAF472": "3.247",
	"D_1T9H0000": "3.189",
	"D_1T9H0011": "3.45",
	"D_1T9H0004": "2.983",
	"D_1T9H0010": "3.6",
	"D_1NBAS572": "2.44",
	"D_1NBAC572": "2.298",
	"D_1NBAD572": "2.231",
	"D_1NBAE572": "2.107",
	"D_1NBAF572": "2.034",
}

func TestLatestCatorceIndices(t *testing.T) {
	ts, c := newFixtureClient(t, fixture(t, "favoritas-14-indices.json"), 0)
	obs, err := c.Latest(idsCatorce)
	assertNoErr(t, err)
	if len(obs) != 14 {
		t.Fatalf("se esperaban 14 observaciones, hubo %d", len(obs))
	}
	for i, o := range obs {
		if o.Code != codigosCatorce[i] {
			t.Fatalf("observación %d: código %s, se esperaba %s (el orden sigue la petición)", i, o.Code, codigosCatorce[i])
		}
		if got := o.Value.String(); got != valoresCatorce[o.Code] {
			t.Fatalf("%s: valor %q, se esperaba el decimal exacto %q", o.Code, got, valoresCatorce[o.Code])
		}
		if got := ym(o.Month); got != "2026-09" {
			t.Fatalf("%s: mes %q, se esperaba 2026-09", o.Code, got)
		}
	}
	if ts.hits.Load() != 1 {
		t.Fatalf("peticiones = %d, se esperaba 1", ts.hits.Load())
	}
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if ts.query.Get("idioma") != "es" {
		t.Fatalf("la petición no lleva idioma=es: %v", ts.query)
	}
	if ts.query.Get("series") != strings.Join(codigosCatorce, ",") {
		t.Fatalf("series = %q, se esperaba %q", ts.query.Get("series"), strings.Join(codigosCatorce, ","))
	}
	if ts.agent != "hipoteca-cli/dev" {
		t.Fatalf("User-Agent = %q, se esperaba hipoteca-cli/dev", ts.agent)
	}
}

// TestLatestAuxiliares: el fixture mezcla éxito y error por elemento en el
// mismo array; los D_1TFECHABOE* no salen del catálogo → not_found/4, y las
// tres series auxiliares llegan como decimales exactos de escala 0.
func TestLatestAuxiliaresMezclaExitoYError(t *testing.T) {
	ts, c := newFixtureClient(t, fixture(t, "favoritas-series-auxiliares.json"), 0)
	codes := []string{"D_1T9H0000A", "D_1T9H0000B", "D_1T9H0000C", "D_1TFECHABOE", "D_1TFECHABOE2", "D_1TFECHABOE3", "D_1TFECHABOE4"}
	obs, err := c.LatestCodes(codes)
	assertErr(t, err, errs.KindNotFound, 4)
	if got := ts.hits.Load(); got != 1 {
		t.Fatalf("peticiones = %d, se esperaba 1", got)
	}
	if len(obs) != 3 {
		t.Fatalf("se esperaban 3 observaciones exitosas, hubo %d", len(obs))
	}
	want := map[string]string{"D_1T9H0000A": "4447519", "D_1T9H0000B": "24498", "D_1T9H0000C": "21"}
	for _, o := range obs {
		if got := o.Value.String(); got != want[o.Code] {
			t.Fatalf("%s: valor %q, se esperaba %q", o.Code, got, want[o.Code])
		}
	}
	for _, code := range []string{"D_1TFECHABOE", "D_1TFECHABOE2", "D_1TFECHABOE3", "D_1TFECHABOE4"} {
		if !strings.Contains(err.Error(), code) {
			t.Fatalf("el error no nombra la serie fallida %s: %s", code, err.Error())
		}
	}
}

// TestLatestSerieInexistenteArray: error por elemento (array) con un código
// forzado que no está en el catálogo → not_found/4.
func TestLatestSerieInexistenteArray(t *testing.T) {
	ts, c := newFixtureClient(t, fixture(t, "error-favoritas-serie-inexistente.json"), 0)
	obs, err := c.LatestCodes([]string{"XX"})
	assertErr(t, err, errs.KindNotFound, 4)
	if len(obs) != 0 {
		t.Fatalf("no se esperaban observaciones, hubo %d", len(obs))
	}
	if ts.hits.Load() != 1 {
		t.Fatalf("peticiones = %d, se esperaba 1", ts.hits.Load())
	}
}

// TestLatestCodigoDeCatalogo: si el código erróneo sale del catálogo, el
// contrato cambió → api_changed/7 (decisión 5).
func TestLatestCodigoDeCatalogoEsAPIChanged(t *testing.T) {
	body := []byte(`[{"codigo":"D_1NBAF472","errNum":404}]`)
	ts, c := newFixtureClient(t, body, 0)
	_, err := c.Latest([]string{"euribor-1y"})
	assertErr(t, err, errs.KindAPIChanged, 7)
	if ts.hits.Load() != 1 {
		t.Fatalf("peticiones = %d, se esperaba 1", ts.hits.Load())
	}
}

// TestLatestSinSeries: el objeto de error de favoritas sin `series` no puede
// ocurrir con un cliente que siempre la envía → api_changed/7.
func TestLatestFavoritasSinSeries(t *testing.T) {
	ts, c := newFixtureClient(t, fixture(t, "error-favoritas-sin-series.json"), 0)
	_, err := c.LatestCodes([]string{"D_1NBAF472"})
	assertErr(t, err, errs.KindAPIChanged, 7)
	if ts.hits.Load() != 1 {
		t.Fatalf("peticiones = %d, se esperaba 1", ts.hits.Load())
	}
}

func TestLatestSinIds(t *testing.T) {
	ts, c := newFixtureClient(t, []byte("[]"), 0)
	_, err := c.Latest(nil)
	assertErr(t, err, errs.KindUsage, 2)
	if ts.hits.Load() != 0 {
		t.Fatalf("no debió haber petición; hubo %d", ts.hits.Load())
	}
}

// TestLatestCuerpoHTML: la ruta inexistente del Banco responde HTML con 400.
func TestLatestCuerpoHTML(t *testing.T) {
	ts, c := newFixtureClient(t, []byte("<html><body>página de error del sitio</body></html>"), http.StatusBadRequest)
	_, err := c.Latest([]string{"euribor-1y"})
	assertErr(t, err, errs.KindAPIChanged, 7)
	if got := ts.hits.Load(); got != 1 {
		t.Fatalf("peticiones = %d, se esperaba 1", got)
	}
}

func TestLatest500ReintentaUnaVez(t *testing.T) {
	ts, c := newFixtureClient(t, []byte("error interno"), http.StatusInternalServerError)
	_, err := c.Latest([]string{"euribor-1y"})
	assertErr(t, err, errs.KindUpstreamUnavailable, 5)
	if got := ts.hits.Load(); got != 2 {
		t.Fatalf("peticiones = %d, se esperaban 2 (un reintento, nunca más)", got)
	}
}

func TestLatest429EsRateLimited(t *testing.T) {
	ts, c := newFixtureClient(t, []byte("demasiadas peticiones"), http.StatusTooManyRequests)
	_, err := c.Latest([]string{"euribor-1y"})
	assertErr(t, err, errs.KindRateLimited, 5)
	if got := ts.hits.Load(); got != 2 {
		t.Fatalf("peticiones = %d, se esperaban 2 (un reintento, nunca más)", got)
	}
}

// TestLatest500LuegoExito: el reintento único salva la petición.
func TestLatest500LuegoExito(t *testing.T) {
	fx := fixture(t, "favoritas-14-indices.json")
	var calls atomic.Int64
	srv := httptestNewServer(t, func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("error interno"))
			return
		}
		_, _ = w.Write(fx)
	})
	c := newTestClient(t, srv.URL)
	obs, err := c.Latest([]string{"euribor-1y"})
	assertNoErr(t, err)
	if got := obsByCode(t, obs, "D_1NBAF472").Value.String(); got != "3.247" {
		t.Fatalf("el reintento debía devolver el valor 3.247; hubo %q", got)
	}
	if calls.Load() != 2 {
		t.Fatalf("peticiones = %d, se esperaban 2", calls.Load())
	}
}

func TestLatestConexionRechazada(t *testing.T) {
	srv := httptestNewServer(t, func(w http.ResponseWriter, r *http.Request) {})
	url := srv.URL
	srv.Close() // el puerto queda cerrado: conexión rechazada, sin caché
	c := newTestClient(t, url)
	_, err := c.Latest([]string{"euribor-1y"})
	assertErr(t, err, errs.KindNetwork, 6)
}

func TestLatestIdDesconocido(t *testing.T) {
	ts, c := newFixtureClient(t, []byte("[]"), 0)
	_, err := c.Latest([]string{"indice-inventado"})
	assertErr(t, err, errs.KindNotFound, 4)
	if !strings.Contains(err.Error(), "indice-inventado") {
		t.Fatalf("el error debe nombrar el id: %s", err.Error())
	}
	if ts.hits.Load() != 0 {
		t.Fatalf("no debió haber petición; hubo %d", ts.hits.Load())
	}
}
