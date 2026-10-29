package app

import (
	"errors"
	"fmt"
	"time"

	"github.com/aam9063/hipoteca-cli/internal/errs"
	"github.com/aam9063/hipoteca-cli/internal/finance"
	"github.com/aam9063/hipoteca-cli/internal/model"
)

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
//
// `now` fecha la atribución de datos procesados (P-05): su data_updated_at es
// la fecha de elaboración del resultado — el día de la ejecución, en UTC — y
// no la de ningún dato: este cálculo no consulta ningún dato del Banco de
// España, sus entradas las da el usuario.
func Calc(principal, annualRatePct model.Decimal, months int, now time.Time) (CalcResult, error) {
	schedule, err := finance.Amortization(finance.Loan{
		Principal:  principal.Decimal(),
		AnnualRate: annualRatePct.Decimal(),
		Months:     months,
	})
	if err != nil {
		return CalcResult{}, calcUsageError(err)
	}
	attr, err := model.ProcessedAttribution(now.UTC().Format("2006-01-02"))
	if err != nil {
		// La fecha sale de time.UTC().Format: nunca ocurre.
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
