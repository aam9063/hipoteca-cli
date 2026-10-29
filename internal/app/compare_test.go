package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
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
// `compare` (FIN-04): recalcular la historia del préstamo con otro índice y
// otro diferencial, usando el valor publicado de cada mes de referencia. Los
// históricos reales salen de los fixtures de testdata/bde servidos por el
// `Source` existente (decisión 1: cada revisión usa su propio mes). El
// servidor de prueba reparte por código de serie: el Euríbor trae su captura
// MAX y el IRPH su captura 30M (2024-03..2026-09).
// ----------------------------------------------------------------------------

// prestamoCompare: préstamo variable corto cuyo ciclo de revisiones cae
// dentro de las dos capturas: revisiones en 2025-01 (referencia 2024-11) y
// 2026-01 (referencia 2025-11), ambas publicadas para euribor-1y e irph.
func prestamoCompare() loan.Loan {
	initialPeriod := 12
	offset := 2
	sinSuelo := ""
	return loan.Loan{
		Name:                  "Casa",
		Type:                  loan.TypeVariable,
		StartDate:             "2024-01-01",
		OriginalPrincipal:     "150000.00",
		TermMonths:            36,
		PaymentDay:            1,
		InitialRate:           "2.00",
		InitialPeriodMonths:   &initialPeriod,
		Index:                 "euribor-1y",
		Spread:                "0.50",
		RevisionEveryMonths:   12,
		ReferenceOffsetMonths: &offset,
		FloorRate:             &sinSuelo,
		RateRounding:          "none",
		InterestConvention:    loan.InterestConventionMonthlyNominal,
		PaymentRounding:       loan.PaymentRoundingHalfUpCents,
		EarlyRepaymentFeePct:  "0",
	}
}

// compareServer sirve la captura de cada serie por su código: euribor-1y
// (D_1NBAF472) la MAX e irph (D_1T9H0000) la de 30M. Cualquier otra serie es
// un error del contrato, como en producción. Con irphOverride no nil se sirve
// ese cuerpo para el IRPH (el test de cobertura necesita un histórico que
// llegue hasta 2021, y el fixture de 30M empieza en 2024-03).
func compareServer(t *testing.T, irphOverride []byte) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/listaSeries", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("series") {
		case "D_1NBAF472":
			_, _ = w.Write(readBDEFixture(t, "listaSeries-euribor-1y-MAX.json"))
		case "D_1T9H0000":
			if irphOverride != nil {
				_, _ = w.Write(irphOverride)
			} else {
				_, _ = w.Write(readBDEFixture(t, "listaSeries-irph-30M.json"))
			}
		default:
			_, _ = w.Write(readBDEFixture(t, "error-listaSeries-serie-inexistente.json"))
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// irphLargo genera un listaSeries del IRPH (D_1T9H0000) de 2021-01 a 2026-09
// con un valor por año (2021: 3.00, 2022: 3.10, … 2026: 3.50), descendente
// como contesta la API. No toca testdata: se genera en el test.
func irphLargo(t *testing.T) []byte {
	t.Helper()
	var fechas, valores []string
	for m := time.Date(2026, time.September, 1, 8, 15, 0, 0, time.UTC); m.After(time.Date(2020, time.December, 1, 0, 0, 0, 0, time.UTC)); m = m.AddDate(0, -1, 0) {
		fechas = append(fechas, m.Format("2006-01-02T15:04:05Z"))
		valores = append(valores, fmt.Sprintf("%.2f", 3.00+0.10*float64(m.Year()-2021)))
	}
	body := fmt.Sprintf(`[{"serie":"D_1T9H0000","descripcion":"IRPH","descripcionCorta":"IRPH","codFrecuencia":"M","decimales":3,"simbolo":"%%","fechaInicio":"2021-01-01T09:15:00Z","fechaFin":"2026-09-01T08:15:00Z","fechas":[%s],"valores":[%s]}]`,
		quoteEach(fechas), strings.Join(valores, ","))
	return []byte(body)
}

// runtimeCompare monta el runtime de prueba con el reparto por serie.
func runtimeCompare(t *testing.T, irphOverride ...[]byte) *Runtime {
	var override []byte
	if len(irphOverride) > 0 {
		override = irphOverride[0]
	}
	src := fakeSource{t: t, base: compareServer(t, override).URL}
	cat, err := bde.LoadCatalog(nil)
	if err != nil {
		t.Fatalf("cargar el catálogo: %v", err)
	}
	return &Runtime{Catalog: cat, Source: src}
}

// pagosEsperados recalcula con el núcleo (misma convención que la
// implementación: la cuota de cada revisión con el capital pendiente y los
// meses restantes) los pagos de cada bloque: el inicial a tipo fijo y uno por
// revisión con su TIN. Devuelve el pago inicial, los pagos de las revisiones
// y los totales del cuadro compuesto.
func pagosEsperados(t *testing.T, tin1, tin2 string) (inicial, rev1, rev2, totalPagado, totalInteres decimal.Decimal) {
	t.Helper()
	cuadro, err := finance.Amortization(finance.Loan{
		Principal:  decimal.RequireFromString("150000"),
		AnnualRate: decimal.RequireFromString("2.00"),
		Months:     36,
	})
	if err != nil {
		t.Fatalf("cuadro inicial: %v", err)
	}
	inicial = cuadro.Rows[0].Payment
	b12 := cuadro.Rows[11].Balance
	c1, err := finance.Amortization(finance.Loan{Principal: b12, AnnualRate: decimal.RequireFromString(tin1), Months: 24})
	if err != nil {
		t.Fatalf("cuadro de la revisión 1: %v", err)
	}
	rev1 = c1.Rows[0].Payment
	b24 := c1.Rows[11].Balance
	c2, err := finance.Amortization(finance.Loan{Principal: b24, AnnualRate: decimal.RequireFromString(tin2), Months: 12})
	if err != nil {
		t.Fatalf("cuadro de la revisión 2: %v", err)
	}
	rev2 = c2.Rows[0].Payment
	sched, err := finance.CompoundSchedule(
		finance.Loan{Principal: decimal.RequireFromString("150000"), Months: 36},
		[]finance.Period{
			{Rate: decimal.RequireFromString("2.00"), Months: 12},
			{Rate: decimal.RequireFromString(tin1), Months: 12},
			{Rate: decimal.RequireFromString(tin2), Months: 12},
		})
	if err != nil {
		t.Fatalf("cuadro compuesto: %v", err)
	}
	return inicial, rev1, rev2, sched.TotalPaid, sched.TotalInterest
}

// TestCompareUsaElIndiceDeSuMesDeReferencia: con los históricos reales de los
// dos índices, cada revisión usa el valor de SU mes de referencia (decisión 1
// de FIN-04) y los dos bloques son simétricos (decisión 3): mismos periodos y
// mismas fechas, cada uno con su serie declarada (decisión 4).
func TestCompareUsaElIndiceDeSuMesDeReferencia(t *testing.T) {
	path := configCon(t, map[string]loan.Loan{"casa": prestamoCompare()})
	rt := runtimeCompare(t)

	res, err := Compare(t.Context(), rt, CompareParams{
		ConfigPath: path,
		LoanID:     "casa",
		Index:      "irph",
		Spread:     "1.00",
		Now:        ahoraFijo(),
	})
	if err != nil {
		t.Fatalf("Compare(): %v", err)
	}
	if res.SchemaVersion != model.SchemaVersion {
		t.Fatalf("schema_version = %d", res.SchemaVersion)
	}
	if res.Loan != "casa" {
		t.Fatalf("loan = %q", res.Loan)
	}

	// Bloque propio: euribor-1y con el diferencial de la escritura; las
	// revisiones usan el Euríbor de su mes de referencia (2.506 en 2024-11 y
	// 2.217 en 2025-11, offset 2).
	if res.Own.Index.ID != "euribor-1y" || res.Own.Index.Code != "D_1NBAF472" || res.Own.Index.Status != "published" {
		t.Fatalf("index propio = %+v, want euribor-1y/D_1NBAF472/published", res.Own.Index)
	}
	if res.Own.Spread.String() != "0.50" || res.Own.InitialRate.String() != "2.00" || res.Own.InitialMonths != 12 {
		t.Fatalf("bloque propio (spread/inicial) = %s/%s/%d", res.Own.Spread, res.Own.InitialRate, res.Own.InitialMonths)
	}
	if len(res.Own.Revisions) != 2 {
		t.Fatalf("el bloque propio trae %d revisiones, want 2", len(res.Own.Revisions))
	}
	revPropia1, revPropia2 := res.Own.Revisions[0], res.Own.Revisions[1]
	if revPropia1.Period != 1 || revPropia1.RevisionDate != "2025-01-01" || revPropia1.ReferenceMonth != "2024-11" {
		t.Fatalf("revisión propia 1 = %+v", revPropia1)
	}
	if revPropia1.IndexValue.String() != "2.506" || revPropia1.Assumed {
		t.Fatalf("valor propio 1 = %s (assumed %v), want 2.506 published", revPropia1.IndexValue, revPropia1.Assumed)
	}
	if revPropia1.AnnualRatePct.String() != "3.006" {
		t.Fatalf("TIN propio 1 = %s, want 3.006 (2.506 + 0.50)", revPropia1.AnnualRatePct)
	}
	if revPropia2.Period != 2 || revPropia2.RevisionDate != "2026-01-01" || revPropia2.ReferenceMonth != "2025-11" {
		t.Fatalf("revisión propia 2 = %+v", revPropia2)
	}
	if revPropia2.IndexValue.String() != "2.217" || revPropia2.AnnualRatePct.String() != "2.717" {
		t.Fatalf("revisión propia 2 = %s/%s, want 2.217/2.717", revPropia2.IndexValue, revPropia2.AnnualRatePct)
	}

	// Bloque alternativo: irph + 1.00 con SUS valores de referencia.
	if res.Alternative.Index.ID != "irph" || res.Alternative.Index.Code != "D_1T9H0000" || res.Alternative.Index.Status != "published" {
		t.Fatalf("index alternativo = %+v, want irph/D_1T9H0000/published", res.Alternative.Index)
	}
	if res.Alternative.Spread.String() != "1.00" || res.Alternative.InitialRate.String() != "2.00" || res.Alternative.InitialMonths != 12 {
		t.Fatalf("bloque alternativo (spread/inicial) = %s/%s/%d", res.Alternative.Spread, res.Alternative.InitialRate, res.Alternative.InitialMonths)
	}
	if len(res.Alternative.Revisions) != 2 {
		t.Fatalf("el bloque alternativo trae %d revisiones, want 2", len(res.Alternative.Revisions))
	}
	revAlt1, revAlt2 := res.Alternative.Revisions[0], res.Alternative.Revisions[1]
	if revAlt1.ReferenceMonth != "2024-11" || revAlt1.IndexValue.String() != "3.206" || revAlt1.AnnualRatePct.String() != "4.206" {
		t.Fatalf("revisión alternativa 1 = %+v, want 2024-11/3.206/4.206", revAlt1)
	}
	if revAlt2.ReferenceMonth != "2025-11" || revAlt2.IndexValue.String() != "2.807" || revAlt2.AnnualRatePct.String() != "3.807" {
		t.Fatalf("revisión alternativa 2 = %+v, want 2025-11/2.807/3.807", revAlt2)
	}

	// Simetría (decisión 3): las fechas y los meses de referencia son los
	// mismos en los dos bloques; nada declara un ganador.
	if revAlt1.RevisionDate != revPropia1.RevisionDate || revAlt2.RevisionDate != revPropia2.RevisionDate {
		t.Fatalf("los bloques no son simétricos: %s/%s vs %s/%s",
			revPropia1.RevisionDate, revPropia2.RevisionDate, revAlt1.RevisionDate, revAlt2.RevisionDate)
	}

	// Cuotas: el pago inicial es el mismo en los dos bloques (tramo inicial
	// al tipo de la escritura) y el de cada revisión sale del núcleo con el
	// capital pendiente y los meses restantes.
	_, wantPropia1, wantPropia2, wantPagadoPropio, wantInteresPropio := pagosEsperados(t, "3.006", "2.717")
	_, wantAlt1, wantAlt2, wantPagadoAlt, wantInteresAlt := pagosEsperados(t, "4.206", "3.807")
	if !revPropia1.Payment.Decimal().Equal(wantPropia1) || !revPropia2.Payment.Decimal().Equal(wantPropia2) {
		t.Fatalf("pagos propios = %s/%s, want %s/%s", revPropia1.Payment, revPropia2.Payment, wantPropia1, wantPropia2)
	}
	if !revAlt1.Payment.Decimal().Equal(wantAlt1) || !revAlt2.Payment.Decimal().Equal(wantAlt2) {
		t.Fatalf("pagos alternativos = %s/%s, want %s/%s", revAlt1.Payment, revAlt2.Payment, wantAlt1, wantAlt2)
	}
	if !res.Own.TotalPaid.Decimal().Equal(wantPagadoPropio) || !res.Own.TotalInterest.Decimal().Equal(wantInteresPropio) {
		t.Fatalf("totales propios = %s/%s, want %s/%s", res.Own.TotalPaid, res.Own.TotalInterest, wantPagadoPropio, wantInteresPropio)
	}
	if !res.Alternative.TotalPaid.Decimal().Equal(wantPagadoAlt) || !res.Alternative.TotalInterest.Decimal().Equal(wantInteresAlt) {
		t.Fatalf("totales alternativos = %s/%s, want %s/%s", res.Alternative.TotalPaid, res.Alternative.TotalInterest, wantPagadoAlt, wantInteresAlt)
	}

	// Diferencia por año: alternativa − propia, mes y año, con las cuotas de
	// cada bloque junto a la diferencia.
	if len(res.Differences) != 2 {
		t.Fatalf("differences = %d entradas, want 2", len(res.Differences))
	}
	d1, d2 := res.Differences[0], res.Differences[1]
	if d1.Period != 1 || d1.Year != 2025 {
		t.Fatalf("diferencia 1 = periodo %d año %d, want periodo 1 año 2025", d1.Period, d1.Year)
	}
	if !d1.PaymentOwn.Decimal().Equal(wantPropia1) || !d1.PaymentAlternative.Decimal().Equal(wantAlt1) {
		t.Fatalf("diferencia 1 = %s vs %s, want %s vs %s", d1.PaymentOwn, d1.PaymentAlternative, wantPropia1, wantAlt1)
	}
	if !d1.DifferenceMonth.Decimal().Equal(wantAlt1.Sub(wantPropia1)) {
		t.Fatalf("difference_month 1 = %s, want %s", d1.DifferenceMonth, wantAlt1.Sub(wantPropia1))
	}
	if !d1.DifferenceYear.Decimal().Equal(wantAlt1.Sub(wantPropia1).Mul(decimal.NewFromInt(12))) {
		t.Fatalf("difference_year 1 = %s", d1.DifferenceYear)
	}
	if d2.Period != 2 || d2.Year != 2026 || !d2.DifferenceMonth.Decimal().Equal(wantAlt2.Sub(wantPropia2)) {
		t.Fatalf("diferencia 2 = %+v", d2)
	}

	// Totales comparados arriba, con su diferencia alternativa − propia.
	if !res.TotalPaid.Own.Decimal().Equal(wantPagadoPropio) || !res.TotalPaid.Alternative.Decimal().Equal(wantPagadoAlt) ||
		!res.TotalPaid.Difference.Decimal().Equal(wantPagadoAlt.Sub(wantPagadoPropio)) {
		t.Fatalf("total_paid = %+v", res.TotalPaid)
	}
	if !res.TotalInterest.Difference.Decimal().Equal(wantInteresAlt.Sub(wantInteresPropio)) {
		t.Fatalf("total_interest = %+v", res.TotalInterest)
	}

	// Atribución de datos procesados (P-05) con la última actualización de
	// las dos series, y sin avisos cuando todo es publicado y el plazo queda
	// cubierto: el cuadro llega hasta el fin del préstamo, así que no hay
	// aviso de cobertura.
	if res.Coverage.From != "2024-11" || res.Coverage.To != "2025-11" {
		t.Fatalf("coverage = %s..%s, want 2024-11..2025-11", res.Coverage.From, res.Coverage.To)
	}
	if res.Attribution.DataUpdatedAt != "2026-09-01" || !res.Attribution.NotEndorsed ||
		res.Attribution.Text != "Elaboración propia con datos extraídos del sitio web del Banco de España (www.bde.es)" {
		t.Fatalf("attribution = %+v", res.Attribution)
	}
	if res.Warnings == nil || len(res.Warnings) != 0 {
		t.Fatalf("warnings = %v, want [] (lista vacía, no null)", res.Warnings)
	}
}

// prestamoArranque es el préstamo de la observación de cierre de FIN-04:
// 150000 a 300 meses desde 2021-06, euríbor + 0.89. Sus revisiones van de
// 2022-06 a 2046, pero la comparación sólo cubre las de meses de referencia
// publicados en los dos índices.
func prestamoArranque() loan.Loan {
	l := prestamoCompleto()
	l.OriginalPrincipal = "150000.00"
	l.TermMonths = 300
	l.Spread = "0.89"
	return l
}

// TestCompareCubreSoloLosMesesPublicados: la comparación es sobre el pasado.
// Se compara hasta la última revisión cuyo mes de referencia está publicado
// en los DOS índices; ninguna revisión posterior se consulta, calcula o
// muestra, y la cobertura se declara en la salida con un aviso cuando el
// plazo sigue más allá (P-06: sin proyección).
func TestCompareCubreSoloLosMesesPublicados(t *testing.T) {
	path := configCon(t, map[string]loan.Loan{"casa": prestamoArranque()})
	rt := runtimeCompare(t, irphLargo(t))

	// Sin ventana: antes fallaba con index_not_published por 2027-04.
	res, err := Compare(t.Context(), rt, CompareParams{
		ConfigPath: path,
		LoanID:     "casa",
		Index:      "irph",
		Spread:     "0",
		Now:        ahoraFijo(),
	})
	if err != nil {
		t.Fatalf("Compare(): %v", err)
	}

	// Revisiones 2022-06..2026-06 (referencias 2022-04..2026-04, ambas
	// series terminan en 2026-09); la de 2027-06 (referencia 2027-04) no
	// existe en la salida.
	if len(res.Own.Revisions) != 5 || len(res.Alternative.Revisions) != 5 {
		t.Fatalf("revisiones = %d/%d, want 5/5 (hasta la referencia 2026-04)",
			len(res.Own.Revisions), len(res.Alternative.Revisions))
	}
	ultima := res.Own.Revisions[len(res.Own.Revisions)-1]
	if ultima.ReferenceMonth != "2026-04" || ultima.RevisionDate != "2026-06-01" {
		t.Fatalf("última revisión = %s/%s, want 2026-04/2026-06-01", ultima.ReferenceMonth, ultima.RevisionDate)
	}
	for _, bloque := range []CompareBlock{res.Own, res.Alternative} {
		for _, rev := range bloque.Revisions {
			if rev.ReferenceMonth > "2026-09" {
				t.Fatalf("la salida trae una revisión con referencia no publicada: %s", rev.ReferenceMonth)
			}
		}
	}

	// Cada revisión usa el índice de su mes de referencia, año por año, en
	// los dos bloques: euríbor 0.013/3.757/3.703/2.143/2.747 e IRPH sintético
	// 3.10/3.20/3.30/3.40/3.50, con spread 0.
	wantOwn := []string{"0.013", "3.757", "3.703", "2.143", "2.747"}
	wantAlt := []string{"3.10", "3.20", "3.30", "3.40", "3.50"}
	for i, rev := range res.Own.Revisions {
		if rev.IndexValue.String() != wantOwn[i] {
			t.Fatalf("valor propio %d (%s) = %s, want %s", i, rev.ReferenceMonth, rev.IndexValue, wantOwn[i])
		}
		if res.Alternative.Revisions[i].IndexValue.String() != wantAlt[i] {
			t.Fatalf("valor alternativo %d (%s) = %s, want %s", i, rev.ReferenceMonth,
				res.Alternative.Revisions[i].IndexValue, wantAlt[i])
		}
	}

	// Cobertura declarada y aviso: el plazo (2046) sigue más allá del
	// periodo publicado, y la salida lo dice sin proyectar nada (P-06).
	if res.Coverage.From != "2022-04" || res.Coverage.To != "2026-04" {
		t.Fatalf("coverage = %s..%s, want 2022-04..2026-04", res.Coverage.From, res.Coverage.To)
	}
	if len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "P-06") || strings.Contains(res.Warnings[0], "2027") {
		t.Fatalf("warnings = %v, want el aviso de cobertura (P-06, sin proyección)", res.Warnings)
	}

	// La ventana dentro del periodo publicado sigue funcionando y el --to
	// declarado no pide meses sin datos.
	ventana, err := Compare(t.Context(), rt, CompareParams{
		ConfigPath: path,
		LoanID:     "casa",
		Index:      "irph",
		Spread:     "0",
		From:       "2024-01",
		To:         "2025-12",
		Now:        ahoraFijo(),
	})
	if err != nil {
		t.Fatalf("Compare() con ventana: %v", err)
	}
	if len(ventana.Own.Revisions) != 2 {
		t.Fatalf("la ventana debe dejar 2 revisiones (2024-06 y 2025-06), got %d", len(ventana.Own.Revisions))
	}
}

// TestCompareToFuturoEsUsage: pedir explícitamente meses posteriores a los
// publicados es usage/2 (la comparación sólo cubre meses publicados), no
// index_not_published: no se puede conseguir ese dato.
func TestCompareToFuturoEsUsage(t *testing.T) {
	path := configCon(t, map[string]loan.Loan{"casa": prestamoArranque()})
	rt := runtimeCompare(t, irphLargo(t))

	_, err := Compare(t.Context(), rt, CompareParams{
		ConfigPath: path,
		LoanID:     "casa",
		Index:      "irph",
		Spread:     "0",
		To:         "2027-01", // posterior a la última referencia publicada + offset
		Now:        ahoraFijo(),
	})
	e := assertAppError(t, err, errs.KindUsage, 2)
	if !containsAll(e.Message, "publicado") {
		t.Fatalf("el mensaje debe decir que sólo se cubren los meses publicados: %q", e.Message)
	}
}

// TestCompareToConHueco: un --to dentro del periodo publicado que cae en un
// mes sin valor en una de las series sigue siendo index_not_published/4
// nombrando el índice y el mes (decisión 2 de FIN-04).
func TestCompareToConHueco(t *testing.T) {
	path := configCon(t, map[string]loan.Loan{"casa": prestamoArranque()})
	rt := runtimeCompare(t) // el IRPH del fixture 30M empieza en 2024-03

	_, err := Compare(t.Context(), rt, CompareParams{
		ConfigPath: path,
		LoanID:     "casa",
		Index:      "irph",
		Spread:     "0",
		To:         "2023-12", // dentro del rango pedido, pero 2022-04 no está en el fixture
		Now:        ahoraFijo(),
	})
	e := assertAppError(t, err, errs.KindIndexNotPublished, 4)
	if !containsAll(e.Message, "irph", "2022-04") {
		t.Fatalf("el mensaje debe nombrar el índice y el mes: %q", e.Message)
	}
}

// TestCompareSinNingunaRevisionPublicada: si ni siquiera la primera revisión
// tiene el mes de referencia publicado en los dos índices no hay nada que
// comparar: usage/2, no un resultado vacío ni un índice inventado.
func TestCompareSinNingunaRevisionPublicada(t *testing.T) {
	l := prestamoArranque()
	l.StartDate = "2026-06-01" // primera revisión 2027-06, referencia 2027-04
	path := configCon(t, map[string]loan.Loan{"casa": l})
	rt := runtimeCompare(t, irphLargo(t))

	_, err := Compare(t.Context(), rt, CompareParams{
		ConfigPath: path,
		LoanID:     "casa",
		Index:      "irph",
		Spread:     "0",
		Now:        ahoraFijo(),
	})
	e := assertAppError(t, err, errs.KindUsage, 2)
	if !containsAll(e.Message, "ninguna revisión") {
		t.Fatalf("el mensaje debe decir que no hay revisiones comparables: %q", e.Message)
	}
}

// TestCompareMesNoPublicadoDelAlternativo: si falta un mes del índice
// alternativo no se inventa (decisión 2): index_not_published/4 nombrando el
// índice y el mes, sin interpolaciones ni valores previos.
func TestCompareMesNoPublicadoDelAlternativo(t *testing.T) {
	l := prestamoCompare()
	// Empezando en 2023-02, la primera revisión (2024-02) usa el índice de
	// 2023-12: el Euríbor lo tiene publicado (3.679) y el IRPH de la captura
	// 30M empieza en 2024-03.
	l.StartDate = "2023-02-01"
	path := configCon(t, map[string]loan.Loan{"casa": l})
	rt := runtimeCompare(t)

	_, err := Compare(t.Context(), rt, CompareParams{
		ConfigPath: path,
		LoanID:     "casa",
		Index:      "irph",
		Spread:     "1.00",
		Now:        ahoraFijo(),
	})
	e := assertAppError(t, err, errs.KindIndexNotPublished, 4)
	if !containsAll(e.Message, "irph", "2023-12") {
		t.Fatalf("el mensaje debe nombrar el índice y el mes: %q", e.Message)
	}
	if !containsAll(e.Hint, "--index-values") {
		t.Fatalf("el hint debe apuntar a --index-values: %q", e.Hint)
	}
}

// TestCompareConValoresSupuestos: --index-values sustituye el histórico del
// índice alternativo por valores supuestos, uno por revisión, marcados
// assumed con el aviso de P-06; el bloque propio sigue siendo publicado.
func TestCompareConValoresSupuestos(t *testing.T) {
	path := configCon(t, map[string]loan.Loan{"casa": prestamoCompare()})
	rt := runtimeCompare(t)

	v1, err := model.NewDecimal("3.0")
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	v2, err := model.NewDecimal("2.9")
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	res, err := Compare(t.Context(), rt, CompareParams{
		ConfigPath:  path,
		LoanID:      "casa",
		Index:       "irph",
		Spread:      "1.00",
		IndexValues: []model.Decimal{v1, v2},
		Now:         ahoraFijo(),
	})
	if err != nil {
		t.Fatalf("Compare() con --index-values: %v", err)
	}
	if res.Own.Index.Status != "published" {
		t.Fatalf("el bloque propio debe seguir siendo publicado, got %q", res.Own.Index.Status)
	}
	if res.Alternative.Index.Status != "assumed" {
		t.Fatalf("index alternativo = %q, want assumed", res.Alternative.Index.Status)
	}
	if res.Alternative.Revisions[0].IndexValue.String() != "3.0" || !res.Alternative.Revisions[0].Assumed {
		t.Fatalf("revisión alternativa 1 = %s (assumed %v), want 3.0 assumed", res.Alternative.Revisions[0].IndexValue, res.Alternative.Revisions[0].Assumed)
	}
	if res.Alternative.Revisions[0].AnnualRatePct.String() != "4.00" {
		t.Fatalf("TIN alternativo 1 = %s, want 4.00 (3.0 + 1.00)", res.Alternative.Revisions[0].AnnualRatePct)
	}
	if res.Alternative.Revisions[1].IndexValue.String() != "2.9" {
		t.Fatalf("revisión alternativa 2 = %s, want 2.9", res.Alternative.Revisions[1].IndexValue)
	}
	if len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "P-06") {
		t.Fatalf("warnings = %v, want el aviso de supuesto (P-06)", res.Warnings)
	}
	// La atribución cita la última actualización de los datos reales usados:
	// el bloque propio sigue siendo publicado (su serie termina en 2026-09).
	if res.Attribution.DataUpdatedAt != "2026-09-01" {
		t.Fatalf("attribution = %s, want la actualización de la serie propia", res.Attribution.DataUpdatedAt)
	}

	// Otro número de valores que revisiones es un error de uso, no un
	// recorte silencioso.
	_, err = Compare(t.Context(), rt, CompareParams{
		ConfigPath:  path,
		LoanID:      "casa",
		Index:       "irph",
		Spread:      "1.00",
		IndexValues: []model.Decimal{v1},
		Now:         ahoraFijo(),
	})
	assertAppError(t, err, errs.KindUsage, 2)
}

// TestCompareVentana: --from/--to limita las revisiones que se muestran y
// comparan; el recálculo completo es el mismo.
func TestCompareVentana(t *testing.T) {
	path := configCon(t, map[string]loan.Loan{"casa": prestamoCompare()})
	rt := runtimeCompare(t)

	completo, err := Compare(t.Context(), rt, CompareParams{
		ConfigPath: path,
		LoanID:     "casa",
		Index:      "irph",
		Spread:     "1.00",
		Now:        ahoraFijo(),
	})
	if err != nil {
		t.Fatalf("Compare(): %v", err)
	}
	ventana, err := Compare(t.Context(), rt, CompareParams{
		ConfigPath: path,
		LoanID:     "casa",
		Index:      "irph",
		Spread:     "1.00",
		From:       "2026-01",
		To:         "2026-11",
		Now:        ahoraFijo(),
	})
	if err != nil {
		t.Fatalf("Compare() con ventana: %v", err)
	}
	if len(ventana.Own.Revisions) != 1 || len(ventana.Alternative.Revisions) != 1 || len(ventana.Differences) != 1 {
		t.Fatalf("la ventana debe dejar una revisión: %d/%d/%d",
			len(ventana.Own.Revisions), len(ventana.Alternative.Revisions), len(ventana.Differences))
	}
	if ventana.Own.Revisions[0].RevisionDate != "2026-01-01" || ventana.Own.Revisions[0].Period != 2 {
		t.Fatalf("la revisión de la ventana = %+v, want el periodo 2 (2026-01)", ventana.Own.Revisions[0])
	}
	if !ventana.TotalPaid.Difference.Decimal().Equal(completo.TotalPaid.Difference.Decimal()) {
		t.Fatalf("la ventana no debe cambiar el recálculo: total_paid difference %s vs %s",
			ventana.TotalPaid.Difference, completo.TotalPaid.Difference)
	}
}

// TestCompareLoose: el camino sin préstamo guardado. Los valores de
// --index-values son los supuestos de los DOS bloques, uno por periodo de 12
// meses (el primero es el tipo de partida); los bloques se diferencian por el
// diferencial propio y el alternativo. Nada se consulta y todo va marcado
// assumed (P-06).
func TestCompareLoose(t *testing.T) {
	principal, err := model.NewDecimal("100000.00")
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	propio, err := model.NewDecimal("0.30")
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	alternativo, err := model.NewDecimal("1.00")
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	valores := []string{"2.0", "2.5", "3.0"}
	var supuestos []model.Decimal
	for _, v := range valores {
		d, err := model.NewDecimal(v)
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
		supuestos = append(supuestos, d)
	}
	res, err := CompareLoose(CompareLooseInputs{
		Principal:  principal,
		TermMonths: 36, // tres periodos de 12 meses: revisiones en los meses 12 y 24
		OwnSpread:  propio,
		Spread:     alternativo,
		Index:      "irph",
		Rounding:   "none",
	}, supuestos, ahoraFijo())
	if err != nil {
		t.Fatalf("CompareLoose(): %v", err)
	}
	for nombre, bloque := range map[string]CompareBlock{"propio": res.Own, "alternativo": res.Alternative} {
		if len(bloque.Revisions) != 2 {
			t.Fatalf("el bloque %s trae %d revisiones, want 2", nombre, len(bloque.Revisions))
		}
		if bloque.Index.Status != "assumed" || !bloque.Revisions[0].Assumed {
			t.Fatalf("el bloque %s debe ser assumed: %+v", nombre, bloque.Index)
		}
		if bloque.Revisions[0].IndexValue.String() != "2.5" || bloque.Revisions[1].IndexValue.String() != "3.0" {
			t.Fatalf("valores del bloque %s = %s/%s, want 2.5/3.0", nombre,
				bloque.Revisions[0].IndexValue, bloque.Revisions[1].IndexValue)
		}
		if bloque.Revisions[0].RevisionDate != "" || bloque.Revisions[0].ReferenceMonth != "" {
			t.Fatalf("el camino sin préstamo no inventa fechas: %+v", bloque.Revisions[0])
		}
		if bloque.InitialMonths != 12 {
			t.Fatalf("el periodo de partida del bloque %s dura %d, want 12", nombre, bloque.InitialMonths)
		}
	}
	// El primer valor es el tipo de partida de cada bloque con su diferencial.
	if res.Own.InitialRate.String() != "2.30" || res.Alternative.InitialRate.String() != "3.00" {
		t.Fatalf("tipos de partida = %s/%s, want 2.30/3.00", res.Own.InitialRate, res.Alternative.InitialRate)
	}
	if res.Own.Revisions[0].AnnualRatePct.String() != "2.80" || res.Alternative.Revisions[0].AnnualRatePct.String() != "3.50" {
		t.Fatalf("TIN de la revisión 1 = %s/%s, want 2.80/3.50",
			res.Own.Revisions[0].AnnualRatePct, res.Alternative.Revisions[0].AnnualRatePct)
	}
	if res.Own.Spread.String() != "0.30" || res.Alternative.Spread.String() != "1.00" {
		t.Fatalf("spreads = %s/%s, want 0.30/1.00", res.Own.Spread, res.Alternative.Spread)
	}
	if res.Own.Index.ID != "" || res.Alternative.Index.ID != "irph" {
		t.Fatalf("índices = %q/%q, want \"\"/irph", res.Own.Index.ID, res.Alternative.Index.ID)
	}
	if len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "P-06") {
		t.Fatalf("warnings = %v, want el aviso de supuesto (P-06)", res.Warnings)
	}
	if res.Attribution.DataUpdatedAt != "2026-10-15" {
		t.Fatalf("attribution = %s, want la fecha de elaboración", res.Attribution.DataUpdatedAt)
	}

	// Otro número de valores que periodos es un error de uso.
	_, err = CompareLoose(CompareLooseInputs{
		Principal: principal, TermMonths: 36, OwnSpread: propio, Spread: alternativo, Index: "irph", Rounding: "none",
	}, supuestos[:2], ahoraFijo())
	assertAppError(t, err, errs.KindUsage, 2)
}

// TestCompareSinRecomendar: la salida compara, no recomienda (P-08, decisión 3
// de FIN-04): el JSON marshalled no contiene ninguna palabra de asesoramiento.
func TestCompareSinRecomendar(t *testing.T) {
	path := configCon(t, map[string]loan.Loan{"casa": prestamoCompare()})
	rt := runtimeCompare(t)

	res, err := Compare(t.Context(), rt, CompareParams{
		ConfigPath: path,
		LoanID:     "casa",
		Index:      "irph",
		Spread:     "1.00",
		Now:        ahoraFijo(),
	})
	if err != nil {
		t.Fatalf("Compare(): %v", err)
	}
	data, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	lower := strings.ToLower(string(data))
	for _, palabra := range []string{"conviene", "mejor", "recomend", "deberías", "ganador", "winner"} {
		if strings.Contains(lower, palabra) {
			t.Errorf("la salida contiene la palabra prohibida %q:\n%s", palabra, data)
		}
	}
}

// TestCompareIndiceDesconocido: un id de índice que no está en el catálogo es
// not_found/4 antes de tocar la red.
func TestCompareIndiceDesconocido(t *testing.T) {
	path := configCon(t, map[string]loan.Loan{"casa": prestamoCompare()})
	rt := runtimeCompare(t)

	_, err := Compare(t.Context(), rt, CompareParams{
		ConfigPath: path,
		LoanID:     "casa",
		Index:      "inventado",
		Spread:     "1.00",
		Now:        ahoraFijo(),
	})
	assertAppError(t, err, errs.KindNotFound, 4)
}
