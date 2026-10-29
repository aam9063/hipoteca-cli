package app

import (
	"testing"
	"time"

	"github.com/aam9063/hipoteca-cli/internal/bde"
	"github.com/aam9063/hipoteca-cli/internal/errs"
	"github.com/aam9063/hipoteca-cli/internal/finance"
	"github.com/aam9063/hipoteca-cli/internal/loan"
	"github.com/aam9063/hipoteca-cli/internal/model"
	"github.com/shopspring/decimal"
)

// ----------------------------------------------------------------------------
// `revision` real sobre los fixtures de testdata/bde servidos por el
// fixtureServer (LOAN-01.5). La serie euribor-1y del fixture MAX termina en
// 2026-09 con valor 3.247; el mes de referencia de una revisión de 2026-11
// con offset 2 es justo 2026-09.
// ----------------------------------------------------------------------------

// prestamoRevision: préstamo variable del ejemplo del arranque con un recibo
// sincronizado en 2026-10 (capital 152341.18, tipo 3.12).
func prestamoRevision() loan.Loan {
	l := prestamoCompleto()
	l.Sync = &loan.Sync{Date: "2026-10-01", OutstandingPrincipal: "152341.18", CurrentRate: "3.12"}
	return l
}

// runtimeRevision monta el runtime de prueba con el fixtureServer.
func runtimeRevision(t *testing.T) *Runtime {
	src := newFakeSource(t)
	cat, err := bde.LoadCatalog(nil)
	if err != nil {
		t.Fatalf("cargar el catálogo: %v", err)
	}
	return &Runtime{Catalog: cat, Source: src}
}

// ahoraFijo es el "hoy" de las pruebas: 2026-10-15.
func ahoraFijo() time.Time {
	return time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
}

// TestRevisionPublicada: la revisión de 2026-11 usa el Euríbor de 2026-09
// (3.247, offset 2), recalcula la cuota con el capital pendiente y los meses
// restantes, y expone la cadena del tipo, las entradas y la atribución
// (P-04, P-05; arranque §5.2).
func TestRevisionPublicada(t *testing.T) {
	path := configCon(t, map[string]loan.Loan{"casa": prestamoRevision()})
	rt := runtimeRevision(t)

	res, err := Revision(t.Context(), rt, RevisionParams{
		ConfigPath: path,
		LoanID:     "casa",
		Date:       "2026-11",
		Now:        ahoraFijo(),
	})
	if err != nil {
		t.Fatalf("Revision(): %v", err)
	}
	if res.SchemaVersion != model.SchemaVersion {
		t.Fatalf("schema_version = %d", res.SchemaVersion)
	}
	if res.Loan != "casa" {
		t.Fatalf("loan = %q", res.Loan)
	}
	if res.RevisionDate != "2026-11-01" {
		t.Fatalf("revision_date = %q, want 2026-11-01 (día de pago 1)", res.RevisionDate)
	}

	// Bloque índice: el mes de referencia sale de la fecha de revisión menos
	// el offset de la escritura; el valor es el publicado por el Banco.
	if res.Index.ID != "euribor-1y" || res.Index.ReferenceMonth != "2026-09" {
		t.Fatalf("index = %s/%s, want euribor-1y/2026-09", res.Index.ID, res.Index.ReferenceMonth)
	}
	if res.Index.Status != "published" || res.Index.Value.String() != "3.247" {
		t.Fatalf("index status/valor = %s/%s, want published/3.247", res.Index.Status, res.Index.Value)
	}
	if res.Index.Series != "D_1NBAF472" {
		t.Fatalf("series = %q, want el código del catálogo", res.Index.Series)
	}
	if res.Index.PublishedAt != "2026-09-01" {
		t.Fatalf("published_at = %q, want 2026-09-01", res.Index.PublishedAt)
	}

	// Cadena del tipo: 3.247 + 0.89 = 4.137, sin suelo ni redondeo.
	if res.Rate.Previous.String() != "3.12" || res.Rate.Spread.String() != "0.89" ||
		res.Rate.Raw.String() != "4.137" || res.Rate.AfterFloor.String() != "4.137" ||
		res.Rate.AfterRounding.String() != "4.137" || res.Rate.New.String() != "4.137" {
		t.Fatalf("cadena del tipo = %+v, want 3.12 + 0.89 = 4.137", res.Rate)
	}

	// Entradas: el capital y la cuota previa vienen del recibo + cuadro
	// (decisión 2); los meses restantes son 360 - 65 = 295.
	if res.Inputs.OutstandingSource != "sync+schedule" {
		t.Fatalf("outstanding_source = %q, want sync+schedule", res.Inputs.OutstandingSource)
	}
	if res.Inputs.RemainingMonths != 295 {
		t.Fatalf("remaining_months = %d, want 295", res.Inputs.RemainingMonths)
	}
	if res.Inputs.InterestConvention != loan.InterestConventionMonthlyNominal {
		t.Fatalf("interest_convention = %q", res.Inputs.InterestConvention)
	}

	// El capital para la cuota nueva es el del recibo (ya es el pendiente
	// tras la cuota del mes del recibo, decisión 2), y la cuota previa es la
	// primera del cuadro que parte del recibo.
	wantBalance := decimal.RequireFromString("152341.18")
	if !res.Inputs.OutstandingPrincipal.Decimal().Equal(wantBalance) {
		t.Fatalf("outstanding = %s, want %s (el capital del recibo)", res.Inputs.OutstandingPrincipal, wantBalance)
	}
	viejo, err := finance.Amortization(finance.Loan{
		Principal:  wantBalance,
		AnnualRate: decimal.RequireFromString("3.12"),
		Months:     296,
	})
	if err != nil {
		t.Fatalf("cuadro del recibo: %v", err)
	}
	nuevo, err := finance.Amortization(finance.Loan{
		Principal:  wantBalance,
		AnnualRate: decimal.RequireFromString("4.137"),
		Months:     295,
	})
	if err != nil {
		t.Fatalf("cuadro nuevo: %v", err)
	}
	if !res.Payment.New.Decimal().Equal(nuevo.Rows[0].Payment) {
		t.Fatalf("payment.new = %s, want %s", res.Payment.New, nuevo.Rows[0].Payment)
	}
	if !res.Payment.Previous.Decimal().Equal(viejo.Rows[0].Payment) {
		t.Fatalf("payment.previous = %s, want %s", res.Payment.Previous, viejo.Rows[0].Payment)
	}
	diff := nuevo.Rows[0].Payment.Sub(viejo.Rows[0].Payment)
	if !res.Payment.DifferenceMonth.Decimal().Equal(diff) {
		t.Fatalf("difference_month = %s, want %s", res.Payment.DifferenceMonth, diff)
	}
	if !res.Payment.DifferenceYear.Decimal().Equal(diff.Mul(decimal.NewFromInt(12))) {
		t.Fatalf("difference_year = %s, want 12×%s", res.Payment.DifferenceYear, diff)
	}
	// §5.2: sin avisos, warnings es una lista vacía, no null.
	if res.Warnings == nil || len(res.Warnings) != 0 {
		t.Fatalf("warnings = %v, want [] (lista vacía, no null)", res.Warnings)
	}
	if res.Attribution.Text != "Elaboración propia con datos extraídos del sitio web del Banco de España (www.bde.es)" ||
		res.Attribution.DataUpdatedAt != "2026-09-01" || !res.Attribution.NotEndorsed {
		t.Fatalf("attribution = %+v", res.Attribution)
	}
}

// TestRevisionAssumed: con --assume no se consulta la serie: el valor es el
// supuesto, va marcado assumed y aparece un aviso (P-06, decisión 5).
func TestRevisionAssumed(t *testing.T) {
	path := configCon(t, map[string]loan.Loan{"casa": prestamoRevision()})
	rt := runtimeRevision(t)

	assume, err := model.NewDecimal("2.35")
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	res, err := Revision(t.Context(), rt, RevisionParams{
		ConfigPath: path,
		LoanID:     "casa",
		Date:       "2026-11",
		Assume:     &assume,
		Now:        ahoraFijo(),
	})
	if err != nil {
		t.Fatalf("Revision() con --assume: %v", err)
	}
	if res.Index.Status != "assumed" || res.Index.Value.String() != "2.35" {
		t.Fatalf("index = %s/%s, want assumed/2.35", res.Index.Status, res.Index.Value)
	}
	if len(res.Warnings) != 1 {
		t.Fatalf("warnings = %v, want el aviso de supuesto", res.Warnings)
	}
	// El TIN nuevo es el supuesto + diferencial, no el valor publicado.
	if res.Rate.Raw.String() != "3.24" {
		t.Fatalf("rate.raw = %s, want 3.24 (2.35 + 0.89)", res.Rate.Raw)
	}
	if res.Attribution.DataUpdatedAt != "2026-10-15" {
		t.Fatalf("attribution = %s, want la fecha de elaboración del escenario", res.Attribution.DataUpdatedAt)
	}
}

// TestRevisionMesNoPublicado: un mes de referencia sin valor en la serie es
// index_not_published/4 con el hint de --assume: nunca se inventa (P-06).
func TestRevisionMesNoPublicado(t *testing.T) {
	path := configCon(t, map[string]loan.Loan{"casa": prestamoRevision()})
	rt := runtimeRevision(t)

	// La revisión de 2026-12 pide el Euríbor de 2026-10, que el fixture no
	// tiene (la serie termina en 2026-09).
	_, err := Revision(t.Context(), rt, RevisionParams{
		ConfigPath: path,
		LoanID:     "casa",
		Date:       "2026-12",
		Now:        ahoraFijo(),
	})
	e := assertAppError(t, err, errs.KindIndexNotPublished, 4)
	if !containsAll(e.Hint, "--assume") {
		t.Fatalf("el hint debe apuntar a --assume: %q", e.Hint)
	}
	if !containsAll(e.Message, "2026-10") {
		t.Fatalf("el mensaje debe nombrar el mes de referencia: %q", e.Message)
	}
}

// TestRevisionPrestamoIncompleto: un préstamo sin un dato de la escritura
// responde loan_incomplete/2 nombrando el campo (decisión 1 de LOAN-01).
func TestRevisionPrestamoIncompleto(t *testing.T) {
	l := prestamoRevision()
	l.ReferenceOffsetMonths = nil
	path := configCon(t, map[string]loan.Loan{"casa": l})
	rt := runtimeRevision(t)

	_, err := Revision(t.Context(), rt, RevisionParams{
		ConfigPath: path,
		LoanID:     "casa",
		Date:       "2026-11",
		Now:        ahoraFijo(),
	})
	e := assertAppError(t, err, errs.KindLoanIncomplete, 2)
	if !containsAll(e.Message, "reference_offset_months") || !containsAll(e.Hint, "reference_offset_months") {
		t.Fatalf("loan_incomplete debe nombrar el campo: %q / %q", e.Message, e.Hint)
	}
}

// TestRevisionTramoInicialSinSync: dentro del tramo inicial la revisión se
// calcula con el cuadro propio del préstamo (outstanding_source schedule) y
// el tipo previo es el inicial.
func TestRevisionTramoInicialSinSync(t *testing.T) {
	l := prestamoCompleto() // sin sync
	path := configCon(t, map[string]loan.Loan{"casa": l})
	rt := runtimeRevision(t)

	res, err := Revision(t.Context(), rt, RevisionParams{
		ConfigPath: path,
		LoanID:     "casa",
		Date:       "2022-06", // mes 12: fin del tramo inicial
		Now:        ahoraFijo(),
	})
	if err != nil {
		t.Fatalf("Revision(): %v", err)
	}
	if res.Inputs.OutstandingSource != "schedule" {
		t.Fatalf("outstanding_source = %q, want schedule", res.Inputs.OutstandingSource)
	}
	if res.Rate.Previous.String() != "1.25" {
		t.Fatalf("tipo previo = %s, want 1.25 (el inicial)", res.Rate.Previous)
	}
	if res.Inputs.RemainingMonths != 348 {
		t.Fatalf("remaining_months = %d, want 348", res.Inputs.RemainingMonths)
	}
}

// TestRevisionCapitalSinCobertura: una revisión posterior al tramo inicial
// sin recibo no se calcula: el índice de las revisiones intermedias no está
// guardado y suponerlo inventaría el capital (P-06).
func TestRevisionCapitalSinCobertura(t *testing.T) {
	l := prestamoCompleto() // sin sync
	path := configCon(t, map[string]loan.Loan{"casa": l})
	rt := runtimeRevision(t)

	_, err := Revision(t.Context(), rt, RevisionParams{
		ConfigPath: path,
		LoanID:     "casa",
		Date:       "2023-06", // mes 24, dos revisiones después del tramo inicial
		Now:        ahoraFijo(),
	})
	assertAppError(t, err, errs.KindUsage, 2)
}

// TestRevisionFijaRespondeUsage: un préstamo fijo no tiene revisiones de
// tipo, así que `revision` no calcula nada: responde usage/2 nombrando el
// tipo del contrato en lugar de presentar una revisión fabricada (verificador
// de FIN-04, major 1).
func TestRevisionFijaRespondeUsage(t *testing.T) {
	l := prestamoCompleto()
	l.Type = loan.TypeFija
	path := configCon(t, map[string]loan.Loan{"casa": l})
	rt := runtimeRevision(t)

	for _, fecha := range []string{"2023-06", ""} { // con y sin --date
		_, err := Revision(t.Context(), rt, RevisionParams{
			ConfigPath: path,
			LoanID:     "casa",
			Date:       fecha,
			Now:        ahoraFijo(),
		})
		e := assertAppError(t, err, errs.KindUsage, 2)
		if !containsAll(e.Message, "fijo", "1.25") {
			t.Fatalf("el mensaje debe decir que el préstamo es fijo y nombrar el tipo del contrato (%q): %q", fecha, e.Message)
		}
		if !containsAll(e.Hint, "schedule") {
			t.Fatalf("el hint debe apuntar a lo que un fijo sí permite: %q", e.Hint)
		}
	}
}

// TestRevisionProximaReviscion: sin --date, la revisión es la primera
// posterior al mes en curso.
func TestRevisionProximaReviscion(t *testing.T) {
	// Préstamo firmado en 2025-11: tramo inicial de 12 meses, primera
	// revisión en 2026-11, cuyo mes de referencia (2026-09) sí está en el
	// fixture.
	l := prestamoRevision()
	l.StartDate = "2025-11-01"
	path := configCon(t, map[string]loan.Loan{"casa": l})
	rt := runtimeRevision(t)

	res, err := Revision(t.Context(), rt, RevisionParams{
		ConfigPath: path,
		LoanID:     "casa",
		Now:        ahoraFijo(), // 2026-10-15
	})
	if err != nil {
		t.Fatalf("Revision() sin --date: %v", err)
	}
	if res.RevisionDate != "2026-11-01" {
		t.Fatalf("revision_date = %q, want 2026-11-01", res.RevisionDate)
	}
	if res.Index.ReferenceMonth != "2026-09" {
		t.Fatalf("reference_month = %q, want 2026-09", res.Index.ReferenceMonth)
	}
}

// TestRevisionIdOpcional: con un único préstamo, omitir el id es válido
// (LOAN-01.4).
func TestRevisionIdOpcional(t *testing.T) {
	path := configCon(t, map[string]loan.Loan{"casa": prestamoRevision()})
	rt := runtimeRevision(t)

	res, err := Revision(t.Context(), rt, RevisionParams{
		ConfigPath: path,
		Date:       "2026-11",
		Now:        ahoraFijo(),
	})
	if err != nil {
		t.Fatalf("Revision() sin id: %v", err)
	}
	if res.Loan != "casa" {
		t.Fatalf("loan = %q, want casa", res.Loan)
	}
}
