package app

import (
	"errors"
	"fmt"

	"github.com/aam9063/hipoteca-cli/internal/errs"
	"github.com/aam9063/hipoteca-cli/internal/finance"
	"github.com/aam9063/hipoteca-cli/internal/model"
)

// catalogVerifiedAt es la fecha de verificación del catálogo de series
// (internal/bde/series.yaml, campo verified_at): la única fecha real que este
// binario asocia a los datos originales. `calc` no consulta la red — sus
// entradas las da el usuario — así que su atribución de datos procesados data
// ese origen estático; el test TestCatalogVerifiedAtCoincideConSeriesYAML
// impide que la constante se desvíe del catálogo.
const catalogVerifiedAt = "2026-10-28"

// CalcInputs son las entradas del cálculo, devueltas en la salida tal cual
// (decisión 2 de CMD-01): la respuesta incluye sus entradas.
type CalcInputs struct {
	Principal     model.Decimal `json:"principal"`
	AnnualRatePct model.Decimal `json:"annual_rate_pct"`
	TermMonths    int           `json:"term_months"`
}

// CalcResult es la salida de `calc`: cifras y nada más (decisión 5 de
// CMD-01), con el cuadro de amortización reducido a sus totales.
type CalcResult struct {
	SchemaVersion int               `json:"schema_version"`
	Inputs        CalcInputs        `json:"inputs"`
	Payment       model.Decimal     `json:"payment"`
	TotalInterest model.Decimal     `json:"total_interest"`
	TotalPaid     model.Decimal     `json:"total_paid"`
	Attribution   model.Attribution `json:"attribution"`
}

// Calc resuelve el cálculo suelto del sistema francés con las convenciones de
// docs/calculos.md a través de internal/finance, sin préstamo guardado y sin
// red. No devuelve ningún consejo: sólo cifras.
func Calc(principal, annualRatePct model.Decimal, months int) (CalcResult, error) {
	schedule, err := finance.Amortization(finance.Loan{
		Principal:  principal.Decimal(),
		AnnualRate: annualRatePct.Decimal(),
		Months:     months,
	})
	if err != nil {
		return CalcResult{}, calcUsageError(err)
	}
	attr, err := model.ProcessedAttribution(catalogVerifiedAt)
	if err != nil {
		// La fecha es una constante validada por test: nunca ocurre.
		return CalcResult{}, errs.New(errs.KindInternal, err.Error(), "")
	}
	return CalcResult{
		SchemaVersion: model.SchemaVersion,
		Inputs: CalcInputs{
			Principal:     principal,
			AnnualRatePct: annualRatePct,
			TermMonths:    months,
		},
		Payment:       model.FromDecimal(schedule.Rows[0].Payment),
		TotalInterest: model.FromDecimal(schedule.TotalInterest),
		TotalPaid:     model.FromDecimal(schedule.TotalPaid),
		Attribution:   attr,
	}, nil
}

// calcUsageError traduce las validaciones de finance (errores centinela sin
// sobre) a errores de uso del contrato: entradas inválidas del usuario son
// usage/2, no internal/1. Un error que finance no clasifica sube tal cual.
func calcUsageError(err error) error {
	var msg string
	switch {
	case errors.Is(err, finance.ErrNegativePrincipal):
		msg = "el capital no puede ser negativo"
	case errors.Is(err, finance.ErrZeroMonths), errors.Is(err, finance.ErrNegativeMonths):
		msg = "el plazo debe ser de al menos un mes"
	case errors.Is(err, finance.ErrUnpayableSchedule):
		msg = fmt.Sprintf("con estas entradas no hay un plan de amortización que representar: %v", err)
	default:
		return err
	}
	return errs.New(errs.KindUsage, msg, "revisa --principal, --rate y --months")
}
