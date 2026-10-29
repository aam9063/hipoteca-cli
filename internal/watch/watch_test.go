package watch

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/aam9063/hipoteca-cli/internal/bde"
	"github.com/aam9063/hipoteca-cli/internal/errs"
	"github.com/aam9063/hipoteca-cli/internal/model"
	"github.com/aam9063/hipoteca-cli/internal/store"
)

// ----------------------------------------------------------------------------
// Fuente y sink de prueba: sin red (regla dura del arranque). El sink graba
// las entregas y puede fingir un fallo.
// ----------------------------------------------------------------------------

type stubSource struct {
	obs map[string]bde.Observation // por código de serie
	err error
}

func (s *stubSource) Latest(_ context.Context, _ []string) ([]bde.Observation, error) {
	if s.err != nil {
		return nil, s.err
	}
	out := make([]bde.Observation, 0, len(s.obs))
	for _, o := range s.obs {
		out = append(out, o)
	}
	return out, nil
}

type sinkRecord struct {
	Payload []byte
}

type stubSink struct {
	calls []sinkRecord
	fail  bool
}

func (s *stubSink) Deliver(_ context.Context, payload any) error {
	if s.fail {
		return errs.New(errs.KindUpstreamUnavailable,
			"el sink de prueba falla siempre", "")
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	s.calls = append(s.calls, sinkRecord{Payload: b})
	return nil
}

type fixture struct {
	db   *store.DB
	deps Deps
	sink *stubSink
	now  time.Time
}

func newFixture(t *testing.T, src Source) *fixture {
	t.Helper()
	db, err := store.Open(t.TempDir() + "/hipoteca.db")
	if err != nil {
		t.Fatalf("abrir la base de prueba: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	cat, err := bde.LoadCatalog(nil)
	if err != nil {
		t.Fatalf("cargar el catálogo: %v", err)
	}
	sink := &stubSink{}
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	return &fixture{
		db: db,
		deps: Deps{
			DB:      db,
			Catalog: cat,
			Source:  src,
			SinkFor: func(string) (Sink, error) { return sink, nil },
			Now:     func() time.Time { return now },
		},
		sink: sink,
		now:  now,
	}
}

func (f *fixture) addWatch(t *testing.T, target string, withImpact bool) store.Watch {
	t.Helper()
	w, err := f.db.AddWatch(KindIndex, target, "webhook", withImpact, f.now)
	if err != nil {
		t.Fatalf("AddWatch: %v", err)
	}
	return w
}

func oct2026() bde.Observation {
	v, _ := model.NewDecimal("2.211")
	return bde.Observation{Code: "D_1NBAF472", Month: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), Value: v}
}

// TestCheckMesNuevoProduceExactamenteUnEvento (WATCH-01.2): con un mes que la
// base no ha visto, check produce un evento con índice, mes, valor, serie y
// atribución, y lo graba.
func TestCheckMesNuevoProduceExactamenteUnEvento(t *testing.T) {
	f := newFixture(t, &stubSource{obs: map[string]bde.Observation{"D_1NBAF472": oct2026()}})
	w := f.addWatch(t, "euribor-1y", false)

	events, err := Check(context.Background(), f.deps)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	e := events[0]
	if e.Event != EventIndexPublished {
		t.Errorf("event = %q, want %q", e.Event, EventIndexPublished)
	}
	if e.Index.ID != "euribor-1y" || e.Index.Series != "D_1NBAF472" || e.Index.Name == "" {
		t.Errorf("index = %+v, want id euribor-1y, serie D_1NBAF472 y nombre del catálogo", e.Index)
	}
	if e.Month != "2026-10" {
		t.Errorf("month = %q, want 2026-10", e.Month)
	}
	if e.Value.String() != "2.211" {
		t.Errorf("value = %q, want 2.211 (decimal exacto, P-07)", e.Value.String())
	}
	if e.Attribution.Text != "Fuente: Sitio web del Banco de España (www.bde.es)" ||
		e.Attribution.DataUpdatedAt != "2026-10-01" || !e.Attribution.NotEndorsed {
		t.Errorf("attribution = %+v, want la del Banco con fecha 2026-10-01", e.Attribution)
	}
	if len(f.sink.calls) != 1 {
		t.Fatalf("el sink recibió %d entregas, want 1", len(f.sink.calls))
	}

	// El evento quedó grabado como entregado.
	seen, err := f.db.SeenMonths(w.ID)
	if err != nil {
		t.Fatalf("SeenMonths: %v", err)
	}
	if len(seen) != 1 || seen[0].DeliveredAt == nil {
		t.Errorf("estado grabado = %+v, want un mes entregado", seen)
	}
}

// TestCheckRepetidoNoDuplica (decisión 2): repetir el check con los mismos
// datos del servidor produce cero eventos y ninguna entrega extra.
func TestCheckRepetidoNoDuplica(t *testing.T) {
	f := newFixture(t, &stubSource{obs: map[string]bde.Observation{"D_1NBAF472": oct2026()}})
	f.addWatch(t, "euribor-1y", false)

	if _, err := Check(context.Background(), f.deps); err != nil {
		t.Fatalf("primer Check: %v", err)
	}
	events, err := Check(context.Background(), f.deps)
	if err != nil {
		t.Fatalf("segundo Check: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("segundo Check devolvió %d eventos, want 0", len(events))
	}
	if len(f.sink.calls) != 1 {
		t.Errorf("el sink recibió %d entregas en total, want 1", len(f.sink.calls))
	}
}

// TestCheckSinVigilancias: sin vigilancias no hay error y events sale vacío
// (no nil, para que el JSON sea [] y no null).
func TestCheckSinVigilancias(t *testing.T) {
	f := newFixture(t, &stubSource{})
	events, err := Check(context.Background(), f.deps)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if events == nil || len(events) != 0 {
		t.Errorf("events = %#v, want slice vacío no nulo", events)
	}
}

// TestSinkFailureConservaElEvento (decisión 5, WATCH-01.5): un fallo del sink
// es error, pero el evento ya grabado no se borra; la siguiente pasada con el
// sink sano lo reintenta y entrega exactamente una vez.
func TestSinkFailureConservaElEvento(t *testing.T) {
	f := newFixture(t, &stubSource{obs: map[string]bde.Observation{"D_1NBAF472": oct2026()}})
	w := f.addWatch(t, "euribor-1y", false)
	f.sink.fail = true

	if _, err := Check(context.Background(), f.deps); err == nil {
		t.Fatal("Check con el sink caído: se esperaba error")
	}
	seen, err := f.db.SeenMonths(w.ID)
	if err != nil {
		t.Fatalf("SeenMonths: %v", err)
	}
	if len(seen) != 1 {
		t.Fatalf("tras el fallo del sink, seen_months = %d filas, want 1 (el evento no se borra)", len(seen))
	}
	if seen[0].DeliveredAt != nil {
		t.Errorf("delivered_at = %v, want nil (no se entregó)", seen[0].DeliveredAt)
	}

	// La siguiente pasada, con el sink sano, reintenta y entrega una vez.
	f.sink.fail = false
	events, err := Check(context.Background(), f.deps)
	if err != nil {
		t.Fatalf("segundo Check: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("el reintento devolvió %d eventos, want 1", len(events))
	}
	if len(f.sink.calls) != 1 {
		t.Fatalf("el sink recibió %d entregas, want 1", len(f.sink.calls))
	}
}

// TestNotificationSinWithImpactNoContieneDatoDelPrestamo (WATCH-01.3,
// decisión 3): el payload de un aviso sin --with-impact, serializado, no
// contiene ni el capital, ni la cuota, ni el id del préstamo, ni bloque de
// impacto.
func TestNotificationSinWithImpactNoContieneDatoDelPrestamo(t *testing.T) {
	f := newFixture(t, &stubSource{obs: map[string]bde.Observation{"D_1NBAF472": oct2026()}})
	f.addWatch(t, "euribor-1y", false)

	events, err := Check(context.Background(), f.deps)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	b, err := json.Marshal(events[0])
	if err != nil {
		t.Fatalf("marshal del payload: %v", err)
	}
	payload := string(b)
	for _, prohibited := range []string{"principal", "payment", `"impact"`, "loan", "casa", "180000", "742.18"} {
		if strings.Contains(payload, prohibited) {
			t.Errorf("el payload sin --with-impact contiene %q:\n%s", prohibited, payload)
		}
	}
	if !strings.Contains(payload, `"attribution"`) {
		t.Errorf("el payload debe llevar la atribución:\n%s", payload)
	}
}

// TestNotificationConWithImpactMarcado (WATCH-01.4, decisión 3): el aviso
// lleva el cambio estimado de cuota, marcado como estimación, y la advertencia
// de que un dato del préstamo sale de la máquina (P-03).
func TestNotificationConWithImpactMarcado(t *testing.T) {
	f := newFixture(t, &stubSource{obs: map[string]bde.Observation{"D_1NBAF472": oct2026()}})
	f.addWatch(t, "euribor-1y", true)
	f.deps.ImpactFor = func(target string, _ model.Decimal) (Impact, []string, error) {
		prev, _ := model.NewDecimal("742.18")
		next, _ := model.NewDecimal("740.31")
		diff, _ := model.NewDecimal("-1.87")
		return Impact{
			Estimated: true,
			Note:      "estimación",
			Loans:     []LoanImpact{{Loan: "casa", Payment: prev, NewPayment: next, DifferenceMonth: diff}},
		}, nil, nil
	}

	events, err := Check(context.Background(), f.deps)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	e := events[0]
	if e.Impact == nil || !e.Impact.Estimated {
		t.Fatalf("impact = %+v, want bloque presente y marcado como estimación", e.Impact)
	}
	if len(e.Impact.Loans) != 1 || e.Impact.Loans[0].Loan != "casa" {
		t.Errorf("impact.loans = %+v, want la cuota estimada del préstamo casa", e.Impact.Loans)
	}
	b, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	payload := string(b)
	if !strings.Contains(payload, "préstamo") || !strings.Contains(payload, "--with-impact") {
		t.Errorf("el payload debe llevar la advertencia P-03 marcando el dato del préstamo:\n%s", payload)
	}
}
