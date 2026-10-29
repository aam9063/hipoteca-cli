package cli

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aam9063/hipoteca-cli/internal/app"
	"github.com/aam9063/hipoteca-cli/internal/bde"
	"github.com/aam9063/hipoteca-cli/internal/errs"
	"github.com/aam9063/hipoteca-cli/internal/model"
)

// ----------------------------------------------------------------------------
// E2E de `watch` (WATCH-01): el árbol cobra completo con fuente de datos de
// prueba, config temporal y base temporal. Se comprueba lo que ve el agente:
// código de salida, stdout con un único valor JSON y stderr vacío en éxito
// (P-01); stdout vacío en fallo.
// ----------------------------------------------------------------------------

type watchDeps struct {
	Deps
	cfgPath string
}

// watchDepsCon monta el árbol con runtime de prueba (fuente stub), config
// temporal con el sink apuntando a un httptest y base de datos temporal.
func watchDepsCon(t *testing.T, src app.Source, sinkHandler http.HandlerFunc) watchDeps {
	t.Helper()
	cat, err := bde.LoadCatalog(nil)
	if err != nil {
		t.Fatalf("cargar el catálogo: %v", err)
	}
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	dbPath := filepath.Join(dir, "hipoteca.db")

	sinkURL := "http://127.0.0.1:1/nunca"
	if sinkHandler != nil {
		srv := httptest.NewServer(sinkHandler)
		t.Cleanup(srv.Close)
		sinkURL = srv.URL
	}
	cfgToml := "schema_version = 1\n\n[sinks.webhook]\ntype = \"webhook\"\nurl = \"" + sinkURL + "\"\n"
	if err := os.WriteFile(cfgPath, []byte(cfgToml), 0o600); err != nil {
		t.Fatalf("escribir la config de prueba: %v", err)
	}
	deps := Deps{
		Runtime: func() (*app.Runtime, error) {
			return &app.Runtime{Catalog: cat, Source: src}, nil
		},
		ConfigPath: func() (string, error) { return cfgPath, nil },
		DataPath:   func() (string, error) { return dbPath, nil },
	}
	return watchDeps{Deps: deps, cfgPath: cfgPath}
}

func oct2026CLI() []bde.Observation {
	v, _ := mustDecimalOK("2.211")
	return []bde.Observation{{
		Code:  "D_1NBAF472",
		Month: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
		Value: v,
	}}
}

func mustDecimalOK(s string) (model.Decimal, error) {
	return model.NewDecimal(s)
}

// sinkOK acepta cualquier POST con 200.
func sinkOK() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }
}

// TestE2EWatchAddListRemove (WATCH-01.1): cada comando imprime un único valor
// JSON y deja stderr vacío; en fallo stdout queda vacío.
func TestE2EWatchAddListRemove(t *testing.T) {
	deps := watchDepsCon(t, &stubSource{}, sinkOK())

	out, errOut, err := runRoot(t, deps.Deps, "watch", "add", "index", "euribor-1y")
	if err != nil {
		t.Fatalf("watch add: %v", err)
	}
	assertSingleJSON(t, out)
	assertEmpty(t, "stderr de watch add", errOut)
	if !strings.Contains(out, `"target":"euribor-1y"`) || !strings.Contains(out, `"with_impact":false`) {
		t.Errorf("watch add stdout = %s", out)
	}

	out, errOut, err = runRoot(t, deps.Deps, "watch", "list")
	if err != nil {
		t.Fatalf("watch list: %v", err)
	}
	assertSingleJSON(t, out)
	assertEmpty(t, "stderr de watch list", errOut)
	if !strings.Contains(out, `"watches":[{"id":1`) {
		t.Errorf("watch list stdout = %s", out)
	}

	out, errOut, err = runRoot(t, deps.Deps, "watch", "remove", "1")
	if err != nil {
		t.Fatalf("watch remove: %v", err)
	}
	assertSingleJSON(t, out)
	assertEmpty(t, "stderr de watch remove", errOut)
	if !strings.Contains(out, `"removed":true`) {
		t.Errorf("watch remove stdout = %s", out)
	}

	// El fallo del e2e: un id inexistente deja stdout vacío.
	out, _, err = runRoot(t, deps.Deps, "watch", "remove", "99")
	if err == nil {
		t.Fatal("watch remove de un id inexistente: se esperaba error")
	}
	assertContractError(t, err, errs.KindNotFound, 4)
	assertEmpty(t, "stdout de un fallo", out)
}

// TestE2EWatchCheckUnEventoYRepetidoSinNinguno (WATCH-01.2): un mes nuevo en
// la respuesta simulada produce exactamente un evento; repetir el check no
// duplica.
func TestE2EWatchCheckUnEventoYRepetidoSinNinguno(t *testing.T) {
	deps := watchDepsCon(t, &stubSource{latest: oct2026CLI()}, sinkOK())

	if _, _, err := runRoot(t, deps.Deps, "watch", "add", "index", "euribor-1y"); err != nil {
		t.Fatalf("watch add: %v", err)
	}
	out, errOut, err := runRoot(t, deps.Deps, "watch", "check")
	if err != nil {
		t.Fatalf("watch check: %v", err)
	}
	assertSingleJSON(t, out)
	assertEmpty(t, "stderr de watch check", errOut)
	if !strings.Contains(out, `"event":"index.published"`) ||
		!strings.Contains(out, `"month":"2026-10"`) ||
		!strings.Contains(out, `"value":"2.211"`) ||
		!strings.Contains(out, `"series":"D_1NBAF472"`) ||
		!strings.Contains(out, "Banco de España") {
		t.Errorf("watch check stdout = %s", out)
	}

	out, _, err = runRoot(t, deps.Deps, "watch", "check")
	if err != nil {
		t.Fatalf("watch check repetido: %v", err)
	}
	if !strings.Contains(out, `"events":[]`) {
		t.Errorf("el check repetido debe devolver events vacío, got %s", out)
	}
}

// TestE2EWatchCheckSinDatosDelPrestamo (WATCH-01.3): con un préstamo guardado
// en la config, el check sin --with-impact no imprime ni entrega ningún dato
// del préstamo.
func TestE2EWatchCheckSinDatosDelPrestamo(t *testing.T) {
	var recibidos []string
	var mu sync.Mutex
	deps := watchDepsCon(t, &stubSource{latest: oct2026CLI()}, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		recibidos = append(recibidos, string(b))
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	})
	cfgConPrestamo := "schema_version = 1\n" +
		"\n[loans.casa]\n" +
		"name = \"Casa\"\ntype = \"variable\"\nstart_date = \"2021-06-01\"\n" +
		"original_principal = \"180000.00\"\nterm_months = 360\npayment_day = 1\n" +
		"initial_rate = \"1.25\"\nindex = \"euribor-1y\"\nspread = \"0.89\"\n" +
		"revision_every_months = 12\nreference_offset_months = 2\n" +
		"floor_rate = \"\"\nrate_rounding = \"none\"\n" +
		"interest_convention = \"monthly-nominal\"\npayment_rounding = \"half-up-cents\"\n" +
		"early_repayment_fee_pct = \"0\"\n" +
		"\n[sinks.webhook]\ntype = \"webhook\"\nurl = \"SINKURL\"\n"
	toml := strings.ReplaceAll(cfgConPrestamo, "SINKURL", sinkURLDe(t, deps))
	if err := os.WriteFile(deps.cfgPath, []byte(toml), 0o600); err != nil {
		t.Fatalf("reescribir la config: %v", err)
	}

	if _, _, err := runRoot(t, deps.Deps, "watch", "add", "index", "euribor-1y"); err != nil {
		t.Fatalf("watch add: %v", err)
	}
	out, _, err := runRoot(t, deps.Deps, "watch", "check")
	if err != nil {
		t.Fatalf("watch check: %v", err)
	}
	for _, prohibited := range []string{"casa", "180000", "principal", `"payment"`, `"impact"`} {
		if strings.Contains(out, prohibited) {
			t.Errorf("el stdout sin --with-impact contiene %q:\n%s", prohibited, out)
		}
	}
	if len(recibidos) != 1 {
		t.Fatalf("el sink recibió %d avisos, want 1", len(recibidos))
	}
	for _, prohibited := range []string{"casa", "180000", "principal", `"payment"`, `"impact"`} {
		if strings.Contains(recibidos[0], prohibited) {
			t.Errorf("el aviso entregado sin --with-impact contiene %q:\n%s", prohibited, recibidos[0])
		}
	}
}

// sinkURLDe lee la URL del sink escrita en la config de prueba.
func sinkURLDe(t *testing.T, deps watchDeps) string {
	t.Helper()
	b, err := os.ReadFile(deps.cfgPath)
	if err != nil {
		t.Fatalf("leer la config de prueba: %v", err)
	}
	const marker = "url = \""
	i := strings.Index(string(b), marker)
	if i < 0 {
		t.Fatal("la config de prueba no tiene sink")
	}
	rest := string(b[i+len(marker):])
	return rest[:strings.Index(rest, "\"")]
}

// TestE2EWatchSinkCaídoDevuelveCódigo5YConservaElEvento (WATCH-01.5): el
// fallo del sink es el código 5 con stdout vacío; el evento sigue en la base y
// el siguiente check con el sink sano lo reintenta.
func TestE2EWatchSinkCaídoDevuelveCódigo5YConservaElEvento(t *testing.T) {
	deps := watchDepsCon(t, &stubSource{latest: oct2026CLI()}, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	if _, _, err := runRoot(t, deps.Deps, "watch", "add", "index", "euribor-1y"); err != nil {
		t.Fatalf("watch add: %v", err)
	}

	out, _, err := runRoot(t, deps.Deps, "watch", "check")
	if err == nil {
		t.Fatal("watch check con el sink caído: se esperaba error")
	}
	assertContractError(t, err, errs.KindUpstreamUnavailable, 5)
	assertEmpty(t, "stdout de un fallo", out)

	// El evento sigue grabado: un sink sano lo reintenta en la siguiente pasada.
	deps2 := watchDepsCon(t, &stubSource{latest: oct2026CLI()}, sinkOK())
	deps2.DataPath = deps.DataPath
	out, _, err = runRoot(t, deps2.Deps, "watch", "check")
	if err != nil {
		t.Fatalf("watch check con el sink sano: %v", err)
	}
	if !strings.Contains(out, `"event":"index.published"`) {
		t.Errorf("el reintento debía entregar el evento grabado, got %s", out)
	}
}

// TestE2EWatchAddConWithImpactAvisaP03 (WATCH-01.4): activar --with-impact
// imprime la advertencia de que un dato del préstamo sale de la máquina.
func TestE2EWatchAddConWithImpactAvisaP03(t *testing.T) {
	deps := watchDepsCon(t, &stubSource{}, sinkOK())

	_, errOut, err := runRoot(t, deps.Deps, "watch", "add", "index", "euribor-1y", "--with-impact")
	if err != nil {
		t.Fatalf("watch add --with-impact: %v", err)
	}
	if !strings.Contains(errOut, "préstamo") || !strings.Contains(errOut, "P-03") {
		t.Errorf("stderr = %q, want la advertencia P-03 sobre el dato del préstamo", errOut)
	}
	// Y sin la bandera no hay advertencia.
	_, errOut, err = runRoot(t, deps.Deps, "watch", "add", "index", "irph")
	if err != nil {
		t.Fatalf("watch add: %v", err)
	}
	assertEmpty(t, "stderr de watch add sin --with-impact", errOut)
}

// TestE2EWatchAddIndiceInexistente: un id que no está en el catálogo es
// not_found/4 y stdout queda vacío.
func TestE2EWatchAddIndiceInexistente(t *testing.T) {
	deps := watchDepsCon(t, &stubSource{}, sinkOK())
	_, _, err := runRoot(t, deps.Deps, "watch", "add", "index", "no-existe")
	if err == nil {
		t.Fatal("se esperaba not_found")
	}
	assertContractError(t, err, errs.KindNotFound, 4)
}

// TestE2EWatchRunRechazaIntervaloMenorDeUnaHora (WATCH-01, arranque §5.3): el
// intervalo mínimo es 1 h; un valor menor es usage/2 y stdout queda vacío. La
// validación aquí es la que evita también cualquier busy-loop con intervalos
// diminutos.
func TestE2EWatchRunRechazaIntervaloMenorDeUnaHora(t *testing.T) {
	deps := watchDepsCon(t, &stubSource{}, sinkOK())
	for _, intervalo := range []string{"500ms", "59m", "0s"} {
		_, _, err := runRoot(t, deps.Deps, "watch", "run", "--interval", intervalo)
		if err == nil {
			t.Fatalf("watch run --interval %s: se esperaba error de uso", intervalo)
		}
		assertContractError(t, err, errs.KindUsage, 2)
	}
}

// TestE2EWatchSinSubcomando: sin subcomando no hay resultado (P-01).
func TestE2EWatchSinSubcomando(t *testing.T) {
	deps := watchDepsCon(t, &stubSource{}, sinkOK())
	_, _, err := runRoot(t, deps.Deps, "watch")
	if err == nil {
		t.Fatal("se esperaba un error de uso")
	}
	assertContractError(t, err, errs.KindUsage, 2)
}
