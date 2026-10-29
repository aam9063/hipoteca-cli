package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/aam9063/hipoteca-cli/internal/app"
	"github.com/aam9063/hipoteca-cli/internal/bde"
	"github.com/aam9063/hipoteca-cli/internal/errs"
	"github.com/aam9063/hipoteca-cli/internal/model"
)

// ----------------------------------------------------------------------------
// E2E de `index`: el árbol cobra completo con una fuente de datos de prueba,
// sin red. Se comprueba lo que ve el agente: código de salida, stdout con un
// único valor JSON y stderr vacío (P-01, arranque §6.1).
// ----------------------------------------------------------------------------

// stubSource devuelve lo que el test fija, sin tocar la red. El resultado
// network/6 de un servidor caído lo clasifica internal/bde (sus propios tests
// lo cubren); aquí se inyecta el sobre ya clasificado para comprobar que el
// árbol de comandos lo deja llegar a main sin degradarlo.
type stubSource struct {
	latest []bde.Observation
	series bde.Series
	err    error
}

func (s *stubSource) Latest(context.Context, []string) ([]bde.Observation, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.latest, nil
}

func (s *stubSource) History(context.Context, string, bde.Rango) (bde.Series, error) {
	if s.err != nil {
		return bde.Series{}, s.err
	}
	return s.series, nil
}

// depsCon construye el árbol con un runtime de prueba sobre la fuente dada y
// el catálogo embebido (estático, sin red).
func depsCon(t *testing.T, src app.Source) Deps {
	t.Helper()
	cat, err := bde.LoadCatalog(nil)
	if err != nil {
		t.Fatalf("cargar el catálogo: %v", err)
	}
	return Deps{Runtime: func() (*app.Runtime, error) {
		return &app.Runtime{Catalog: cat, Source: src}, nil
	}}
}

func runRoot(t *testing.T, deps Deps, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	cmd := NewRootCmdWithDeps(BuildInfo{Version: "test", Commit: "abc1234", Date: "2026-10-28"}, deps)
	var outBuf, errBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	cmd.SetErr(&errBuf)
	cmd.SetArgs(args)
	err = cmd.Execute()
	return outBuf.String(), errBuf.String(), err
}

// assertContractError comprueba que el error lleva el sobre del contrato con
// el kind y el código estables esperados.
func assertContractError(t *testing.T, err error, kind errs.Kind, code int) *errs.Error {
	t.Helper()
	if err == nil {
		t.Fatal("se esperaba un error, no hubo")
	}
	var e *errs.Error
	if !errors.As(err, &e) {
		t.Fatalf("el error debe llevar el sobre del contrato (errs.Error), got %v", err)
	}
	if e.Kind != kind {
		t.Errorf("kind = %q, se esperaba %q (mensaje: %s)", e.Kind, kind, e.Message)
	}
	if got := errs.ExitCode(e.Kind); got != code {
		t.Errorf("código de salida = %d, se esperaba %d", got, code)
	}
	return e
}

func assertEmpty(t *testing.T, what, out string) {
	t.Helper()
	if out != "" {
		t.Errorf("%s debe quedar vacío, got %q", what, out)
	}
}

// stdout con un único valor JSON terminado en salto de línea (P-01).
func assertSingleJSON(t *testing.T, out string) {
	t.Helper()
	if !strings.HasSuffix(out, "\n") {
		t.Errorf("el stdout debe terminar en un salto de línea, got %q", out)
	}
	dec := json.NewDecoder(strings.NewReader(out))
	var v any
	if err := dec.Decode(&v); err != nil {
		t.Fatalf("el stdout no es un JSON válido: %v\n%s", err, out)
	}
	if dec.More() {
		t.Errorf("el stdout debe contener un único valor JSON, got %q", out)
	}
}

// `hipoteca index latest euribor-1y`: un JSON en stdout y éxito.
func TestE2EIndexLatestEscribeUnJSONEnStdout(t *testing.T) {
	sept2026 := time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC)
	obs := []bde.Observation{{Code: "D_1NBAF472", Month: sept2026, Value: mustDecimal(t, "3.247")}}
	deps := depsCon(t, &stubSource{latest: obs})

	stdout, stderr, err := runRoot(t, deps, "index", "latest", "euribor-1y")
	if err != nil {
		t.Fatalf("index latest debe tener éxito, got %v", err)
	}
	assertEmpty(t, "stderr", stderr)

	var res app.IndexValues
	if err := json.Unmarshal([]byte(stdout), &res); err != nil {
		t.Fatalf("el stdout no es el JSON del contrato: %v\n%s", err, stdout)
	}
	if res.SchemaVersion != 1 {
		t.Errorf("schema_version = %d, se esperaba 1", res.SchemaVersion)
	}
	if len(res.Indices) != 1 || res.Indices[0].ID != "euribor-1y" || res.Indices[0].Value.String() != "3.247" {
		t.Errorf("indices = %+v, se esperaba euribor-1y a 3.247", res.Indices)
	}
	if !res.Attribution.NotEndorsed {
		t.Error("not_endorsed debe ser true (P-05)")
	}
	assertSingleJSON(t, stdout)
}

// Un --range fuera del contrato es usage/2 con la lista de valores aceptados
// y stdout vacío (decisión 4 de CMD-01): nada de recortes silenciosos.
func TestE2EIndexShowRangoInvalido(t *testing.T) {
	deps := depsCon(t, &stubSource{})
	for _, rango := range []string{"36M", "12M", "1989", "2099", "ayer"} {
		t.Run(rango, func(t *testing.T) {
			stdout, _, err := runRoot(t, deps, "index", "show", "euribor-1y", "--range", rango)
			e := assertContractError(t, err, errs.KindUsage, 2)
			assertEmpty(t, "stdout", stdout)
			if !strings.Contains(e.Message, rango) {
				t.Errorf("el mensaje debe nombrar el rango pedido, got %q", e.Message)
			}
			for _, aceptado := range []string{"30M", "60M", "MAX"} {
				if !strings.Contains(e.Hint, aceptado) {
					t.Errorf("el hint debe listar los valores aceptados (%s falta): %q", aceptado, e.Hint)
				}
			}
		})
	}
}

// `hipoteca index show` válido: el histórico llega a stdout con su rango.
func TestE2EIndexShowValido(t *testing.T) {
	sept2026 := time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC)
	agost2026 := time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC)
	series := bde.Series{
		Code: "D_1NBAF472",
		End:  sept2026,
		Observations: []bde.Observation{
			{Code: "D_1NBAF472", Month: agost2026, Value: mustDecimal(t, "2.954")},
			{Code: "D_1NBAF472", Month: sept2026, Value: mustDecimal(t, "3.247")},
		},
	}
	deps := depsCon(t, &stubSource{series: series})

	stdout, stderr, err := runRoot(t, deps, "index", "show", "euribor-1y", "--range", "30M")
	if err != nil {
		t.Fatalf("index show debe tener éxito, got %v", err)
	}
	assertEmpty(t, "stderr", stderr)
	var res app.IndexHistoryResult
	if err := json.Unmarshal([]byte(stdout), &res); err != nil {
		t.Fatalf("el stdout no es el JSON del contrato: %v\n%s", err, stdout)
	}
	if res.Range != "30M" || len(res.Observations) != 2 {
		t.Errorf("range/observaciones = %q/%d, se esperaba 30M/2", res.Range, len(res.Observations))
	}
	if res.Observations[0].Month != "2026-08" || res.Observations[1].Month != "2026-09" {
		t.Errorf("las observaciones deben llegar en orden ascendente, got %+v", res.Observations)
	}
}

// `index latest` sin ids es un error de uso.
func TestE2EIndexLatestSinIds(t *testing.T) {
	deps := depsCon(t, &stubSource{})
	stdout, _, err := runRoot(t, deps, "index", "latest")
	assertContractError(t, err, errs.KindUsage, 2)
	assertEmpty(t, "stdout", stdout)
}

// Sin red: la fuente falla con network y el árbol lo deja llegar a main para
// que el proceso salga con el código 6 del contrato (§6.4).
func TestE2EIndexLatestSinRed(t *testing.T) {
	deps := depsCon(t, &stubSource{err: errs.New(errs.KindNetwork,
		"no se pudo contactar con la API del Banco de España", "comprueba la conexión a internet")})
	stdout, _, err := runRoot(t, deps, "index", "latest", "euribor-1y")
	assertContractError(t, err, errs.KindNetwork, 6)
	assertEmpty(t, "stdout", stdout)
}

// mustDecimal construye un model.Decimal de prueba; el fallo aborta el test.
func mustDecimal(t *testing.T, s string) model.Decimal {
	t.Helper()
	d, err := model.NewDecimal(s)
	if err != nil {
		t.Fatalf("decimal de prueba %q: %v", s, err)
	}
	return d
}
