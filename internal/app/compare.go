package app

import (
	"context"
	"fmt"
	"time"

	"github.com/aam9063/hipoteca-cli/internal/bde"
	"github.com/aam9063/hipoteca-cli/internal/errs"
	"github.com/aam9063/hipoteca-cli/internal/finance"
	"github.com/aam9063/hipoteca-cli/internal/model"
	"github.com/shopspring/decimal"
)

// ----------------------------------------------------------------------------
// `compare` (FIN-04): recalcular la historia de un préstamo con otro índice y
// otro diferencial, usando el valor publicado de cada mes de referencia, y
// comparar los dos bloques sin recomendar nada (P-08).
//
// El cálculo es el mismo núcleo (decisión 5 de FIN-04): para cada bloque se
// forma un tramo por revisión con el TIN de la cadena NewRate (índice de SU
// mes de referencia + diferencial → suelo → redondeo de escritura) y el cuadro
// sale de CompoundSchedule, no de un camino paralelo. Si falta un mes del
// índice es index_not_published/4: no se interpola ni se inventa (decisión 2).
// ----------------------------------------------------------------------------

// CompareIndexBlock declara el índice de un bloque (decisión 4): el id y el
// código de su serie, y si sus valores son publicados o supuestos (P-06).
type CompareIndexBlock struct {
	ID     string `json:"id"`
	Code   string `json:"code"`
	Status string `json:"status"` // published | assumed
}

// CompareRevision es una revisión de un bloque: el periodo al que pertenece,
// su fecha y su mes de referencia (vacíos en el camino sin préstamo guardado,
// que no inventa un calendario), el valor del índice usado, el TIN resultante
// y la cuota del periodo.
type CompareRevision struct {
	Period         int           `json:"period"`
	RevisionDate   string        `json:"revision_date,omitempty"`
	ReferenceMonth string        `json:"reference_month,omitempty"`
	IndexValue     model.Decimal `json:"index_value"`
	Assumed        bool          `json:"assumed"`
	AnnualRatePct  model.Decimal `json:"annual_rate_pct"`
	Payment        model.Decimal `json:"payment"`
}

// CompareBlock es uno de los dos bloques simétricos (decisión 3): su índice,
// su diferencial, el periodo de partida, las revisiones con su cuota y los
// totales del cuadro completo.
type CompareBlock struct {
	Index         CompareIndexBlock `json:"index"`
	Spread        model.Decimal     `json:"spread"`
	InitialRate   model.Decimal     `json:"initial_rate_pct"`
	InitialMonths int               `json:"initial_period_months"`
	Revisions     []CompareRevision `json:"revisions"`
	TotalPaid     model.Decimal     `json:"total_paid"`
	TotalInterest model.Decimal     `json:"total_interest"`
}

// CompareDifference es la diferencia por año entre los dos bloques para una
// revisión: las dos cuotas y la diferencia mensual y anual (alternativa −
// propia). No hay adjetivos: sólo cifras (P-08).
type CompareDifference struct {
	Period             int           `json:"period"`
	Year               int           `json:"year,omitempty"`
	PaymentOwn         model.Decimal `json:"payment_own"`
	PaymentAlternative model.Decimal `json:"payment_alternative"`
	DifferenceMonth    model.Decimal `json:"difference_month"`
	DifferenceYear     model.Decimal `json:"difference_year"`
}

// CompareTotalDifference compara un total de los dos bloques con su
// diferencia (alternativa − propia).
type CompareTotalDifference struct {
	Own         model.Decimal `json:"own"`
	Alternative model.Decimal `json:"alternative"`
	Difference  model.Decimal `json:"difference"`
}

// CompareCoverage declara el periodo que la comparación cubre: el primer y
// el último mes de referencia comparados (YYYY-MM). La comparación es sobre
// el pasado: no hay proyección sobre meses sin datos publicados (P-06).
type CompareCoverage struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// CompareResult es la salida de `compare`: los dos bloques, la cobertura del
// periodo comparado, la diferencia por año, los totales comparados, los
// avisos (P-06) y la atribución de datos procesados (P-05).
type CompareResult struct {
	SchemaVersion int                    `json:"schema_version"`
	Loan          string                 `json:"loan,omitempty"`
	From          string                 `json:"from,omitempty"`
	To            string                 `json:"to,omitempty"`
	Coverage      CompareCoverage        `json:"coverage"`
	Own           CompareBlock           `json:"own_index"`
	Alternative   CompareBlock           `json:"alternative_index"`
	Differences   []CompareDifference    `json:"differences"`
	TotalPaid     CompareTotalDifference `json:"total_paid"`
	TotalInterest CompareTotalDifference `json:"total_interest"`
	Warnings      []string               `json:"warnings"`
	Attribution   model.Attribution      `json:"attribution"`
}

// CompareParams son los parámetros de `compare` sobre un préstamo guardado.
// Index y Spread son el índice alternativo y su diferencial; From y To son la
// ventana opcional (YYYY-MM) de las revisiones que se muestran; IndexValues
// sustituye el histórico del índice alternativo por valores supuestos, uno por
// revisión (P-06); Now es el reloj inyectado.
type CompareParams struct {
	ConfigPath  string
	LoanID      string
	Index       string
	Spread      string
	From        string
	To          string
	IndexValues []model.Decimal
	Now         time.Time
}

// CompareLooseInputs son las entradas del camino de `compare` sin préstamo
// guardado: un préstamo hipotético con capital y plazo, un diferencial propio
// y el alternativo, y los valores supuestos del índice — uno por periodo de 12
// meses, el primero es el tipo de partida — que usan los DOS bloques (P-06).
type CompareLooseInputs struct {
	Principal  model.Decimal
	TermMonths int
	OwnSpread  model.Decimal
	Spread     model.Decimal
	Index      string
	Floor      *model.Decimal
	Rounding   string
}

// Compare recalcula la historia del préstamo guardado con cada índice: el
// propio de la escritura y el alternativo pedido, y compara los dos bloques.
// Cada revisión usa el valor publicado de SU mes de referencia (decisión 1) y
// la cuota de cada tramo sale del núcleo con el capital pendiente y los meses
// restantes (decisión 5). La salida compara; no recomienda (decisión 3).
func Compare(ctx context.Context, rt *Runtime, p CompareParams) (CompareResult, error) {
	cfg, err := loadConfigAt(p.ConfigPath)
	if err != nil {
		return CompareResult{}, err
	}
	if len(cfg.Loans) == 0 {
		return CompareResult{}, ErrLoanIncomplete("compare")
	}
	id, err := resolveLoanID(cfg.Loans, p.LoanID)
	if err != nil {
		return CompareResult{}, err
	}
	l := cfg.Loans[id]
	if err := l.Validate(); err != nil {
		return CompareResult{}, loanError(id, err)
	}
	if p.Index == "" {
		return CompareResult{}, errs.New(errs.KindUsage,
			"falta --index: indica el índice alternativo con el que comparar",
			"ejemplo: hipoteca compare "+id+" --index irph --spread 1.00")
	}
	if p.Spread == "" {
		return CompareResult{}, errs.New(errs.KindUsage,
			"falta --spread: indica el diferencial del índice alternativo",
			"ejemplo: hipoteca compare "+id+" --index "+p.Index+" --spread 1.00")
	}
	altSpread, err := model.NewDecimal(p.Spread)
	if err != nil {
		return CompareResult{}, errs.New(errs.KindUsage,
			fmt.Sprintf("el diferencial %q no es un decimal válido", p.Spread),
			"usa el diferencial en tanto por ciento, p. ej. 1.00")
	}
	ownIdx, err := rt.Catalog.Lookup(l.Index)
	if err != nil {
		return CompareResult{}, err
	}
	altIdx, err := rt.Catalog.Lookup(p.Index)
	if err != nil {
		return CompareResult{}, err
	}
	start, err := time.Parse("2006-01-02", l.StartDate)
	if err != nil {
		return CompareResult{}, errs.New(errs.KindUsage,
			fmt.Sprintf("la fecha de inicio %q no es una fecha YYYY-MM-DD", l.StartDate), "")
	}
	from, to, err := parseVentana(p.From, p.To)
	if err != nil {
		return CompareResult{}, err
	}

	// Los tramos del préstamo: el tramo inicial y uno por revisión, cada uno
	// hasta la siguiente revisión o el fin del plazo. El reparto exacto se
	// hace más abajo, sobre las revisiones comparables (hasta el último mes
	// publicado en los dos índices).
	boundaries := revisionMonthFromStart(l)
	offset := *l.ReferenceOffsetMonths
	floor, err := floorOf(l)
	if err != nil {
		return CompareResult{}, loanError(id, err)
	}
	initialRate, err := model.NewDecimal(l.InitialRate)
	if err != nil {
		return CompareResult{}, loanError(id, err)
	}
	principal, err := model.NewDecimal(l.OriginalPrincipal)
	if err != nil {
		return CompareResult{}, loanError(id, err)
	}
	spread, err := model.NewDecimal(l.Spread)
	if err != nil {
		return CompareResult{}, loanError(id, err)
	}

	// Histórico del índice propio: siempre real. Si falta un mes de los que
	// el propio préstamo necesita, es index_not_published: la comparación no
	// puede inventarse.
	ownSeries, err := rt.Source.History(ctx, l.Index, bde.RangoMAX)
	if err != nil {
		return CompareResult{}, err
	}

	// Histórico del alternativo: real, o supuesto con --index-values (P-06).
	var altSeries *bde.Series
	if len(p.IndexValues) == 0 {
		s, err := rt.Source.History(ctx, p.Index, bde.RangoMAX)
		if err != nil {
			return CompareResult{}, err
		}
		altSeries = &s
	}

	// La comparación es sobre el pasado: se compara hasta la última revisión
	// cuyo mes de referencia está publicado en los DOS índices (con valores
	// supuestos, hasta el último publicado del índice propio). Ninguna
	// revisión posterior se consulta, calcula o muestra: el futuro no se
	// estima (P-06).
	ownLimit := lastMonthDe(ownSeries)
	refLimit := ownLimit
	if altSeries != nil {
		refLimit = lastMonthComunDe(ownSeries, *altSeries)
	}
	if !to.IsZero() {
		if monthKeyDe(to) > monthKeyDe(refLimit.AddDate(0, offset, 0)) {
			return CompareResult{}, errs.New(errs.KindUsage,
				"la ventana pide meses posteriores a los publicados: la comparación sólo cubre los meses publicados por el Banco de España",
				"baja --to a un mes dentro del periodo publicado")
		}
	}
	comparables := make([]int, 0, len(boundaries))
	for _, b := range boundaries {
		if referenceMonthDe(start, b, l.PaymentDay, offset).After(refLimit) {
			break
		}
		if !to.IsZero() && monthKeyDe(addMonths(start, b, l.PaymentDay)) > monthKeyDe(to) {
			break
		}
		comparables = append(comparables, b)
	}
	if len(comparables) == 0 {
		return CompareResult{}, errs.New(errs.KindUsage,
			"no hay ninguna revisión cuyo mes de referencia esté publicado en los dos índices: no hay nada que comparar", "")
	}
	// El recálculo termina en la siguiente revisión tras la última comparada
	// (o el fin del plazo): los meses posteriores dependen de índices sin
	// publicar y no se proyectan.
	cutEnd := l.TermMonths
	if len(comparables) < len(boundaries) {
		cutEnd = boundaries[len(comparables)]
	}
	lens := compareLens(comparables, cutEnd)
	// Valores de cada revisión comparada para cada bloque: el índice propio
	// se lee de su serie; el alternativo de la suya o de los supuestos. Cada
	// revisión usa el índice de SU mes de referencia (decisión 1 de FIN-04).
	if altSeries == nil && len(p.IndexValues) != len(comparables) {
		return CompareResult{}, errs.New(errs.KindUsage,
			fmt.Sprintf("con --index-values se esperaban %d valores, uno por revisión comparada, y hay %d", len(comparables), len(p.IndexValues)), "")
	}
	ownValues := make([]decimal.Decimal, len(comparables))
	for i, b := range comparables {
		obs, ok := observationAt(ownSeries, referenceMonthDe(start, b, l.PaymentDay, offset))
		if !ok {
			return CompareResult{}, indexNotPublishedDe(l.Index, referenceMonthDe(start, b, l.PaymentDay, offset), "")
		}
		ownValues[i] = obs.Value.Decimal()
	}
	altValues := make([]decimal.Decimal, len(comparables))
	for i, b := range comparables {
		if altSeries == nil {
			altValues[i] = p.IndexValues[i].Decimal()
			continue
		}
		refMonth := referenceMonthDe(start, b, l.PaymentDay, offset)
		obs, ok := observationAt(*altSeries, refMonth)
		if !ok {
			return CompareResult{}, indexNotPublishedDe(p.Index, refMonth,
				fmt.Sprintf("usa --index-values <valores> para suponer los valores del índice alternativo dentro del periodo publicado, p. ej. \"hipoteca compare %s --index %s --spread %s --index-values 2.0,2.2\"", id, p.Index, p.Spread))
		}
		altValues[i] = obs.Value.Decimal()
	}

	// Los dos bloques con el mismo núcleo, hasta el fin del periodo
	// publicado.
	ownBlock, _, ownEnd, err := compareBlockFrom(principal.Decimal(), cutEnd, lens, initialRate.Decimal(),
		ownValues, spread.Decimal(), floor, finance.RateRounding(l.RateRounding),
		CompareIndexBlock{ID: ownIdx.ID, Code: ownIdx.Code, Status: "published"},
		comparables, func(b int) (string, string) {
			revDate := addMonths(start, b, l.PaymentDay)
			return revDate.Format("2006-01-02"), referenceMonthDe(start, b, l.PaymentDay, offset).Format("2006-01")
		}, &ownSeries)
	if err != nil {
		return CompareResult{}, err
	}
	altBlock, _, altEnd, err := compareBlockFrom(principal.Decimal(), cutEnd, lens, initialRate.Decimal(),
		altValues, altSpread.Decimal(), floor, finance.RateRounding(l.RateRounding),
		CompareIndexBlock{ID: altIdx.ID, Code: altIdx.Code, Status: indexStatus(altSeries != nil)},
		comparables, func(b int) (string, string) {
			revDate := addMonths(start, b, l.PaymentDay)
			return revDate.Format("2006-01-02"), referenceMonthDe(start, b, l.PaymentDay, offset).Format("2006-01")
		}, altSeries)
	if err != nil {
		return CompareResult{}, err
	}

	// Ventana: --from deja fuera de la salida las revisiones anteriores (el
	// recálculo siempre empieza en el inicio del préstamo: el capital de cada
	// tramo lo exige); --to ya acotó el cálculo arriba.
	included := indicesEnVentana(comparables, start, l.PaymentDay, from, to)
	if len(included) == 0 {
		return CompareResult{}, errs.New(errs.KindUsage,
			"la ventana no cubre ninguna revisión publicada", "")
	}
	diffs := diferencias(ownBlock.Revisions, altBlock.Revisions, comparables, start, l.PaymentDay, included)
	ownBlock.Revisions = seleccionarRevisiones(ownBlock.Revisions, included)
	altBlock.Revisions = seleccionarRevisiones(altBlock.Revisions, included)

	// Cobertura del periodo comparado y aviso cuando el plazo sigue más
	// allá: sin proyección (P-06).
	cobertura := CompareCoverage{
		From: referenceMonthDe(start, comparables[0], l.PaymentDay, offset).Format("2006-01"),
		To:   referenceMonthDe(start, comparables[len(comparables)-1], l.PaymentDay, offset).Format("2006-01"),
	}
	warnings := []string{}
	if cutEnd < l.TermMonths {
		warnings = append(warnings,
			fmt.Sprintf("la comparación cubre sólo el periodo publicado por el Banco de España (meses de referencia de %s a %s): el resto del plazo no se proyecta (P-06)", cobertura.From, cobertura.To))
	}
	if altSeries == nil {
		warnings = append(warnings,
			"los valores del índice alternativo son supuestos tuyos con --index-values: no son datos publicados por el Banco de España (P-06)")
	}
	attr, err := atribucionComparada(p.Now, ownEnd, altEnd)
	if err != nil {
		return CompareResult{}, err
	}
	return CompareResult{
		SchemaVersion: model.SchemaVersion,
		Loan:          id,
		From:          p.From,
		To:            p.To,
		Coverage:      cobertura,
		Own:           ownBlock,
		Alternative:   altBlock,
		Differences:   diffs,
		TotalPaid: CompareTotalDifference{
			Own:         ownBlock.TotalPaid,
			Alternative: altBlock.TotalPaid,
			Difference:  model.NewMoney(altBlock.TotalPaid.Decimal().Sub(ownBlock.TotalPaid.Decimal())),
		},
		TotalInterest: CompareTotalDifference{
			Own:         ownBlock.TotalInterest,
			Alternative: altBlock.TotalInterest,
			Difference:  model.NewMoney(altBlock.TotalInterest.Decimal().Sub(ownBlock.TotalInterest.Decimal())),
		},
		Warnings:    warnings,
		Attribution: attr,
	}, nil
}

// lastMonthDe devuelve el último mes publicado de una serie (día 1, 00:00
// UTC); cero si la serie no trae observaciones.
func lastMonthDe(s bde.Series) time.Time {
	if len(s.Observations) == 0 {
		return time.Time{}
	}
	return s.Observations[len(s.Observations)-1].Month
}

// lastMonthComunDe devuelve el último mes publicado en las DOS series: el
// más reciente presente en ambas. Es el límite de la comparación: más allá
// no hay datos de los dos índices y nada se proyecta.
func lastMonthComunDe(a, b bde.Series) time.Time {
	propios := make(map[string]bool, len(a.Observations))
	for _, o := range a.Observations {
		propios[monthString(o.Month)] = true
	}
	for i := len(b.Observations) - 1; i >= 0; i-- {
		if propios[monthString(b.Observations[i].Month)] {
			return b.Observations[i].Month
		}
	}
	return time.Time{}
}

// monthKeyDe reduce un tiempo a su clave de mes (año*12+mes) para comparar
// meses con independencia del día.
func monthKeyDe(t time.Time) int {
	return t.Year()*12 + int(t.Month())
}

// CompareLoose resuelve `compare` sin préstamo guardado: un préstamo
// hipotético con el capital y el plazo dados, en tramos anuales con los
// valores supuestos — uno por periodo de 12 meses; el primero es el tipo de
// partida — y los dos diferenciales. Nada se consulta: todos los valores son
// supuestos y van marcados como tales (P-06), y la atribución data la
// elaboración. La salida compara; no recomienda (P-08).
func CompareLoose(in CompareLooseInputs, indexValues []model.Decimal, now time.Time) (CompareResult, error) {
	if in.TermMonths < 1 {
		return CompareResult{}, errs.New(errs.KindUsage, "el plazo debe ser de al menos un mes", "")
	}
	if in.Principal.Decimal().Sign() <= 0 {
		return CompareResult{}, errs.New(errs.KindUsage, "el capital debe ser mayor que cero", "")
	}
	// Tramos anuales: el último toma los meses que quedan.
	var lens []int
	for quedan := in.TermMonths; quedan > 0; quedan -= 12 {
		if quedan > 12 {
			lens = append(lens, 12)
		} else {
			lens = append(lens, quedan)
		}
	}
	if len(indexValues) != len(lens) {
		return CompareResult{}, errs.New(errs.KindUsage,
			fmt.Sprintf("con --index-values se esperaban %d valores, uno por periodo de 12 meses, y hay %d", len(lens), len(indexValues)), "")
	}
	values := make([]decimal.Decimal, len(indexValues))
	for i, v := range indexValues {
		values[i] = v.Decimal()
	}
	var floor *decimal.Decimal
	if in.Floor != nil {
		f := in.Floor.Decimal()
		floor = &f
	}
	// El periodo de partida usa el primer valor pasado por la misma cadena;
	// las revisiones, el resto. Las revisiones caen al acabar cada tramo.
	initialRate, err := finance.NewRate(values[0], in.OwnSpread.Decimal(), floor, finance.RateRounding(in.Rounding))
	if err != nil {
		return CompareResult{}, scenarioUsageError(err)
	}
	altInitial, err := finance.NewRate(values[0], in.Spread.Decimal(), floor, finance.RateRounding(in.Rounding))
	if err != nil {
		return CompareResult{}, scenarioUsageError(err)
	}
	var boundaries []int
	for i := 0; i < len(lens)-1; i++ {
		boundaries = append(boundaries, suma(lens[:i+1]))
	}
	ownBlock, _, _, err := compareBlockFrom(in.Principal.Decimal(), in.TermMonths, lens, initialRate.Final,
		values[1:], in.OwnSpread.Decimal(), floor, finance.RateRounding(in.Rounding),
		CompareIndexBlock{Status: "assumed"}, boundaries, nil, nil)
	if err != nil {
		return CompareResult{}, err
	}
	altBlock, _, _, err := compareBlockFrom(in.Principal.Decimal(), in.TermMonths, lens, altInitial.Final,
		values[1:], in.Spread.Decimal(), floor, finance.RateRounding(in.Rounding),
		CompareIndexBlock{ID: in.Index, Status: "assumed"}, boundaries, nil, nil)
	if err != nil {
		return CompareResult{}, err
	}
	attr, err := model.ProcessedAttribution(now.UTC().Format("2006-01-02"))
	if err != nil {
		return CompareResult{}, errs.New(errs.KindInternal, err.Error(), "")
	}
	todos := make([]int, len(boundaries))
	for i := range boundaries {
		todos[i] = i
	}
	return CompareResult{
		SchemaVersion: model.SchemaVersion,
		Own:           ownBlock,
		Alternative:   altBlock,
		Differences:   diferencias(ownBlock.Revisions, altBlock.Revisions, boundaries, time.Time{}, 0, todos),
		TotalPaid: CompareTotalDifference{
			Own:         ownBlock.TotalPaid,
			Alternative: altBlock.TotalPaid,
			Difference:  model.NewMoney(altBlock.TotalPaid.Decimal().Sub(ownBlock.TotalPaid.Decimal())),
		},
		TotalInterest: CompareTotalDifference{
			Own:         ownBlock.TotalInterest,
			Alternative: altBlock.TotalInterest,
			Difference:  model.NewMoney(altBlock.TotalInterest.Decimal().Sub(ownBlock.TotalInterest.Decimal())),
		},
		Warnings: []string{
			"los valores de los índices son supuestos tuyos con --index-values: no son datos publicados por el Banco de España (P-06)",
		},
		Attribution: attr,
	}, nil
}

// compareBlockFrom genera un bloque completo con el núcleo: el periodo de
// partida al tipo dado (en el camino con préstamo es el tipo inicial de la
// escritura; en el camino suelto, el primer valor supuesto) y un tramo por
// revisión con el TIN de su valor de índice pasado por la cadena NewRate
// (decisión 5 de FIN-04). Devuelve también la fecha de actualización de la
// serie, si la hay. describe, si no es nil, da la fecha y el mes de referencia
// de cada revisión (camino con préstamo; el suelto no inventa calendario).
func compareBlockFrom(principal decimal.Decimal, term int, lens []int, initialRate decimal.Decimal,
	values []decimal.Decimal, spread decimal.Decimal, floor *decimal.Decimal, rounding finance.RateRounding,
	index CompareIndexBlock, boundaries []int, describe func(int) (string, string), series *bde.Series,
) (CompareBlock, []decimal.Decimal, *time.Time, error) {
	block := CompareBlock{
		Index:         index,
		Spread:        model.FromDecimal(spread),
		InitialRate:   model.FromDecimal(initialRate),
		InitialMonths: lens[0],
		Revisions:     make([]CompareRevision, 0, len(values)),
	}
	rates := make([]decimal.Decimal, len(values))
	for i, v := range values {
		rate, err := finance.NewRate(v, spread, floor, rounding)
		if err != nil {
			return CompareBlock{}, nil, nil, scenarioUsageError(err)
		}
		rates[i] = rate.Final
		rev := CompareRevision{
			Period:        i + 1, // la revisión i+1 (el tramo 1 es el periodo de partida)
			IndexValue:    model.FromDecimal(v),
			Assumed:       series == nil,
			AnnualRatePct: model.FromDecimal(rate.Final),
		}
		if describe != nil {
			rev.RevisionDate, rev.ReferenceMonth = describe(boundaries[i])
		}
		block.Revisions = append(block.Revisions, rev)
	}

	// Tramos: el de partida con el tipo inicial y uno por revisión; el cuadro
	// es el compuesto del núcleo, no un camino paralelo (decisión 5).
	periods := make([]finance.Period, 0, len(lens))
	periods = append(periods, finance.Period{Rate: initialRate, Months: lens[0]})
	for i := range values {
		periods = append(periods, finance.Period{Rate: rates[i], Months: lens[i+1]})
	}
	sched, err := finance.CompoundSchedule(
		finance.Loan{Principal: principal, Months: term},
		periods)
	if err != nil {
		return CompareBlock{}, nil, nil, scenarioUsageError(err)
	}
	if len(sched.Rows) < term {
		return CompareBlock{}, nil, nil, errs.New(errs.KindInternal,
			"el cuadro compuesto se cerró antes de agotar el plazo", "")
	}
	// El pago de cada tramo es la primera fila del tramo; la cuota de la
	// revisión i es la del tramo i+1.
	payments := make([]decimal.Decimal, len(lens))
	offset := 0
	for i, n := range lens {
		payments[i] = sched.Rows[offset].Payment
		offset += n
	}
	for i := range block.Revisions {
		block.Revisions[i].Payment = model.NewMoney(payments[i+1])
	}
	block.TotalPaid = model.NewMoney(sched.TotalPaid)
	block.TotalInterest = model.NewMoney(sched.TotalInterest)
	var end *time.Time
	if series != nil {
		e := series.End
		end = &e
	}
	return block, payments, end, nil
}

// referenceMonthDe es el mes de referencia de la revisión que cae en el mes b
// desde el inicio del préstamo: la fecha de revisión menos el offset.
func referenceMonthDe(start time.Time, elapsed, paymentDay, offset int) time.Time {
	return finance.ReferenceMonth(addMonths(start, elapsed, paymentDay), offset)
}

// indexNotPublishedDe construye el error del contrato para un mes de
// referencia sin valor publicado (decisión 2 de FIN-04): nunca se inventa.
func indexNotPublishedDe(index string, refMonth time.Time, hint string) error {
	return errs.New(errs.KindIndexNotPublished,
		fmt.Sprintf("el índice %s de %s aún no está publicado", index, refMonth.Format("2006-01")), hint)
}

// suma totaliza una lista de enteros.
func suma(xs []int) int {
	n := 0
	for _, x := range xs {
		n += x
	}
	return n
}

// compareLens reparte el plazo en tramos: uno hasta la primera revisión, uno
// entre revisiones y el resto hasta el fin del plazo.
func compareLens(boundaries []int, term int) []int {
	lens := make([]int, 0, len(boundaries)+1)
	prev := 0
	for _, b := range boundaries {
		lens = append(lens, b-prev)
		prev = b
	}
	lens = append(lens, term-prev)
	return lens
}

// indexStatus nombra el estado del índice de un bloque.
func indexStatus(published bool) string {
	if published {
		return "published"
	}
	return "assumed"
}

// parseVentana valida --from/--to: meses YYYY-MM opcionales, from no posterior
// a to.
func parseVentana(from, to string) (time.Time, time.Time, error) {
	zero := time.Time{}
	parse := func(s string) (time.Time, error) {
		if s == "" {
			return zero, nil
		}
		t, err := time.Parse("2006-01", s)
		if err != nil {
			return zero, errs.New(errs.KindUsage,
				fmt.Sprintf("la ventana %q no es un mes YYYY-MM", s),
				"usa un mes YYYY-MM, p. ej. --from 2025-01 --to 2026-01")
		}
		return t, nil
	}
	f, err := parse(from)
	if err != nil {
		return zero, zero, err
	}
	t, err := parse(to)
	if err != nil {
		return zero, zero, err
	}
	if !f.IsZero() && !t.IsZero() && f.After(t) {
		return zero, zero, errs.New(errs.KindUsage,
			"la ventana está al revés: --from es posterior a --to", "")
	}
	return f, t, nil
}

// indicesEnVentana devuelve los índices de las revisiones cuya fecha cae en la
// ventana; sin ventana, todas. La numeración de periodo es la global.
func indicesEnVentana(boundaries []int, start time.Time, paymentDay int, from, to time.Time) []int {
	included := make([]int, 0, len(boundaries))
	for i, b := range boundaries {
		if from.IsZero() && to.IsZero() {
			included = append(included, i)
			continue
		}
		revMonth := addMonths(start, b, paymentDay)
		if !from.IsZero() && revMonth.Before(from) {
			continue
		}
		if !to.IsZero() && revMonth.After(to) {
			continue
		}
		included = append(included, i)
	}
	return included
}

// seleccionarRevisiones deja las revisiones cuyos índices están incluidos.
func seleccionarRevisiones(revs []CompareRevision, included []int) []CompareRevision {
	if len(included) == len(revs) {
		return revs
	}
	filtradas := make([]CompareRevision, 0, len(included))
	for _, i := range included {
		filtradas = append(filtradas, revs[i])
	}
	return filtradas
}

// diferencias empareja los bloques revisión a revisión: las dos cuotas y la
// diferencia mensual y anual (alternativa − propia), sin ningún juicio (P-08).
// El año sale de la fecha de revisión; el camino sin préstamo no lo inventa.
func diferencias(ownRevs, altRevs []CompareRevision, boundaries []int, start time.Time, paymentDay int, included []int) []CompareDifference {
	diffs := make([]CompareDifference, 0, len(included))
	for _, i := range included {
		ownRev, altRev := ownRevs[i], altRevs[i]
		d := CompareDifference{
			Period:             ownRev.Period,
			PaymentOwn:         ownRev.Payment,
			PaymentAlternative: altRev.Payment,
			DifferenceMonth:    model.NewMoney(altRev.Payment.Decimal().Sub(ownRev.Payment.Decimal())),
		}
		d.DifferenceYear = model.NewMoney(d.DifferenceMonth.Decimal().Mul(decimal.NewFromInt(12)))
		if !start.IsZero() {
			d.Year = addMonths(start, boundaries[i], paymentDay).Year()
		}
		diffs = append(diffs, d)
	}
	return diffs
}

// atribucionComparada construye la atribución de datos procesados (P-05)
// fechada a la última actualización entre las series usadas; si ningún bloque
// consultó datos del Banco, data la elaboración del escenario.
func atribucionComparada(now time.Time, ends ...*time.Time) (model.Attribution, error) {
	var last time.Time
	for _, e := range ends {
		if e != nil && e.After(last) {
			last = *e
		}
	}
	updatedAt := last.Format("2006-01-02")
	if last.IsZero() {
		updatedAt = now.UTC().Format("2006-01-02")
	}
	return model.ProcessedAttribution(updatedAt)
}
