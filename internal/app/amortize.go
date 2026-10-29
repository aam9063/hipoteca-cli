package app

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aam9063/hipoteca-cli/internal/errs"
	"github.com/aam9063/hipoteca-cli/internal/finance"
	"github.com/aam9063/hipoteca-cli/internal/model"
)

// AmortizeInputs son las entradas de `amortize`, devueltas en la salida tal
// cual: la respuesta incluye sus entradas. Mode es "reduce-payment",
// "reduce-term" o "compare"; AfterMonth es el mes del evento (índice de la
// cuota, 1-based, decisión 3 de CMD-02) y FeePct la comisión en tanto por
// ciento (0 en esta versión: los préstamos guardados son LOAN-01 y la CLI no
// expone todavía una bandera de comisión).
type AmortizeInputs struct {
	// Loan es el id del préstamo guardado del que salieron capital, tipo,
	// plazo y comisión; vacío en el camino de flags sueltas (LOAN-01).
	Loan          string        `json:"loan,omitempty"`
	Principal     model.Decimal `json:"principal"`
	AnnualRatePct model.Decimal `json:"annual_rate_pct"`
	TermMonths    int           `json:"term_months"`
	Amount        model.Decimal `json:"amount"`
	AfterMonth    int           `json:"after_month"`
	Mode          string        `json:"mode"`
	FeePct        model.Decimal `json:"fee_pct"`
}

// AmortizeOutcome son las cifras de UNA opción de amortización anticipada:
// comisión y desembolso, cuota y plazo antes y después, y el ahorro de
// intereses con su signo (decisión 4 de FIN-03). Nada aquí compara opciones
// ni recomienda (P-08): con `compare` la salida lleva los dos bloques en
// paralelo y decide el usuario.
type AmortizeOutcome struct {
	Mode          string        `json:"mode"`
	Amount        model.Decimal `json:"amount"`
	Fee           model.Decimal `json:"fee"`
	Outlay        model.Decimal `json:"outlay"`
	BalanceBefore model.Decimal `json:"balance_before_event"`
	BalanceAfter  model.Decimal `json:"balance_after_event"`

	PaymentBefore model.Decimal `json:"payment_before"`
	PaymentAfter  model.Decimal `json:"payment_after"`
	TermBefore    int           `json:"term_months_before"`
	TermAfter     int           `json:"term_months_after"`

	InterestBefore model.Decimal `json:"interest_before"`
	InterestAfter  model.Decimal `json:"interest_after"`
	InterestSaved  model.Decimal `json:"interest_saved"`
	MonthsSaved    int           `json:"months_saved"`

	// ClosesAtEvent marca el cierre M1 (FIN-03): el importe agotó el saldo y
	// el préstamo termina EN el evento. Lo que guarda PaymentAfter depende del
	// modo: en reduce-payment es 0.00 porque no queda plazo sobre el que
	// recalcular una cuota; en reduce-term es la cuota mantenida, que es la
	// del propio mes del evento y sí se cobró. En ambos casos el cuadro
	// termina en el mes del evento (TermAfter = AfterMonth): después del
	// evento no existe ninguna cuota y la salida no inventa una.
	ClosesAtEvent *EventClose `json:"closes_at_event,omitempty"`
}

// EventClose es el marcador explícito del cierre por el evento: el mes en el
// que la amortización agotó el saldo, con el importe y el saldo justo antes
// del evento (BalanceAfter del bloque queda en 0.00).
type EventClose struct {
	AfterMonth    int           `json:"after_month"`
	Amount        model.Decimal `json:"amount"`
	BalanceBefore model.Decimal `json:"balance_before_event"`
}

// AmortizeResult es la salida de `amortize`: las entradas, el bloque (o los
// dos, en compare) de resultado y la atribución de datos procesados (P-05).
// En modo reduce-payment sólo se rellena reduce_payment; en reduce-term, sólo
// reduce_term; en compare, los dos. Nunca hay un tercer bloque ni una
// jerarquía de ganador: dos opciones al mismo nivel.
type AmortizeResult struct {
	SchemaVersion int               `json:"schema_version"`
	Inputs        AmortizeInputs    `json:"inputs"`
	ReducePayment *AmortizeOutcome  `json:"reduce_payment,omitempty"`
	ReduceTerm    *AmortizeOutcome  `json:"reduce_term,omitempty"`
	Attribution   model.Attribution `json:"attribution"`
}

// Amortize resuelve la amortización anticipada del cálculo suelto (CMD-02)
// con internal/finance.Prepay, sin préstamo guardado y sin red. El evento es
// el mes AfterMonth del cuadro desde el inicio del cálculo (decisión 3). No
// devuelve ningún consejo: sólo cifras, las dos opciones en compare.
func Amortize(in AmortizeInputs, now time.Time) (AmortizeResult, error) {
	switch in.Mode {
	case "reduce-payment", "reduce-term", "compare":
	default:
		return AmortizeResult{}, errs.New(errs.KindUsage,
			fmt.Sprintf("el modo %q no es correcto", in.Mode),
			"usa reduce-payment, reduce-term o compare")
	}

	modes := []string{in.Mode}
	if in.Mode == "compare" {
		// Decisión 1: las dos opciones con sus cifras, al mismo nivel.
		modes = []string{string(finance.ReducePayment), string(finance.ReduceTerm)}
	}
	outcomes := make(map[string]AmortizeOutcome, len(modes))
	for _, mode := range modes {
		out, err := prepay(in, finance.Mode(mode))
		if err != nil {
			return AmortizeResult{}, amortizeUsageError(err)
		}
		outcomes[mode] = out
	}

	attr, err := model.ProcessedAttribution(now.UTC().Format("2006-01-02"))
	if err != nil {
		// La fecha sale de time.UTC().Format: nunca ocurre.
		return AmortizeResult{}, errs.New(errs.KindInternal, err.Error(), "")
	}
	res := AmortizeResult{
		SchemaVersion: model.SchemaVersion,
		Inputs:        in,
		Attribution:   attr,
	}
	if out, ok := outcomes[string(finance.ReducePayment)]; ok {
		out.Mode = string(finance.ReducePayment)
		res.ReducePayment = &out
	}
	if out, ok := outcomes[string(finance.ReduceTerm)]; ok {
		out.Mode = string(finance.ReduceTerm)
		res.ReduceTerm = &out
	}
	return res, nil
}

// prepay llama a finance.Prepay y traduce el resultado al bloque de salida,
// incluido el marcador de cierre por el evento (M1).
func prepay(in AmortizeInputs, mode finance.Mode) (AmortizeOutcome, error) {
	pre, err := finance.Prepay(finance.Loan{
		Principal:  in.Principal.Decimal(),
		AnnualRate: in.AnnualRatePct.Decimal(),
		Months:     in.TermMonths,
	}, finance.EarlyRepayment{
		Amount:     in.Amount.Decimal(),
		FeePct:     in.FeePct.Decimal(),
		AfterMonth: in.AfterMonth,
	}, mode)
	if err != nil {
		return AmortizeOutcome{}, err
	}
	out := AmortizeOutcome{
		Mode:           string(mode),
		Amount:         model.NewMoney(pre.Amount),
		Fee:            model.NewMoney(pre.Fee),
		Outlay:         model.NewMoney(pre.Outlay),
		BalanceBefore:  model.NewMoney(pre.BalanceBefore),
		BalanceAfter:   model.NewMoney(pre.BalanceAfter),
		PaymentBefore:  model.NewMoney(pre.PaymentBefore),
		PaymentAfter:   model.NewMoney(pre.PaymentAfter),
		TermBefore:     pre.TermBefore,
		TermAfter:      pre.TermAfter,
		InterestBefore: model.NewMoney(pre.InterestBefore),
		InterestAfter:  model.NewMoney(pre.InterestAfter),
		InterestSaved:  model.NewMoney(pre.InterestSaved),
		MonthsSaved:    pre.MonthsSaved,
	}
	// M1 (FIN-03): el cuadro cierra por el evento. La salida lo marca de forma
	// explícita con el mes del evento en lugar de dejar un pago final que no
	// existe: BalanceAfter 0.00, PaymentAfter 0.00 y TermAfter = AfterMonth ya
	// vienen así de finance.
	if pre.BalanceAfter.IsZero() {
		out.ClosesAtEvent = &EventClose{
			AfterMonth:    pre.AfterMonth,
			Amount:        model.NewMoney(pre.Amount),
			BalanceBefore: model.NewMoney(pre.BalanceBefore),
		}
	}
	return out, nil
}

// sinPrefijoFinance quita el prefijo "finance: " de los mensajes de los
// centinelas del paquete interno: el usuario lee el hecho y el hint, no el
// nombre de nuestro paquete (verificador, nit 3).
func sinPrefijoFinance(msg string) string {
	return strings.ReplaceAll(msg, "finance: ", "")
}

// amortizeUsageError traduce las validaciones de finance a errores de uso del
// contrato: entradas inválidas del usuario son usage/2, no internal/1.
func amortizeUsageError(err error) error {
	var msg string
	switch {
	case errors.Is(err, finance.ErrUnknownPrepayMode):
		msg = "el modo de amortización no es correcto"
	case errors.Is(err, finance.ErrNegativePrepayAmount):
		msg = "el importe de la amortización no puede ser negativo"
	case errors.Is(err, finance.ErrNegativeFeePct):
		msg = "la comisión no puede ser negativa"
	case errors.Is(err, finance.ErrAfterMonthOutsideSchedule):
		msg = sinPrefijoFinance(err.Error())
	case errors.Is(err, finance.ErrAmountExceedsBalance):
		msg = sinPrefijoFinance(err.Error())
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
	return errs.New(errs.KindUsage, msg, "revisa --principal, --rate, --months, --amount y --date")
}
