package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aam9063/hipoteca-cli/internal/bde"
	"github.com/aam9063/hipoteca-cli/internal/config"
	"github.com/aam9063/hipoteca-cli/internal/errs"
	"github.com/aam9063/hipoteca-cli/internal/loan"
	"github.com/aam9063/hipoteca-cli/internal/model"
	"github.com/aam9063/hipoteca-cli/internal/watch"
)

// ----------------------------------------------------------------------------
// `watch` en la capa de orquestación (WATCH-01): alta/lista/borrado sobre la
// base local, una pasada de check y el bucle run. Sin red: la fuente es un
// stub y el sink va a una URL httptest o no se llega a usar.
// ----------------------------------------------------------------------------

// watchStubSource devuelve lo que el test fija, sin tocar la red.
type watchStubSource struct {
	obs []bde.Observation
	err error
}

func (s *watchStubSource) Latest(context.Context, []string) ([]bde.Observation, error) {
	return s.obs, s.err
}

type watchFixture struct {
	dbPath    string
	cfgPath   string
	cfg       config.Config
	catalog   *bde.Catalog
	source    *watchStubSource
	now       time.Time
	delivered *[][]byte // cuerpos recibidos por el sink httptest
}

func newWatchFixture(t *testing.T) *watchFixture {
	t.Helper()
	cat, err := bde.LoadCatalog(nil)
	if err != nil {
		t.Fatalf("cargar el catálogo: %v", err)
	}
	delivered := &[][]byte{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		*delivered = append(*delivered, b)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	cfg := config.Default()
	cfg.Sinks["webhook"] = config.Sink{Type: "webhook", URL: srv.URL}
	f := &watchFixture{
		dbPath:    t.TempDir() + "/hipoteca.db",
		cfgPath:   t.TempDir() + "/config.toml",
		cfg:       cfg,
		catalog:   cat,
		source:    &watchStubSource{},
		now:       time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC),
		delivered: delivered,
	}
	return f
}

func (f *watchFixture) saveConfig(t *testing.T) {
	t.Helper()
	if err := f.cfg.Save(f.cfgPath); err != nil {
		t.Fatalf("guardar la config de prueba: %v", err)
	}
}

func octubre(t *testing.T) bde.Observation {
	t.Helper()
	v, err := model.NewDecimal("2.211")
	if err != nil {
		t.Fatalf("decimal: %v", err)
	}
	return bde.Observation{Code: "D_1NBAF472", Month: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), Value: v}
}

// prestamoCasa construye un préstamo completo que usa euribor-1y.
func prestamoCasa() loan.Loan {
	tramo := 12
	offset := 2
	sinSuelo := ""
	return loan.Loan{
		Name:                  "Casa",
		Type:                  loan.TypeVariable,
		StartDate:             "2021-06-01",
		OriginalPrincipal:     "180000.00",
		TermMonths:            360,
		PaymentDay:            1,
		InitialRate:           "1.25",
		Index:                 "euribor-1y",
		Spread:                "0.89",
		RevisionEveryMonths:   12,
		ReferenceOffsetMonths: &offset,
		InitialPeriodMonths:   &tramo,
		FloorRate:             &sinSuelo,
		RateRounding:          "none",
		InterestConvention:    loan.InterestConventionMonthlyNominal,
		PaymentRounding:       loan.PaymentRoundingHalfUpCents,
		EarlyRepaymentFeePct:  "0",
	}
}

// TestWatchAddListRemoveRoundTrip (WATCH-01.1): alta, lista y borrado con una
// salida JSON cada uno.
func TestWatchAddListRemoveRoundTrip(t *testing.T) {
	f := newWatchFixture(t)
	f.saveConfig(t)

	added, err := WatchAdd(f.dbPath, f.cfgPath, f.catalog, watch.KindIndex, "euribor-1y", "webhook", false, f.now)
	if err != nil {
		t.Fatalf("WatchAdd: %v", err)
	}
	if added.Watch.ID == 0 || added.Watch.Target != "euribor-1y" || added.SchemaVersion != model.SchemaVersion {
		t.Errorf("WatchAdd = %+v", added)
	}

	list, err := WatchList(f.dbPath)
	if err != nil {
		t.Fatalf("WatchList: %v", err)
	}
	if len(list.Watches) != 1 || list.Watches[0].ID != added.Watch.ID {
		t.Errorf("WatchList = %+v, want la vigilancia recién creada", list)
	}

	removed, err := WatchRemove(f.dbPath, added.Watch.ID)
	if err != nil {
		t.Fatalf("WatchRemove: %v", err)
	}
	if !removed.Removed || removed.Watch != added.Watch.ID {
		t.Errorf("WatchRemove = %+v, want removed=true del id %d", removed, added.Watch.ID)
	}
	list, err = WatchList(f.dbPath)
	if err != nil {
		t.Fatalf("WatchList: %v", err)
	}
	if len(list.Watches) != 0 {
		t.Errorf("tras remove, WatchList = %d vigilancias, want 0", len(list.Watches))
	}
}

// TestWatchAddValidaIndiceYSink: un índice que no está en el catálogo es
// not_found/4; un sink sin sección en la config es usage/2; un sink de un
// tipo que no es webhook es usage/2 (v1).
func TestWatchAddValidaIndiceYSink(t *testing.T) {
	f := newWatchFixture(t)
	f.saveConfig(t)

	if _, err := WatchAdd(f.dbPath, f.cfgPath, f.catalog, watch.KindIndex, "no-existe", "webhook", false, f.now); err == nil {
		t.Error("WatchAdd con un índice inexistente: se esperaba error")
	} else {
		assertCodeApp(t, err, 4)
	}
	if _, err := WatchAdd(f.dbPath, f.cfgPath, f.catalog, watch.KindIndex, "euribor-1y", "fantasma", false, f.now); err == nil {
		t.Error("WatchAdd con un sink sin sección en config: se esperaba error")
	} else {
		assertCodeApp(t, err, 2)
	}
	f.cfg.Sinks["push"] = config.Sink{Type: "telefono", URL: "x"}
	f.saveConfig(t)
	if _, err := WatchAdd(f.dbPath, f.cfgPath, f.catalog, watch.KindIndex, "euribor-1y", "push", false, f.now); err == nil {
		t.Error("WatchAdd con un sink que no es webhook: se esperaba error (v1)")
	} else {
		assertCodeApp(t, err, 2)
	}
	if _, err := WatchAdd(f.dbPath, f.cfgPath, f.catalog, "bolsa", "euribor-1y", "webhook", false, f.now); err == nil {
		t.Error("WatchAdd con un kind que no es index: se esperaba error")
	} else {
		assertCodeApp(t, err, 2)
	}
}

// TestWatchCheckDetectaElMesNuevo (WATCH-01.2): una pasada produce un evento;
// la repetida, ninguno; y el resultado lleva schema_version y events no nulo.
func TestWatchCheckDetectaElMesNuevo(t *testing.T) {
	f := newWatchFixture(t)
	f.saveConfig(t)
	if _, err := WatchAdd(f.dbPath, f.cfgPath, f.catalog, watch.KindIndex, "euribor-1y", "webhook", false, f.now); err != nil {
		t.Fatalf("WatchAdd: %v", err)
	}
	f.source.obs = []bde.Observation{octubre(t)}

	res, err := WatchCheck(context.Background(), f.dbPath, f.cfgPath, f.source, f.catalog, f.now)
	if err != nil {
		t.Fatalf("WatchCheck: %v", err)
	}
	if res.SchemaVersion != model.SchemaVersion || len(res.Events) != 1 {
		t.Fatalf("WatchCheck = %+v, want schema_version %d y 1 evento", res, model.SchemaVersion)
	}
	if res.Events[0].Index.Series != "D_1NBAF472" || res.Events[0].Month != "2026-10" {
		t.Errorf("evento = %+v", res.Events[0])
	}

	res, err = WatchCheck(context.Background(), f.dbPath, f.cfgPath, f.source, f.catalog, f.now)
	if err != nil {
		t.Fatalf("WatchCheck repetido: %v", err)
	}
	if res.Events == nil || len(res.Events) != 0 {
		t.Errorf("WatchCheck repetido = %d eventos, want 0 y slice no nulo", len(res.Events))
	}
}

// TestWatchCheckSinImpactoNoSacaDatosDelPrestamo (WATCH-01.3, decisión 3 y
// P-03): serializada, la salida de check sin --with-impact no contiene ni el
// id del préstamo, ni el capital, ni la cuota.
func TestWatchCheckSinImpactoNoSacaDatosDelPrestamo(t *testing.T) {
	f := newWatchFixture(t)
	f.cfg.Loans["casa"] = prestamoCasa()
	f.saveConfig(t)
	if _, err := WatchAdd(f.dbPath, f.cfgPath, f.catalog, watch.KindIndex, "euribor-1y", "webhook", false, f.now); err != nil {
		t.Fatalf("WatchAdd: %v", err)
	}
	f.source.obs = []bde.Observation{octubre(t)}

	res, err := WatchCheck(context.Background(), f.dbPath, f.cfgPath, f.source, f.catalog, f.now)
	if err != nil {
		t.Fatalf("WatchCheck: %v", err)
	}
	b, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	payload := string(b)
	for _, prohibited := range []string{"casa", "180000", "principal", `"payment"`, `"impact"`} {
		if strings.Contains(payload, prohibited) {
			t.Errorf("la salida sin --with-impact contiene %q (dato del préstamo):\n%s", prohibited, payload)
		}
	}
}

// TestWatchCheckConImpactoIncluyeCuotaEstimada (WATCH-01.4, decisión 3): el
// aviso lleva el cambio estimado de cuota del préstamo que usa el índice,
// marcado como estimación y con la advertencia P-03.
func TestWatchCheckConImpactoIncluyeCuotaEstimada(t *testing.T) {
	f := newWatchFixture(t)
	f.cfg.Loans["casa"] = prestamoCasa()
	f.saveConfig(t)
	if _, err := WatchAdd(f.dbPath, f.cfgPath, f.catalog, watch.KindIndex, "euribor-1y", "webhook", true, f.now); err != nil {
		t.Fatalf("WatchAdd: %v", err)
	}
	f.source.obs = []bde.Observation{octubre(t)}

	res, err := WatchCheck(context.Background(), f.dbPath, f.cfgPath, f.source, f.catalog, f.now)
	if err != nil {
		t.Fatalf("WatchCheck: %v", err)
	}
	if len(res.Events) != 1 {
		t.Fatalf("events = %d, want 1", len(res.Events))
	}
	e := res.Events[0]
	if e.Impact == nil || !e.Impact.Estimated {
		t.Fatalf("impact = %+v, want bloque presente y marcado estimated", e.Impact)
	}
	if len(e.Impact.Loans) != 1 || e.Impact.Loans[0].Loan != "casa" {
		t.Fatalf("impact.loans = %+v, want la estimación del préstamo casa", e.Impact.Loans)
	}
	li := e.Impact.Loans[0]
	if li.Payment.String() == "0" || li.NewPayment.String() == "0" || li.DifferenceMonth.String() == "0" {
		t.Errorf("la estimación viene vacía: %+v", li)
	}
	predo := e.Impact.Loans[0].Payment.Decimal()
	nuevo := e.Impact.Loans[0].NewPayment.Decimal()
	if !nuevo.Sub(predo).Equal(e.Impact.Loans[0].DifferenceMonth.Decimal()) {
		t.Errorf("difference_month = %s, want %s", e.Impact.Loans[0].DifferenceMonth, nuevo.Sub(predo))
	}
	b, _ := json.Marshal(e)
	if !strings.Contains(string(b), "préstamo") || !strings.Contains(string(b), "--with-impact") {
		t.Errorf("el aviso debe marcarse con la advertencia P-03:\n%s", b)
	}
}

// TestWatchCheckConPrestamoIncompletoNoInventa (P-06): un préstamo que usa el
// índice pero está incompleto no produce números inventados: el aviso no lleva
// su impacto y el warning lo dice.
func TestWatchCheckConPrestamoIncompletoNoInventa(t *testing.T) {
	f := newWatchFixture(t)
	l := prestamoCasa()
	l.FloorRate = nil // falta un dato de la escritura
	f.cfg.Loans["casa"] = l
	f.saveConfig(t)
	if _, err := WatchAdd(f.dbPath, f.cfgPath, f.catalog, watch.KindIndex, "euribor-1y", "webhook", true, f.now); err != nil {
		t.Fatalf("WatchAdd: %v", err)
	}
	f.source.obs = []bde.Observation{octubre(t)}

	res, err := WatchCheck(context.Background(), f.dbPath, f.cfgPath, f.source, f.catalog, f.now)
	if err != nil {
		t.Fatalf("WatchCheck: %v", err)
	}
	if len(res.Events) != 1 {
		t.Fatalf("events = %d, want 1 (el aviso sigue saliendo)", len(res.Events))
	}
	if res.Events[0].Impact != nil {
		t.Errorf("impact = %+v, want nil: sin dato de la escritura no hay estimación (P-06)", res.Events[0].Impact)
	}
	hayAviso := false
	for _, w := range res.Events[0].Warnings {
		if strings.Contains(w, "incompleto") {
			hayAviso = true
		}
	}
	if !hayAviso {
		t.Errorf("warnings = %v, want uno que nombre el préstamo incompleto", res.Events[0].Warnings)
	}
}

// TestWatchRunEjecutaPasadasYTerminaConElContexto: el bucle hace su pasada y
// espera el intervalo entre pasadas; la cancelación del contexto termina el
// run sin error. El sueño inyectado evita tanto el busy-loop como la espera
// real de 1 h en los tests.
func TestWatchRunEjecutaPasadasYTerminaConElContexto(t *testing.T) {
	f := newWatchFixture(t)
	f.saveConfig(t)
	if _, err := WatchAdd(f.dbPath, f.cfgPath, f.catalog, watch.KindIndex, "euribor-1y", "webhook", false, f.now); err != nil {
		t.Fatalf("WatchAdd: %v", err)
	}
	f.source.obs = []bde.Observation{octubre(t)}

	checks := 0
	pasadas := 0
	err := WatchRun(context.Background(), RunParams{
		DBPath:   f.dbPath,
		CfgPath:  f.cfgPath,
		Source:   f.source,
		Catalog:  f.catalog,
		Interval: time.Hour,
		Now:      func() time.Time { return f.now },
		Sleep: func(ctx context.Context, _ time.Duration) error {
			pasadas++
			if pasadas >= 2 {
				return context.Canceled // segunda espera: fin del run
			}
			return nil
		},
		Report: func(error) { checks++ },
	})
	if err != nil {
		t.Fatalf("WatchRun: %v", err)
	}
	if pasadas < 2 {
		t.Errorf("el run durmió %d veces, want al menos 2 (una pasada y una espera)", pasadas)
	}
}

// TestWatchRunReportaElFalloSinMorir: un fallo del sink en una pasada no mata
// el run si hay Report: lo notifica y sigue con la siguiente (decisión 5: el
// evento queda grabado para el reintento).
func TestWatchRunReportaElFalloSinMorir(t *testing.T) {
	f := newWatchFixture(t)
	f.saveConfig(t)
	if _, err := WatchAdd(f.dbPath, f.cfgPath, f.catalog, watch.KindIndex, "euribor-1y", "webhook", false, f.now); err != nil {
		t.Fatalf("WatchAdd: %v", err)
	}
	// Fuente caída: cada pasada falla antes de tocar el sink.
	f.source.err = errs.New(errs.KindNetwork, "sin red", "")

	informes := 0
	pasadas := 0
	err := WatchRun(context.Background(), RunParams{
		DBPath:   f.dbPath,
		CfgPath:  f.cfgPath,
		Source:   f.source,
		Catalog:  f.catalog,
		Interval: time.Hour,
		Now:      func() time.Time { return f.now },
		Sleep: func(context.Context, time.Duration) error {
			pasadas++
			if pasadas >= 2 {
				return context.Canceled
			}
			return nil
		},
		Report: func(error) { informes++ },
	})
	if err != nil {
		t.Fatalf("WatchRun con fallos en las pasadas: %v", err)
	}
	if informes < 1 {
		t.Errorf("Report se llamó %d veces, want al menos 1", informes)
	}
	if !errors.Is(err, context.Canceled) && err != nil {
		t.Errorf("err = %v, want nil", err)
	}
}

func assertCodeApp(t *testing.T, err error, code int) {
	t.Helper()
	env, ok := errs.From(err)
	if !ok {
		t.Fatalf("el error debe llevar el sobre del contrato, got %v", err)
	}
	if env.Error.ExitCode != code {
		t.Fatalf("código = %d, want %d (mensaje: %s)", env.Error.ExitCode, code, env.Error.Message)
	}
}
