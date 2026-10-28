package bde

import (
	"strings"
	"testing"

	"github.com/aam9063/hipoteca-cli/internal/errs"
)

// TestHistoryTreintaMeses: forma completa del contrato, 31 puntos (30M incluye
// el mes en curso), orden ascendente aunque el fixture baja descendente.
func TestHistoryTreintaMeses(t *testing.T) {
	ts, c := newFixtureClient(t, fixture(t, "listaSeries-euribor-1y-30M.json"), 0)
	s, err := c.History("euribor-1y", Rango30M)
	assertNoErr(t, err)
	if s.Code != "D_1NBAF472" {
		t.Fatalf("código = %q, se esperaba D_1NBAF472", s.Code)
	}
	if len(s.Observations) != 31 {
		t.Fatalf("puntos = %d, se esperaban 31", len(s.Observations))
	}
	assertAscending(t, s.Observations)
	first, last := s.Observations[0], s.Observations[len(s.Observations)-1]
	if ym(first.Month) != "2024-03" || first.Value.String() != "3.718" {
		t.Fatalf("primero = %s %s, se esperaba 2024-03 3.718", ym(first.Month), first.Value.String())
	}
	if ym(last.Month) != "2026-09" || last.Value.String() != "3.247" {
		t.Fatalf("último = %s %s, se esperaba 2026-09 3.247", ym(last.Month), last.Value.String())
	}
	// Metadatos de publicación de la serie completa, no del rango pedido.
	if s.Frequency != "M" || s.Decimals != 3 || s.Unit != "%" {
		t.Fatalf("metadatos = %s/%d/%s, se esperaba M/3/%%", s.Frequency, s.Decimals, s.Unit)
	}
	if ym(s.Start) != "1999-01" || ym(s.End) != "2026-09" {
		t.Fatalf("fechaInicio/fechaFin = %s/%s, se esperaba 1999-01/2026-09", ym(s.Start), ym(s.End))
	}
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if ts.query.Get("idioma") != "es" || ts.query.Get("series") != "D_1NBAF472" || ts.query.Get("rango") != "30M" {
		t.Fatalf("petición inesperada: %v", ts.query)
	}
}

func TestHistoryMAX(t *testing.T) {
	_, c := newFixtureClient(t, fixture(t, "listaSeries-euribor-1y-MAX.json"), 0)
	s, err := c.History("euribor-1y", RangoMAX)
	assertNoErr(t, err)
	if len(s.Observations) != 333 {
		t.Fatalf("puntos = %d, se esperaban 333", len(s.Observations))
	}
	assertAscending(t, s.Observations)
	first, last := s.Observations[0], s.Observations[len(s.Observations)-1]
	if ym(first.Month) != "1999-01" || first.Value.String() != "3.062" {
		t.Fatalf("primero = %s %s, se esperaba 1999-01 3.062", ym(first.Month), first.Value.String())
	}
	if ym(last.Month) != "2026-09" || last.Value.String() != "3.247" {
		t.Fatalf("último = %s %s, se esperaba 2026-09 3.247", ym(last.Month), last.Value.String())
	}
	// Un valor negativo con tres decimales del fixture debe sobrevivir exacto:
	// ni float64 ni un recorte de escala pueden haberlo tocado.
	found := false
	for _, o := range s.Observations {
		if o.Value.String() == "-0.272" {
			found = true
		}
	}
	if !found {
		t.Fatal("el valor -0.272 del fixture no llegó como decimal exacto")
	}
}

func TestHistoryAnioCerrado2025(t *testing.T) {
	ts, c := newFixtureClient(t, fixture(t, "listaSeries-euribor-1y-2025.json"), 0)
	r, err := RangoAnio(2025)
	assertNoErr(t, err)
	s, err := c.History("euribor-1y", r)
	assertNoErr(t, err)
	if len(s.Observations) != 12 {
		t.Fatalf("puntos = %d, se esperaban 12", len(s.Observations))
	}
	assertAscending(t, s.Observations)
	first, last := s.Observations[0], s.Observations[len(s.Observations)-1]
	if ym(first.Month) != "2025-01" || first.Value.String() != "2.525" {
		t.Fatalf("primero = %s %s, se esperaba 2025-01 2.525", ym(first.Month), first.Value.String())
	}
	if ym(last.Month) != "2025-12" || last.Value.String() != "2.267" {
		t.Fatalf("último = %s %s, se esperaba 2025-12 2.267", ym(last.Month), last.Value.String())
	}
	if ts.query.Get("rango") != "2025" {
		t.Fatalf("rango = %q, se esperaba 2025", ts.query.Get("rango"))
	}
}

// TestHistoryAnio2026MesNoPublicado: octubre de 2026 no está en la serie y no
// hay centinela: el histórico simplemente termina en septiembre (P-06).
func TestHistoryAnio2026MesNoPublicado(t *testing.T) {
	_, c := newFixtureClient(t, fixture(t, "listaSeries-euribor-1y-2026.json"), 0)
	r, err := RangoAnio(2026)
	assertNoErr(t, err)
	s, err := c.History("euribor-1y", r)
	assertNoErr(t, err)
	if len(s.Observations) != 9 {
		t.Fatalf("puntos = %d, se esperaban 9", len(s.Observations))
	}
	last := s.Observations[len(s.Observations)-1]
	if ym(last.Month) != "2026-09" || last.Value.String() != "3.247" {
		t.Fatalf("último = %s %s, se esperaba 2026-09 3.247", ym(last.Month), last.Value.String())
	}
	for _, o := range s.Observations {
		if ym(o.Month) == "2026-10" {
			t.Fatal("octubre de 2026 no está publicado y no debe aparecer")
		}
	}
}

// TestHistoryEstr1W: la serie corta de €STR, cuyo descripcionCorta es
// ambiguo ("¤STR" sin plazo), se parsea igual y conserva 2.44 con su escala.
func TestHistoryEstr1W(t *testing.T) {
	ts, c := newFixtureClient(t, fixture(t, "listaSeries-estr-1w-30M.json"), 0)
	s, err := c.History("estr-1w", Rango30M)
	assertNoErr(t, err)
	if s.Code != "D_1NBAS572" {
		t.Fatalf("código = %q, se esperaba D_1NBAS572", s.Code)
	}
	if len(s.Observations) != 31 {
		t.Fatalf("puntos = %d, se esperaban 31", len(s.Observations))
	}
	assertAscending(t, s.Observations)
	last := s.Observations[len(s.Observations)-1]
	if ym(last.Month) != "2026-09" || last.Value.String() != "2.44" {
		t.Fatalf("último = %s %q, se esperaba 2026-09 \"2.44\" con su escala", ym(last.Month), last.Value.String())
	}
	if ts.query.Get("series") != "D_1NBAS572" {
		t.Fatalf("series = %q, se esperaba D_1NBAS572", ts.query.Get("series"))
	}
}

func TestHistoryIRPH(t *testing.T) {
	_, c := newFixtureClient(t, fixture(t, "listaSeries-irph-30M.json"), 0)
	s, err := c.History("irph", Rango30M)
	assertNoErr(t, err)
	if s.Code != "D_1T9H0000" {
		t.Fatalf("código = %q, se esperaba D_1T9H0000", s.Code)
	}
	if len(s.Observations) != 31 {
		t.Fatalf("puntos = %d, se esperaban 31", len(s.Observations))
	}
	last := s.Observations[len(s.Observations)-1]
	if ym(last.Month) != "2026-09" || last.Value.String() != "3.189" {
		t.Fatalf("último = %s %s, se esperaba 2026-09 3.189", ym(last.Month), last.Value.String())
	}
}

func assertAscending(t *testing.T, obs []Observation) {
	t.Helper()
	for i := 1; i < len(obs); i++ {
		if !obs[i].Month.After(obs[i-1].Month) {
			t.Fatalf("las observaciones no son ascendentes en %d: %s no es posterior a %s",
				i, ym(obs[i].Month), ym(obs[i-1].Month))
		}
	}
}

// TestHistorySerieInexistente: errNum 412 con "La serie XX no existe" →
// not_found/4 (docs/series.md §6).
func TestHistorySerieInexistente(t *testing.T) {
	ts, c := newFixtureClient(t, fixture(t, "error-listaSeries-serie-inexistente.json"), 0)
	_, err := c.History("euribor-1y", RangoMAX)
	assertErr(t, err, errs.KindNotFound, 4)
	if ts.hits.Load() != 1 {
		t.Fatalf("peticiones = %d, se esperaba 1", ts.hits.Load())
	}
}

// TestHistoryRangoInvalido: la API acepta por construcción los rangos que
// enviamos; un 412 de rango significa que el contrato cambió → api_changed/7.
func TestHistoryRangoInvalido(t *testing.T) {
	_, c := newFixtureClient(t, fixture(t, "error-listaSeries-rango-invalido.json"), 0)
	_, err := c.History("euribor-1y", RangoMAX)
	assertErr(t, err, errs.KindAPIChanged, 7)
}

// TestHistorySinIdioma: el cliente siempre envía idioma=es, así que este error
// solo puede significar un contrato roto → api_changed/7.
func TestHistorySinIdioma(t *testing.T) {
	_, c := newFixtureClient(t, fixture(t, "error-listaSeries-sin-idioma.json"), 0)
	_, err := c.History("euribor-1y", Rango30M)
	assertErr(t, err, errs.KindAPIChanged, 7)
}

func TestHistoryIdDesconocido(t *testing.T) {
	ts, c := newFixtureClient(t, []byte("[]"), 0)
	_, err := c.History("indice-inventado", Rango30M)
	assertErr(t, err, errs.KindNotFound, 4)
	if !strings.Contains(err.Error(), "indice-inventado") {
		t.Fatalf("el error debe nombrar el id: %s", err.Error())
	}
	if ts.hits.Load() != 0 {
		t.Fatalf("no debió haber petición; hubo %d", ts.hits.Load())
	}
}

func TestHistoryRangoVacio(t *testing.T) {
	ts, c := newFixtureClient(t, []byte("[]"), 0)
	_, err := c.History("euribor-1y", Rango{})
	assertErr(t, err, errs.KindUsage, 2)
	if ts.hits.Load() != 0 {
		t.Fatalf("no debió haber petición; hubo %d", ts.hits.Load())
	}
}

func TestRangoTipos(t *testing.T) {
	if Rango30M.String() != "30M" || Rango60M.String() != "60M" || RangoMAX.String() != "MAX" {
		t.Fatal("los rangos fijos no tienen el valor exacto de la API")
	}
}

func TestRangoAnio(t *testing.T) {
	orig := nowYear
	defer func() { nowYear = orig }()
	nowYear = func() int { return 2026 }

	r, err := RangoAnio(2025)
	assertNoErr(t, err)
	if r.String() != "2025" {
		t.Fatalf("rango = %q, se esperaba 2025", r.String())
	}
	if _, err := RangoAnio(2027); err == nil {
		t.Fatal("un año futuro no se expone: la API lo rechazaría con 412")
	} else {
		var e *errs.Error
		if !asErrs(err, &e) || e.Kind != errs.KindUsage {
			t.Fatalf("el año futuro debe ser error de uso: %v", err)
		}
	}
	if _, err := RangoAnio(1989); err == nil {
		t.Fatal("un año sin sentido no se expone")
	}
}
