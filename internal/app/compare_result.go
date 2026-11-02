package app

import (
	"fmt"
	"time"

	"github.com/aam9063/hipoteca-cli/internal/errs"
	"github.com/aam9063/hipoteca-cli/internal/model"
	"github.com/shopspring/decimal"
)

// ----------------------------------------------------------------------------
// La presentación del resultado de `compare` (FIN-04, REF-05): la ventana
// pedida, el emparejado revisión a revisión con sus diferencias y la
// atribución. Sin ninguna cifra nueva: lo que sale aquí es la misma
// comparación que produce el recorrido de compare_history.go, sólo ordenada
// y recortada para la salida.
// ----------------------------------------------------------------------------

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
