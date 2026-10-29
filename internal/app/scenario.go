package app

import (
	"errors"
	"fmt"
	"time"

	"github.com/aam9063/hipoteca-cli/internal/errs"
	"github.com/aam9063/hipoteca-cli/internal/finance"
	"github.com/aam9063/hipoteca-cli/internal/model"
	"github.com/shopspring/decimal"
)

// ScenarioInputs son las entradas de `scenario`, devueltas en la salida tal
// cual: el préstamo suelto (capital y meses restantes) y la cadena de revisión
// opcional que se aplica a cada valor del índice. Sin diferencial, suelo ni
// redondeo, el TIN de cada valor es el valor mismo: la cadena NewRate con el
// diferencial 0, sin suelo y sin redondeo es la identidad.
type ScenarioInputs struct {
	// Loan es el id del préstamo guardado del que salieron capital, plazo,
	// diferencial, suelo y redondeo; vacío en el camino de flags sueltas.
	Loan       string         `json:"loan,omitempty"`
	Principal  model.Decimal  `json:"principal"`
	TermMonths int            `json:"term_months"`
	Spread     model.Decimal  `json:"spread"`
	Floor      *model.Decimal `json:"floor,omitempty"`
	Rounding   string         `json:"rate_rounding"`
}

// ScenarioValue es el resultado para UN valor de índice asumido: el valor
// ecoado tal cual (la escala que dio el usuario se conserva: "1.50" es
// "1.50"), la marca assumed (P-06), el TIN resultante y la cuota con el coste
// total sobre los meses restantes. Nada aquí recomienda (P-08).
type ScenarioValue struct {
	IndexValue    model.Decimal `json:"index_value"`
	Assumed       bool          `json:"assumed"`
	AnnualRatePct model.Decimal `json:"annual_rate_pct"`
	Payment       model.Decimal `json:"payment"`
	TotalInterest model.Decimal `json:"total_interest"`
	TotalPaid     model.Decimal `json:"total_paid"`
}

// ScenarioResult es la salida de `scenario`: las entradas, un bloque por cada
// valor pedido y la atribución de datos procesados (P-05).
type ScenarioResult struct {
	SchemaVersion int               `json:"schema_version"`
	Inputs        ScenarioInputs    `json:"inputs"`
	Values        []ScenarioValue   `json:"values"`
	Attribution   model.Attribution `json:"attribution"`
}

// Scenario resuelve el escenario de cuota para cada valor de índice dado
// (CMD-02.4): TIN vía la cadena NewRate existente (índice + diferencial →
// suelo → redondeo de escritura, convención 1.5) y cuota y coste total con el
// núcleo sobre los meses restantes. Los valores son supuestos del usuario:
// cada bloque se marca assumed porque no viene de la API (P-06, decisión 4 de
// CMD-02). No devuelve ningún consejo: sólo cifras.
func Scenario(in ScenarioInputs, indexValues []model.Decimal, now time.Time) (ScenarioResult, error) {
	if len(indexValues) == 0 {
		return ScenarioResult{}, errs.New(errs.KindUsage,
			"no se ha indicado ningún valor de índice",
			"pasa los valores separados por comas, p. ej. --index-values 1.5,2,2.5,3")
	}
	attr, err := model.ProcessedAttribution(now.UTC().Format("2006-01-02"))
	if err != nil {
		// La fecha sale de time.UTC().Format: nunca ocurre.
		return ScenarioResult{}, errs.New(errs.KindInternal, err.Error(), "")
	}

	var floor *decimal.Decimal
	if in.Floor != nil {
		f := in.Floor.Decimal()
		floor = &f
	}
	values := make([]ScenarioValue, 0, len(indexValues))
	for _, v := range indexValues {
		rate, err := finance.NewRate(v.Decimal(), in.Spread.Decimal(), floor,
			finance.RateRounding(in.Rounding))
		if err != nil {
			return ScenarioResult{}, scenarioUsageError(err)
		}
		schedule, err := finance.Amortization(finance.Loan{
			Principal:  in.Principal.Decimal(),
			AnnualRate: rate.Final,
			Months:     in.TermMonths,
		})
		if err != nil {
			return ScenarioResult{}, scenarioUsageError(err)
		}
		values = append(values, ScenarioValue{
			IndexValue: v,
			Assumed:    true,
			// La cuota y los totales son dinero: siempre a dos decimales en el
			// JSON, aunque sean 0.00. El TIN es un tipo: conserva su escala.
			AnnualRatePct: model.FromDecimal(rate.Final),
			Payment:       model.NewMoney(schedule.Rows[0].Payment),
			TotalInterest: model.NewMoney(schedule.TotalInterest),
			TotalPaid:     model.NewMoney(schedule.TotalPaid),
		})
	}
	return ScenarioResult{
		SchemaVersion: model.SchemaVersion,
		Inputs:        in,
		Values:        values,
		Attribution:   attr,
	}, nil
}

// scenarioUsageError traduce las validaciones de finance a errores de uso del
// contrato: entradas inválidas del usuario son usage/2, no internal/1.
func scenarioUsageError(err error) error {
	var msg string
	switch {
	case errors.Is(err, finance.ErrUnsupportedRateRounding):
		// La lista de redondeos válidos es parte de la ayuda al usuario; el
		// mensaje no incrusta la cadena del error interno.
		msg = "el redondeo de escritura no está soportado (usa none, 1/8, 1/4 o 0.05)"
	case errors.Is(err, finance.ErrNegativePrincipal):
		msg = "el capital no puede ser negativo"
	case errors.Is(err, finance.ErrZeroMonths), errors.Is(err, finance.ErrNegativeMonths):
		msg = "el plazo debe ser de al menos un mes"
	case errors.Is(err, finance.ErrUnpayableSchedule):
		msg = fmt.Sprintf("con estas entradas no hay un plan de amortización que representar: %s",
			sinPrefijoFinance(err.Error()))
	default:
		return err
	}
	return errs.New(errs.KindUsage, msg, "revisa --principal, --months, --spread, --floor y --rounding")
}
