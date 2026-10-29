package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/aam9063/hipoteca-cli/internal/bde"
	"github.com/aam9063/hipoteca-cli/internal/errs"
	"github.com/aam9063/hipoteca-cli/internal/model"
)

// ----------------------------------------------------------------------------
// Fuente de pruebas: sirve las capturas reales de testdata/bde por HTTP, con
// la misma forma de petición que la API (favoritas / listaSeries), de modo que
// la capa app se ejerce contra respuestas reales sin tocar la red. Ningún
// test de este paquete sale de la máquina (regla dura del arranque).
// ----------------------------------------------------------------------------

// fixtureServer levanta un servidor local que contesta con los fixtures de
// testdata/bde. El rango 60M no tiene captura: se sirve un histórico
// descendente generado en el test con la misma forma del contrato.
func fixtureServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/favoritas", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(readBDEFixture(t, "favoritas-14-indices.json"))
	})
	mux.HandleFunc("/listaSeries", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("rango") {
		case "30M":
			_, _ = w.Write(readBDEFixture(t, "listaSeries-euribor-1y-30M.json"))
		case "60M":
			_, _ = w.Write([]byte(inline60M(t)))
		case "MAX":
			_, _ = w.Write(readBDEFixture(t, "listaSeries-euribor-1y-MAX.json"))
		case "2025":
			_, _ = w.Write(readBDEFixture(t, "listaSeries-euribor-1y-2025.json"))
		case "2026":
			_, _ = w.Write(readBDEFixture(t, "listaSeries-euribor-1y-2026.json"))
		default:
			_, _ = w.Write(readBDEFixture(t, "error-listaSeries-rango-invalido.json"))
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func readBDEFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "bde", name))
	if err != nil {
		t.Fatalf("leer el fixture %s: %v", name, err)
	}
	return data
}

// inline60M construye un listaSeries descendente de 60 meses (2026-09 hacia
// atrás), la misma forma que el contrato de docs/series.md §3.2.
func inline60M(t *testing.T) string {
	t.Helper()
	fechas := make([]string, 0, 60)
	valores := make([]string, 0, 60)
	for i := 0; i < 60; i++ {
		m := time.Date(2026, time.September, 1, 8, 15, 0, 0, time.UTC).AddDate(0, -i, 0)
		fechas = append(fechas, m.Format("2006-01-02T15:04:05Z"))
		valores = append(valores, fmt.Sprintf("%.3f", 2.0+float64(i)/1000))
	}
	return fmt.Sprintf(`[{"serie":"D_1NBAF472","descripcion":"Euribor a un año","descripcionCorta":"Euribor a un año","codFrecuencia":"M","decimales":3,"simbolo":"%%","fechaInicio":"1999-01-01T09:15:00Z","fechaFin":"2026-09-01T08:15:00Z","fechas":[%s],"valores":[%s]}]`,
		quoteEach(fechas), strings.Join(valores, ","))
}

func quoteEach(in []string) string {
	out := make([]string, len(in))
	for i, v := range in {
		out[i] = `"` + v + `"`
	}
	return strings.Join(out, ",")
}

// fakeSource es la fuente de pruebas: pide por HTTP al servidor de fixtures y
// decodifica al mínimo del contrato, igual que hace internal/bde con la API
// real (json.Number para no perder el texto exacto del valor).
type fakeSource struct {
	t    *testing.T
	base string
}

func newFakeSource(t *testing.T) fakeSource {
	return fakeSource{t: t, base: fixtureServer(t).URL}
}

func (f fakeSource) get(ctx context.Context, path string, query url.Values) []byte {
	f.t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.base+path+"?"+query.Encode(), nil)
	if err != nil {
		f.t.Fatalf("construir la petición de prueba: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		f.t.Fatalf("petición de prueba: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		f.t.Fatalf("leer la respuesta de prueba: %v", err)
	}
	return body
}

func (f fakeSource) Latest(ctx context.Context, ids []string) ([]bde.Observation, error) {
	cat, err := bde.LoadCatalog(nil)
	if err != nil {
		return nil, err
	}
	codes := make([]string, 0, len(ids))
	for _, id := range ids {
		idx, err := cat.Lookup(id)
		if err != nil {
			return nil, err
		}
		codes = append(codes, idx.Code)
	}
	body := f.get(ctx, "/favoritas", url.Values{"series": {strings.Join(codes, ",")}})
	var elems []struct {
		Serie      string      `json:"serie"`
		ErrNum     *int        `json:"errNum"`
		FechaValor string      `json:"fechaValor"`
		Valor      json.Number `json:"valor"`
	}
	if err := json.Unmarshal(body, &elems); err != nil {
		return nil, err
	}
	want := map[string]bool{}
	for _, c := range codes {
		want[c] = true
	}
	obs := make([]bde.Observation, 0, len(codes))
	for _, e := range elems {
		if !want[e.Serie] {
			continue // el fixture trae las 14 series; sólo cuentan las pedidas
		}
		m, err := time.Parse(time.RFC3339, e.FechaValor)
		if err != nil {
			return nil, err
		}
		v, err := model.NewDecimal(e.Valor.String())
		if err != nil {
			return nil, err
		}
		obs = append(obs, bde.Observation{
			Code:  e.Serie,
			Month: time.Date(m.Year(), m.Month(), 1, 0, 0, 0, 0, time.UTC),
			Value: v,
		})
	}
	return obs, nil
}

func (f fakeSource) History(ctx context.Context, id string, rango bde.Rango) (bde.Series, error) {
	cat, err := bde.LoadCatalog(nil)
	if err != nil {
		return bde.Series{}, err
	}
	idx, err := cat.Lookup(id)
	if err != nil {
		return bde.Series{}, err
	}
	body := f.get(ctx, "/listaSeries", url.Values{"series": {idx.Code}, "rango": {rango.String()}})
	var elems []struct {
		Serie       string        `json:"serie"`
		Descripcion string        `json:"descripcion"`
		FechaInicio string        `json:"fechaInicio"`
		FechaFin    string        `json:"fechaFin"`
		Fechas      []string      `json:"fechas"`
		Valores     []json.Number `json:"valores"`
	}
	if err := json.Unmarshal(body, &elems); err != nil {
		return bde.Series{}, err
	}
	if len(elems) != 1 {
		return bde.Series{}, fmt.Errorf("el servidor de prueba devolvió %d series", len(elems))
	}
	e := elems[0]
	month := func(instant string) (time.Time, error) {
		m, err := time.Parse(time.RFC3339, instant)
		if err != nil {
			return time.Time{}, err
		}
		return time.Date(m.Year(), m.Month(), 1, 0, 0, 0, 0, time.UTC), nil
	}
	start, err := month(e.FechaInicio)
	if err != nil {
		return bde.Series{}, err
	}
	end, err := month(e.FechaFin)
	if err != nil {
		return bde.Series{}, err
	}
	// La API contesta descendente y el cliente real la invierte una vez; la
	// fuente de pruebas reproduce exactamente ese resultado.
	obs := make([]bde.Observation, 0, len(e.Fechas))
	for i := len(e.Fechas) - 1; i >= 0; i-- {
		m, err := month(e.Fechas[i])
		if err != nil {
			return bde.Series{}, err
		}
		v, err := model.NewDecimal(e.Valores[i].String())
		if err != nil {
			return bde.Series{}, err
		}
		obs = append(obs, bde.Observation{Code: e.Serie, Month: m, Value: v})
	}
	return bde.Series{
		Code:         e.Serie,
		Description:  e.Descripcion,
		Start:        start,
		End:          end,
		Observations: obs,
	}, nil
}

// failingSource falla siempre con el error que lleva: sirve para comprobar
// que app propaga el sobre del contrato sin degradarlo.
type failingSource struct{ err error }

func (f failingSource) Latest(context.Context, []string) ([]bde.Observation, error) {
	return nil, f.err
}

func (f failingSource) History(context.Context, string, bde.Rango) (bde.Series, error) {
	return bde.Series{}, f.err
}

// ----------------------------------------------------------------------------
// Tests
// ----------------------------------------------------------------------------

// index latest: el último valor del fixture, en el orden pedido, con la
// atribución de datos sin procesar (P-05).
func TestLatestIndicesDevuelveElUltimoValorDelFixture(t *testing.T) {
	src := newFakeSource(t)
	cat, err := bde.LoadCatalog(nil)
	if err != nil {
		t.Fatalf("cargar el catálogo: %v", err)
	}
	res, err := LatestIndices(context.Background(), src, cat, []string{"euribor-1y"})
	if err != nil {
		t.Fatalf("LatestIndices: %v", err)
	}
	if res.SchemaVersion != model.SchemaVersion {
		t.Errorf("schema_version = %d, se esperaba %d", res.SchemaVersion, model.SchemaVersion)
	}
	if len(res.Indices) != 1 {
		t.Fatalf("la salida trae %d índices, se esperaba 1", len(res.Indices))
	}
	got := res.Indices[0]
	if got.ID != "euribor-1y" || got.Code != "D_1NBAF472" {
		t.Errorf("id/código = %q/%q, se esperaba euribor-1y/D_1NBAF472", got.ID, got.Code)
	}
	if got.Name != "Euríbor a 1 año" {
		t.Errorf("name = %q, se esperaba el nombre del catálogo", got.Name)
	}
	if got.Month != "2026-09" {
		t.Errorf("month = %q, se esperaba 2026-09", got.Month)
	}
	if got.Value.String() != "3.247" {
		t.Errorf("value = %q, se esperaba el texto exacto del fixture 3.247", got.Value.String())
	}
	want, err := model.RawDataAttribution("2026-09-01")
	if err != nil {
		t.Fatalf("construir la atribución esperada: %v", err)
	}
	if res.Attribution != want {
		t.Errorf("attribution = %+v, se esperaba la de datos sin procesar con data_updated_at 2026-09-01: %+v", res.Attribution, want)
	}
}

// Varios ids: el array de la salida sigue el orden pedido, no el del fixture.
func TestLatestIndicesMantieneElOrdenPedido(t *testing.T) {
	src := newFakeSource(t)
	cat, err := bde.LoadCatalog(nil)
	if err != nil {
		t.Fatalf("cargar el catálogo: %v", err)
	}
	res, err := LatestIndices(context.Background(), src, cat, []string{"euribor-1w", "euribor-1y"})
	if err != nil {
		t.Fatalf("LatestIndices: %v", err)
	}
	if len(res.Indices) != 2 {
		t.Fatalf("la salida trae %d índices, se esperaban 2", len(res.Indices))
	}
	if res.Indices[0].ID != "euribor-1w" || res.Indices[0].Value.String() != "2.289" {
		t.Errorf("primero = %s %s, se esperaba euribor-1w 2.289", res.Indices[0].ID, res.Indices[0].Value.String())
	}
	if res.Indices[1].ID != "euribor-1y" || res.Indices[1].Value.String() != "3.247" {
		t.Errorf("segundo = %s %s, se esperaba euribor-1y 3.247", res.Indices[1].ID, res.Indices[1].Value.String())
	}
}

// Un id desconocido es not_found/4 y nombra el id.
func TestLatestIndicesIdDesconocidoEsNotFound(t *testing.T) {
	src := newFakeSource(t)
	cat, err := bde.LoadCatalog(nil)
	if err != nil {
		t.Fatalf("cargar el catálogo: %v", err)
	}
	_, err = LatestIndices(context.Background(), src, cat, []string{"inventado"})
	var e *errs.Error
	if !errors.As(err, &e) {
		t.Fatalf("el error debe llevar el sobre del contrato, got %v", err)
	}
	if e.Kind != errs.KindNotFound {
		t.Errorf("kind = %q, se esperaba not_found", e.Kind)
	}
	if !strings.Contains(e.Error(), "inventado") {
		t.Errorf("el mensaje debe nombrar el id pedido, got %q", e.Error())
	}
}

// Sin ids es un error de uso, no una petición vacía.
func TestLatestIndicesSinIdsEsUsage(t *testing.T) {
	src := newFakeSource(t)
	cat, err := bde.LoadCatalog(nil)
	if err != nil {
		t.Fatalf("cargar el catálogo: %v", err)
	}
	_, err = LatestIndices(context.Background(), src, cat, nil)
	var e *errs.Error
	if !errors.As(err, &e) || e.Kind != errs.KindUsage {
		t.Fatalf("sin ids se esperaba usage, got %v", err)
	}
}

// index list: todos los índices del catálogo con su último valor.
func TestIndexListDevuelveTodoElCatalogo(t *testing.T) {
	src := newFakeSource(t)
	cat, err := bde.LoadCatalog(nil)
	if err != nil {
		t.Fatalf("cargar el catálogo: %v", err)
	}
	res, err := IndexList(context.Background(), src, cat)
	if err != nil {
		t.Fatalf("IndexList: %v", err)
	}
	if len(res.Indices) != len(cat.IDs()) {
		t.Fatalf("la salida trae %d índices y el catálogo tiene %d", len(res.Indices), len(cat.IDs()))
	}
	var euribor *IndexValue
	for i := range res.Indices {
		if res.Indices[i].ID == "euribor-1y" {
			euribor = &res.Indices[i]
		}
	}
	if euribor == nil {
		t.Fatal("el listado no incluye euribor-1y")
	}
	if euribor.Value.String() != "3.247" || euribor.Month != "2026-09" {
		t.Errorf("euribor-1y = %s %s, se esperaba 3.247 en 2026-09", euribor.Value.String(), euribor.Month)
	}
	if !res.Attribution.NotEndorsed {
		t.Error("not_endorsed debe ser true siempre (P-05)")
	}
	if res.Attribution.DataUpdatedAt != "2026-09-01" {
		t.Errorf("data_updated_at = %q, se esperaba 2026-09-01", res.Attribution.DataUpdatedAt)
	}
}

// index show: los cuatro rangos válidos del contrato (docs/series.md §2.3),
// cada uno contra su captura real.
func TestIndexHistoryConLosCuatroRangosValidos(t *testing.T) {
	cases := []struct {
		rango     string
		count     int
		primero   string
		ultimo    string
		lastValue string
	}{
		{"30M", 31, "2024-03", "2026-09", "3.247"},
		{"60M", 60, "2021-10", "2026-09", "2.000"},
		{"MAX", 333, "1999-01", "2026-09", "3.247"},
		{"2025", 12, "2025-01", "2025-12", "2.267"},
	}
	for _, tc := range cases {
		t.Run(tc.rango, func(t *testing.T) {
			src := newFakeSource(t)
			cat, err := bde.LoadCatalog(nil)
			if err != nil {
				t.Fatalf("cargar el catálogo: %v", err)
			}
			rango, err := bde.RangoAnio(2025)
			if tc.rango == "2025" {
				if err != nil {
					t.Fatalf("RangoAnio(2025): %v", err)
				}
			} else {
				switch tc.rango {
				case "30M":
					rango = bde.Rango30M
				case "60M":
					rango = bde.Rango60M
				case "MAX":
					rango = bde.RangoMAX
				}
			}
			res, err := IndexHistory(context.Background(), src, cat, "euribor-1y", rango)
			if err != nil {
				t.Fatalf("IndexHistory(%s): %v", tc.rango, err)
			}
			if res.Range != tc.rango {
				t.Errorf("range = %q, se esperaba %q", res.Range, tc.rango)
			}
			if res.Index.ID != "euribor-1y" || res.Index.Code != "D_1NBAF472" {
				t.Errorf("index = %s/%s, se esperaba euribor-1y/D_1NBAF472", res.Index.ID, res.Index.Code)
			}
			if len(res.Observations) != tc.count {
				t.Fatalf("la salida trae %d observaciones, se esperaban %d", len(res.Observations), tc.count)
			}
			if res.Observations[0].Month != tc.primero {
				t.Errorf("primera observación = %s, se esperaba %s (orden ascendente)", res.Observations[0].Month, tc.primero)
			}
			last := res.Observations[len(res.Observations)-1]
			if last.Month != tc.ultimo {
				t.Errorf("última observación = %s, se esperaba %s", last.Month, tc.ultimo)
			}
			if last.Value.String() != tc.lastValue {
				t.Errorf("último valor = %s, se esperaba %s", last.Value.String(), tc.lastValue)
			}
			// La atribución es la de datos sin procesar; data_updated_at es la
			// fechaFin de la serie completa (el mes más reciente publicado por
			// la fuente), no el final del rango pedido.
			want, err := model.RawDataAttribution("2026-09-01")
			if err != nil {
				t.Fatalf("construir la atribución esperada: %v", err)
			}
			if res.Attribution != want {
				t.Errorf("attribution = %+v, se esperaba %+v", res.Attribution, want)
			}
		})
	}
}

// El error de la fuente sube tal cual: kind network → código 6, sin
// reenvolverlo en otra cosa (main decide la salida a partir del sobre).
func TestAppPropagaElSobreDelErrorDeRed(t *testing.T) {
	src := failingSource{err: errs.New(errs.KindNetwork, "no se pudo contactar con la API del Banco de España", "comprueba la conexión a internet")}
	cat, err := bde.LoadCatalog(nil)
	if err != nil {
		t.Fatalf("cargar el catálogo: %v", err)
	}
	_, err = LatestIndices(context.Background(), src, cat, []string{"euribor-1y"})
	var e *errs.Error
	if !errors.As(err, &e) {
		t.Fatalf("el error debe llevar el sobre del contrato, got %v", err)
	}
	if e.Kind != errs.KindNetwork || errs.ExitCode(e.Kind) != 6 {
		t.Errorf("kind/código = %q/%d, se esperaba network/6", e.Kind, errs.ExitCode(e.Kind))
	}
}

// catalogVerifiedAt es la única fecha real que el binario asocia a los datos
// originales cuando no hay red: debe coincidir con verified_at de
// internal/bde/series.yaml, o la atribución de calc mentiría sobre su origen.
func TestCatalogVerifiedAtCoincideConSeriesYAML(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "bde", "series.yaml"))
	if err != nil {
		t.Fatalf("leer series.yaml: %v", err)
	}
	re := regexp.MustCompile(`(?m)^verified_at:\s*"([^"]+)"`)
	m := re.FindSubmatch(data)
	if m == nil {
		t.Fatal("series.yaml no declara verified_at")
	}
	if string(m[1]) != catalogVerifiedAt {
		t.Errorf("catalogVerifiedAt = %q, pero series.yaml declara verified_at %q: actualiza la constante", catalogVerifiedAt, string(m[1]))
	}
}
