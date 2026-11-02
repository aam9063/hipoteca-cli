package loan

import (
	"fmt"
	"time"

	"github.com/aam9063/hipoteca-cli/internal/config"
	"github.com/aam9063/hipoteca-cli/internal/errs"
	"github.com/aam9063/hipoteca-cli/internal/finance"
	"github.com/aam9063/hipoteca-cli/internal/model"
)

// Los tres tipos de préstamo del contrato (docs/ARRANQUE.md §4.1).
const (
	TypeVariable = "variable"
	TypeMixta    = "mixta"
	TypeFija     = "fija"
)

// Las dos convenciones de cálculo que el núcleo soporta hoy (docs/calculos.md
// §1): una sola cada una; cualquier otro valor es un error de uso, no una
// convención supuesta.
const (
	InterestConventionMonthlyNominal = "monthly-nominal"
	PaymentRoundingHalfUpCents       = "half-up-cents"
)

// IncompleteError es el centinela de la validación estricta (decisión 1 de
// LOAN-01): el préstamo no declara un dato que depende de la escritura y la
// CLI no lo supone. Field es el nombre exacto del campo en la config
// (sync.<campo> para los campos de la sincronización), que la capa de
// aplicación convierte en loan_incomplete/2 con un hint que lo nombra.
type IncompleteError struct {
	Field string
}

// FieldEarlyRepaymentFeePct es el nombre exacto del campo de la comisión de
// amortización anticipada en la config (REF-04): es el único obligatorio sólo
// para los comandos que lo usan (amortize), no para guardar ni para revision
// y schedule.
const FieldEarlyRepaymentFeePct = "early_repayment_fee_pct"

// Error implementa error.
func (e *IncompleteError) Error() string {
	return fmt.Sprintf("el préstamo no está completo: falta %s", e.Field)
}

// Sync y Loan son los tipos del esquema de la config ([loans.<id>], REF-02):
// la forma del fichero vive en internal/config, que es quien lo escribe y lo
// carga, y aquí llegan vía alias para que el resto del programa —y sobre todo
// las reglas de validez que aplica este paquete— siga hablando de loan.Loan.
// Con el alias, estas funciones son las válidas del tipo del contrato.
type (
	Loan = config.Loan
	Sync = config.Sync
)

// MissingFields devuelve los nombres de todos los campos obligatorios que el
// préstamo no declara, en el orden del modelo. Es lo que `loan show` lista
// para avisar (arranque §4.1) y lo que Validate usa para el centinela.
// Sólo mira presencia: los valores presentes se validan en ValidateValues.
func MissingFields(l Loan) []string {
	var missing []string
	add := func(campo string, vacio bool) {
		if vacio {
			missing = append(missing, campo)
		}
	}
	add("name", l.Name == "")
	add("type", l.Type == "")
	add("start_date", l.StartDate == "")
	add("original_principal", l.OriginalPrincipal == "")
	add("term_months", l.TermMonths == 0)
	add("payment_day", l.PaymentDay == 0)
	add("initial_rate", l.InitialRate == "")
	add("initial_period_months", l.InitialPeriodMonths == nil)
	add("index", l.Index == "")
	add("spread", l.Spread == "")
	add("revision_every_months", l.RevisionEveryMonths == 0)
	add("reference_offset_months", l.ReferenceOffsetMonths == nil)
	add("floor_rate", l.FloorRate == nil)
	add("rate_rounding", l.RateRounding == "")
	add("interest_convention", l.InterestConvention == "")
	add("payment_rounding", l.PaymentRounding == "")
	add("early_repayment_fee_pct", l.EarlyRepaymentFeePct == "")
	if l.Sync != nil {
		add("sync.date", l.Sync.Date == "")
		add("sync.outstanding_principal", l.Sync.OutstandingPrincipal == "")
		add("sync.current_rate", l.Sync.CurrentRate == "")
	}
	return missing
}

// Validate aplica la validación estricta completa: primero la presencia de
// los campos obligatorios — el primer campo ausente devuelve el centinela
// *IncompleteError nombrándolo— y después los valores de los presentes. Un
// valor presente pero imposible es un error de uso del contrato (código 2),
// nunca un error interno.
func Validate(l Loan) error {
	if missing := MissingFields(l); len(missing) > 0 {
		return &IncompleteError{Field: missing[0]}
	}
	return ValidateValues(l)
}

// ValidateWithoutFee valida el préstamo para los comandos que NO usan la
// comisión de amortización — revision y schedule (REF-04, S9)—: igual que
// Validate, pero early_repayment_fee_pct puede faltar. La regla «nada se
// supone» no se relaja (S11): el campo no se rellena con ningún valor por
// defecto, simplemente no se exige donde no hace falta; amortize sigue
// validando con Validate, que sí la exige (S10).
func ValidateWithoutFee(l Loan) error {
	if missing := MissingFields(l); len(missing) > 0 {
		// early_repayment_fee_pct es el único campo que ValidateWithoutFee
		// tolera ausente: si es el único que falta, hay cálculo.
		if len(missing) > 1 || missing[0] != FieldEarlyRepaymentFeePct {
			return &IncompleteError{Field: missing[0]}
		}
	}
	return ValidateValues(l)
}

// ValidateValues valida sólo los valores de los campos presentes: decimales
// bien formados, rangos y enumerados. No mira presencia, de modo que un
// préstamo a medias (que `loan add` permite guardar) puede validarse por
// partes a medida que el usuario completa datos.
func ValidateValues(l Loan) error {
	usage := func(format string, args ...any) error {
		return errs.New(errs.KindUsage, fmt.Sprintf(format, args...),
			"revisa el valor del campo en la config o con \"hipoteca loan edit\"")
	}
	switch l.Type {
	case TypeVariable, TypeMixta, TypeFija:
	default:
		return usage("el tipo %q no es correcto: usa variable, mixta o fija", l.Type)
	}
	if l.StartDate != "" {
		if _, err := time.Parse("2006-01-02", l.StartDate); err != nil {
			return usage("la fecha de inicio %q no es una fecha YYYY-MM-DD", l.StartDate)
		}
	}
	if l.OriginalPrincipal != "" {
		d, err := model.NewDecimal(l.OriginalPrincipal)
		if err != nil {
			return usage("el capital original %q no es un decimal válido", l.OriginalPrincipal)
		}
		if d.Decimal().Sign() <= 0 {
			return usage("el capital original %q debe ser mayor que cero", l.OriginalPrincipal)
		}
	}
	if l.TermMonths < 0 {
		return usage("el plazo %d no puede ser negativo", l.TermMonths)
	}
	if l.PaymentDay < 0 || l.PaymentDay > 31 {
		return usage("el día de pago %d está fuera de rango: usa un día del mes (1..31)", l.PaymentDay)
	}
	if l.InitialRate != "" {
		d, err := model.NewDecimal(l.InitialRate)
		if err != nil {
			return usage("el tipo inicial %q no es un decimal válido", l.InitialRate)
		}
		if d.Decimal().Sign() < 0 {
			return usage("el tipo inicial %q no puede ser negativo", l.InitialRate)
		}
	}
	if l.InitialPeriodMonths != nil {
		if *l.InitialPeriodMonths < 0 {
			return usage("el tramo inicial %d no puede ser negativo", *l.InitialPeriodMonths)
		}
		if l.TermMonths > 0 && *l.InitialPeriodMonths > l.TermMonths {
			return usage("el tramo inicial (%d meses) no puede durar más que el plazo (%d meses)",
				*l.InitialPeriodMonths, l.TermMonths)
		}
	}
	if l.Spread != "" {
		if _, err := model.NewDecimal(l.Spread); err != nil {
			return usage("el diferencial %q no es un decimal válido", l.Spread)
		}
	}
	if l.SpreadUnbonified != "" {
		if _, err := model.NewDecimal(l.SpreadUnbonified); err != nil {
			return usage("el diferencial sin bonificaciones %q no es un decimal válido", l.SpreadUnbonified)
		}
	}
	if l.RevisionEveryMonths < 0 {
		return usage("la frecuencia de revisión %d no puede ser negativa", l.RevisionEveryMonths)
	}
	if l.ReferenceOffsetMonths != nil && *l.ReferenceOffsetMonths < 0 {
		return usage("el desplazamiento del mes de referencia %d no puede ser negativo", *l.ReferenceOffsetMonths)
	}
	if l.FloorRate != nil && *l.FloorRate != "" {
		d, err := model.NewDecimal(*l.FloorRate)
		if err != nil {
			return usage("el suelo %q no es un decimal válido", *l.FloorRate)
		}
		if d.Decimal().Sign() < 0 {
			return usage("el suelo %q no puede ser negativo", *l.FloorRate)
		}
	}
	switch finance.RateRounding(l.RateRounding) {
	case finance.RateRoundingNone, finance.RateRoundingEighth,
		finance.RateRoundingQuarter, finance.RateRoundingFiveHundredths:
	default:
		if l.RateRounding != "" {
			return usage("el redondeo de escritura %q no está soportado: usa none, 1/8, 1/4 o 0.05", l.RateRounding)
		}
	}
	if l.InterestConvention != "" && l.InterestConvention != InterestConventionMonthlyNominal {
		return usage("la convención de interés %q no está soportada: usa %s",
			l.InterestConvention, InterestConventionMonthlyNominal)
	}
	if l.PaymentRounding != "" && l.PaymentRounding != PaymentRoundingHalfUpCents {
		return usage("el redondeo de cuota %q no está soportado: usa %s",
			l.PaymentRounding, PaymentRoundingHalfUpCents)
	}
	if l.EarlyRepaymentFeePct != "" {
		d, err := model.NewDecimal(l.EarlyRepaymentFeePct)
		if err != nil {
			return usage("la comisión de amortización anticipada %q no es un decimal válido", l.EarlyRepaymentFeePct)
		}
		if d.Decimal().Sign() < 0 {
			return usage("la comisión de amortización anticipada %q no puede ser negativa", l.EarlyRepaymentFeePct)
		}
	}
	if l.Sync != nil {
		if l.Sync.Date != "" {
			if _, err := time.Parse("2006-01-02", l.Sync.Date); err != nil {
				return usage("la fecha del recibo %q no es una fecha YYYY-MM-DD", l.Sync.Date)
			}
		}
		if l.Sync.OutstandingPrincipal != "" {
			d, err := model.NewDecimal(l.Sync.OutstandingPrincipal)
			if err != nil {
				return usage("el capital pendiente del recibo %q no es un decimal válido", l.Sync.OutstandingPrincipal)
			}
			if d.Decimal().Sign() <= 0 {
				return usage("el capital pendiente del recibo %q debe ser mayor que cero", l.Sync.OutstandingPrincipal)
			}
		}
		if l.Sync.CurrentRate != "" {
			d, err := model.NewDecimal(l.Sync.CurrentRate)
			if err != nil {
				return usage("el tipo vigente del recibo %q no es un decimal válido", l.Sync.CurrentRate)
			}
			if d.Decimal().Sign() < 0 {
				return usage("el tipo vigente del recibo %q no puede ser negativo", l.Sync.CurrentRate)
			}
		}
	}
	return nil
}
