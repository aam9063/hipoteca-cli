package app

import (
	"fmt"
	"time"

	"github.com/aam9063/hipoteca-cli/internal/bde"
	"github.com/aam9063/hipoteca-cli/internal/errs"
	"github.com/aam9063/hipoteca-cli/internal/finance"
	"github.com/aam9063/hipoteca-cli/internal/model"
	"github.com/shopspring/decimal"
)

// ----------------------------------------------------------------------------
// El recorrido de históricos de `compare` (FIN-04, REF-05): límites de las
// series, meses de referencia, tramos y la construcción de cada bloque con el
// núcleo (finance.CompoundSchedule). La orquestación está en compare.go y la
// presentación del resultado, en compare_result.go; aquí no hay formato de
// salida, sólo el pase por los datos publicados del Banco.
// ----------------------------------------------------------------------------

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
