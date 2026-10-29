package loan

import (
	"fmt"
	"time"

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

// Error implementa error.
func (e *IncompleteError) Error() string {
	return fmt.Sprintf("el préstamo no está completo: falta %s", e.Field)
}

// Sync es la resincronización con el recibo real del banco (opcional;
// docs/ARRANQUE.md §4.1). Cuando está presente, el capital pendiente y el
// tipo de partida son los del recibo, no los calculados (decisión 2 de
// LOAN-01), así que sus tres campos son obligatorios juntos.
type Sync struct {
	// Date es la fecha del recibo, YYYY-MM-DD.
	Date string `toml:"date" json:"date"`
	// OutstandingPrincipal es el capital pendiente según el recibo, en euros,
	// como decimal exacto ("152341.18").
	OutstandingPrincipal string `toml:"outstanding_principal" json:"outstanding_principal"`
	// CurrentRate es el TIN vigente según el recibo, en tanto por ciento
	// ("3.12").
	CurrentRate string `toml:"current_rate" json:"current_rate"`
}

// Loan es un préstamo guardado en la config local ([loans.<id>],
// docs/ARRANQUE.md §4.1). Los importes y los tipos son cadenas decimales
// exactas — el formato TOML del arranque conserva la escala que escribió el
// usuario ("180000.00" sigue siendo "180000.00")— y la validación las parsea
// con model.NewDecimal: nunca hay float64 y el valor que viaja a
// internal/finance es un shopspring decimal exacto.
//
// Los campos que dependen de la escritura son obligatorios o explícitos
// (decisión 1): ReferenceOffsetMonths y FloorRate van con puntero porque el
// cero y la cadena vacía son valores legítimos (offset 0 y "sin suelo"), de
// modo que su ausencia en el TOML (puntero nil) es lo que marca el préstamo
// como incompleto. FloorRate vacío no es un suelo inventado: es la ausencia
// de suelo declarada.
type Loan struct {
	// Name es el nombre con el que el usuario reconoce el préstamo.
	Name string `toml:"name" json:"name"`
	// Type es variable, mixta o fija (§4.1).
	Type string `toml:"type" json:"type"`
	// StartDate es la fecha de la primera cuota o de firma, según escritura,
	// YYYY-MM-DD.
	StartDate string `toml:"start_date" json:"start_date"`
	// OriginalPrincipal es el capital original en euros ("180000.00").
	OriginalPrincipal string `toml:"original_principal" json:"original_principal"`
	// TermMonths es el plazo total en meses.
	TermMonths int `toml:"term_months" json:"term_months"`
	// PaymentDay es el día del mes en que se paga la cuota (1..31).
	PaymentDay int `toml:"payment_day" json:"payment_day"`
	// InitialRate es el TIN anual % del periodo inicial ("1.25").
	InitialRate string `toml:"initial_rate" json:"initial_rate"`
	// InitialPeriodMonths son los meses a tipo fijo antes de la primera
	// revisión; 0 es un valor legítimo y por eso va con puntero.
	InitialPeriodMonths *int `toml:"initial_period_months" json:"initial_period_months"`
	// Index es el id del índice del catálogo ("euribor-1y").
	Index string `toml:"index" json:"index"`
	// Spread es el diferencial en puntos ("0.89").
	Spread string `toml:"spread" json:"spread"`
	// SpreadUnbonified es el diferencial sin bonificaciones; opcional.
	SpreadUnbonified string `toml:"spread_unbonified,omitempty" json:"spread_unbonified,omitempty"`
	// RevisionEveryMonths es la frecuencia de revisión en meses.
	RevisionEveryMonths int `toml:"revision_every_months" json:"revision_every_months"`
	// ReferenceOffsetMonths es qué mes de índice se usa: N meses antes del mes
	// de revisión, según escritura; 0 es legítimo y por eso va con puntero.
	ReferenceOffsetMonths *int `toml:"reference_offset_months" json:"reference_offset_months"`
	// FloorRate es el suelo del tipo ("2.00") o "" para sin suelo; su ausencia
	// (nil) deja el préstamo incompleto.
	FloorRate *string `toml:"floor_rate" json:"floor_rate"`
	// RateRounding es el redondeo de escritura del tipo: none, 1/8, 1/4 o 0.05.
	RateRounding string `toml:"rate_rounding" json:"rate_rounding"`
	// InterestConvention es la convención de interés; hoy sólo
	// monthly-nominal (TIN/12 por mes).
	InterestConvention string `toml:"interest_convention" json:"interest_convention"`
	// PaymentRounding es el redondeo de la cuota; hoy sólo half-up-cents.
	PaymentRounding string `toml:"payment_rounding" json:"payment_rounding"`
	// EarlyRepaymentFeePct es la comisión de amortización anticipada en
	// tanto por ciento del importe según escritura ("0" si no la hay).
	EarlyRepaymentFeePct string `toml:"early_repayment_fee_pct" json:"early_repayment_fee_pct"`
	// Sync es la última sincronización con el recibo; opcional.
	Sync *Sync `toml:"sync,omitempty" json:"sync,omitempty"`
}

// MissingFields devuelve los nombres de todos los campos obligatorios que el
// préstamo no declara, en el orden del modelo. Es lo que `loan show` lista
// para avisar (arranque §4.1) y lo que Validate usa para el centinela.
// Sólo mira presencia: los valores presentes se validan en ValidateValues.
func (l Loan) MissingFields() []string {
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
func (l Loan) Validate() error {
	if missing := l.MissingFields(); len(missing) > 0 {
		return &IncompleteError{Field: missing[0]}
	}
	return l.ValidateValues()
}

// ValidateValues valida sólo los valores de los campos presentes: decimales
// bien formados, rangos y enumerados. No mira presencia, de modo que un
// préstamo a medias (que `loan add` permite guardar) puede validarse por
// partes a medida que el usuario completa datos.
func (l Loan) ValidateValues() error {
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
