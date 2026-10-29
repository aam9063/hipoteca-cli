package finance

import (
	"errors"
	"fmt"
	"math/big"

	"github.com/shopspring/decimal"
)

// Errores centinela de la amortización anticipada (FIN-03.4). internal/errs
// los mapeará a usage/internal en CORE-01.
var (
	// ErrUnknownPrepayMode: el modo no es ni reduce-payment ni reduce-term.
	ErrUnknownPrepayMode = errors.New("finance: modo de amortización anticipada desconocido")
	// ErrNegativePrepayAmount: el importe a amortizar es negativo.
	ErrNegativePrepayAmount = errors.New("finance: el importe de la amortización no puede ser negativo")
	// ErrNegativeFeePct: el porcentaje de comisión es negativo.
	ErrNegativeFeePct = errors.New("finance: el porcentaje de comisión no puede ser negativo")
	// ErrAfterMonthOutsideSchedule: el mes del evento no es una mensualidad
	// del cuadro (0, negativo, o posterior al cierre real del préstamo).
	ErrAfterMonthOutsideSchedule = errors.New("finance: el mes de la amortización está fuera del cuadro")
	// ErrAmountExceedsBalance: el importe supera el capital pendiente en ese
	// mes. Amortizar exactamente el saldo sí es válido y cierra el préstamo.
	ErrAmountExceedsBalance = errors.New("finance: el importe supera el capital pendiente en ese mes")
)

// Mode es el modo de aplicar una amortización anticipada.
type Mode string

const (
	// ReducePayment conserva el plazo restante y recalcula la cuota con el
	// saldo nuevo (decisión 2).
	ReducePayment Mode = "reduce-payment"
	// ReduceTerm conserva la cuota y el plazo es el número de meses que la
	// cierran; la última cuota absorbe el residuo (decisión 3).
	ReduceTerm Mode = "reduce-term"
)

// EarlyRepayment son los datos del evento de amortización anticipada.
// AfterMonth es el índice de la cuota tras la que se amortiza (1-based, el
// evento ocurre justo después de cobrar esa cuota).
type EarlyRepayment struct {
	Amount     decimal.Decimal
	FeePct     decimal.Decimal // comisión en tanto por ciento, p. ej. "0.50"
	AfterMonth int
}

// Prepayment es el resultado de una amortización anticipada. Expone todas las
// cifras antes y después del evento para que la salida pueda mostrar el porqué
// de cada una (P-04). Los meses son siempre sobre el préstamo completo
// (TermBefore y TermAfter cuentan desde el mes 1), igual que el JSON de los
// casos de referencia.
type Prepayment struct {
	// Entradas aplicadas y comisión. Outlay es el desembolso total del
	// usuario: Amount + Fee. La comisión se paga aparte y NO reduce el
	// capital: el saldo baja exactamente por Amount (decisión 1).
	Mode       Mode
	Amount     decimal.Decimal
	FeePct     decimal.Decimal
	Fee        decimal.Decimal
	Outlay     decimal.Decimal
	AfterMonth int

	// Saldo justo tras la cuota AfterMonth y tras aplicar la amortización.
	BalanceBefore decimal.Decimal
	BalanceAfter  decimal.Decimal

	// Cuota y plazo antes y después. TermBefore es el plazo real del cuadro
	// sin amortizar (filas del núcleo) y TermAfter el real con el evento;
	// ambos cuentan meses totales desde el origen del préstamo.
	PaymentBefore decimal.Decimal
	PaymentAfter  decimal.Decimal
	TermBefore    int
	TermAfter     int

	// Intereses: los del préstamo sin amortizar, los del cuadro con el
	// evento, y el ahorro entre ambos medido contra el mismo préstamo
	// (decisión 4). InterestSaved conserva el signo: es ≥ 0 con un TIN no
	// negativo y ≤ 0 con un TIN negativo, donde amortizar renuncia al
	// subsidio que el saldo pendiente devengaba a favor del prestatario
	// (decisión 5). MonthsSaved es TermBefore − TermAfter: 0 en
	// reduce-payment salvo cierre anticipado por excedente de redondeo.
	InterestBefore decimal.Decimal
	InterestAfter  decimal.Decimal
	InterestSaved  decimal.Decimal
	MonthsSaved    int

	// InterestBeforeWindow es el interés que el cuadro intacto ganaría en la
	// misma ventana que recorre el cuadro nuevo (meses AfterMonth+1..TermAfter):
	// con TIN negativo es el subsidio en juego durante esa ventana, lo que
	// hace interpretable el signo del ahorro. 0.00 si el evento cierra el
	// préstamo.
	InterestBeforeWindow decimal.Decimal

	// Schedule es el cuadro completo con el evento aplicado: las filas
	// 1..AfterMonth son las del préstamo original y las siguientes las del
	// tramo recalculado, renumeradas de forma continua hasta el cierre en
	// exactamente 0.00. Excepción M1 (importe que agota el saldo): el cuadro
	// cierra POR EL PROPIO EVENTO — la última fila es la del mes del evento y
	// lleva el capital anterior al evento, BalanceAfter queda en 0.00 exacto,
	// TermAfter es el mes del evento, y Schedule.TotalPaid NO incluye el
	// desembolso (importe + comisión). La capa de comando (CMD-02) debe
	// presentar el evento explícitamente en ese caso.
	Schedule Schedule
}

// Prepay calcula el efecto de amortizar anticipadamente rep.Amount tras la
// cuota rep.AfterMonth del préstamo loan, en el modo dado. Reutiliza el núcleo
// sin recalcular nada por su cuenta: el cuadro previo y el tramo posterior de
// reduce-payment salen de Amortization (cuota incluida, convención 1.5 de
// docs/calculos.md), y la comisión y el interés mensual usan el mismo
// redondeo half-up a céntimos del resto del paquete.
//
// La comisión es round_half_up(Amount · FeePct / 100); el desembolso del
// usuario es Amount + Fee y el saldo baja exactamente por Amount (decisión 1).
//
// M1 (importe que agota el saldo): el préstamo cierra por el propio evento.
// El Schedule devuelto termina en la fila del mes del evento, que lleva el
// capital anterior al evento; el cierre en 0.00 exacto queda expresado por
// BalanceAfter y TermAfter (= rep.AfterMonth), y Schedule.TotalPaid no incluye
// el desembolso: CMD-02 debe renderizar el evento como línea propia.
//
// En ReduceTerm se mantiene la cuota y el plazo es el número de meses que la
// cierran; el interés mensual se redondea igual que en Amortization y la
// última cuota absorbe el residuo. Como la cuota amortizaba un saldo mayor con
// el mismo tipo, el saldo menor cierra en el mismo número de meses o menos,
// así que el bucle termina siempre dentro del plazo original.
func Prepay(loan Loan, rep EarlyRepayment, mode Mode) (Prepayment, error) {
	switch mode {
	case ReducePayment, ReduceTerm:
	default:
		return Prepayment{}, ErrUnknownPrepayMode
	}
	if rep.Amount.Sign() < 0 {
		return Prepayment{}, ErrNegativePrepayAmount
	}
	if rep.FeePct.Sign() < 0 {
		return Prepayment{}, ErrNegativeFeePct
	}

	baseline, err := Amortization(loan)
	if err != nil {
		return Prepayment{}, err
	}
	if rep.AfterMonth < 1 || rep.AfterMonth > len(baseline.Rows) {
		return Prepayment{}, fmt.Errorf("%w: mes %d, cuadro de %d filas",
			ErrAfterMonthOutsideSchedule, rep.AfterMonth, len(baseline.Rows))
	}

	eventRow := baseline.Rows[rep.AfterMonth-1]
	balanceBefore := eventRow.Balance
	if rep.Amount.GreaterThan(balanceBefore) {
		return Prepayment{}, fmt.Errorf("%w: importe %s, capital pendiente %s en el mes %d",
			ErrAmountExceedsBalance, rep.Amount.String(), balanceBefore.String(), rep.AfterMonth)
	}

	fee := centsIntToDecimal(roundHalfUpCents(new(big.Rat).Quo(
		new(big.Rat).Mul(rep.Amount.Rat(), rep.FeePct.Rat()), ratHundred)))
	balanceAfter := balanceBefore.Sub(rep.Amount)

	pre := Prepayment{
		Mode:           mode,
		Amount:         rep.Amount,
		FeePct:         rep.FeePct,
		Fee:            fee,
		Outlay:         rep.Amount.Add(fee),
		AfterMonth:     rep.AfterMonth,
		BalanceBefore:  balanceBefore,
		BalanceAfter:   balanceAfter,
		PaymentBefore:  eventRow.Payment,
		TermBefore:     len(baseline.Rows),
		InterestBefore: baseline.TotalInterest,
		// Las filas hasta el evento son las del préstamo original, sin tocar.
		Schedule: Schedule{Rows: append([]Row(nil), baseline.Rows[:rep.AfterMonth]...)},
	}

	var newRows []Row
	switch mode {
	case ReducePayment:
		// Decisión 2: mismo plazo restante, cuota recalculada con el saldo
		// nuevo por el propio núcleo.
		if balanceAfter.IsZero() {
			pre.PaymentAfter = decimal.Zero
			pre.TermAfter = rep.AfterMonth
		} else {
			after, err := Amortization(Loan{
				Principal:  balanceAfter,
				AnnualRate: loan.AnnualRate,
				Months:     len(baseline.Rows) - rep.AfterMonth,
			})
			if err != nil {
				return Prepayment{}, err
			}
			newRows = renumberRows(after.Rows, rep.AfterMonth)
			pre.PaymentAfter = after.Rows[0].Payment
			pre.TermAfter = rep.AfterMonth + len(after.Rows)
		}
	case ReduceTerm:
		// Decisión 3: misma cuota, el plazo es el que la cierra.
		pre.PaymentAfter = eventRow.Payment
		if balanceAfter.IsZero() {
			pre.TermAfter = rep.AfterMonth
		} else {
			rows, err := closeWithFixedPayment(balanceAfter, loan.AnnualRate, eventRow.Payment,
				len(baseline.Rows)-rep.AfterMonth)
			if err != nil {
				return Prepayment{}, err
			}
			newRows = renumberRows(rows, rep.AfterMonth)
			pre.TermAfter = rep.AfterMonth + len(rows)
		}
	}
	pre.Schedule.Rows = append(pre.Schedule.Rows, newRows...)

	var totalInterest, totalPaid decimal.Decimal
	totalInterest, totalPaid = decimal.Zero, decimal.Zero
	for _, row := range pre.Schedule.Rows {
		totalInterest = totalInterest.Add(row.Interest)
		totalPaid = totalPaid.Add(row.Payment)
	}
	pre.Schedule.TotalInterest = totalInterest
	pre.Schedule.TotalPaid = totalPaid
	pre.InterestAfter = totalInterest
	pre.InterestSaved = pre.InterestBefore.Sub(pre.InterestAfter)
	pre.MonthsSaved = pre.TermBefore - pre.TermAfter

	// Interés del cuadro intacto en la ventana que recorre el cuadro nuevo:
	// son filas del préstamo original (TermAfter ≤ TermBefore), así que se
	// leen directamente del cuadro base.
	if pre.TermAfter > rep.AfterMonth {
		window := decimal.Zero
		for _, row := range baseline.Rows[rep.AfterMonth:pre.TermAfter] {
			window = window.Add(row.Interest)
		}
		pre.InterestBeforeWindow = window
	}

	return pre, nil
}

// renumberRows desplaza los meses de las filas de un tramo para que sigan de
// forma continua a las del cuadro original.
func renumberRows(rows []Row, afterMonth int) []Row {
	out := make([]Row, len(rows))
	for k, row := range rows {
		row.Month = afterMonth + 1 + k
		out[k] = row
	}
	return out
}

// closeWithFixedPayment amortiza balance con la cuota fija payment usando
// exactamente la maquinaria del núcleo: interés mensual redondeado half-up a
// céntimos antes de amortizar (convención 1.4) y última cuota que absorbe el
// residuo cuando la cuota cubriría capital + interés (convención 1.3). Es el
// bucle de Amortization con cuota constante en lugar de calculada por
// Payment: el tramo de reduce-term no tiene un plazo de entrada, es lo que
// este bucle mide. Como payment amortizaba un saldo mayor con el mismo tipo,
// cada interés del mes es menor o igual y el cierre llega en el plazo dado o
// antes; el guard de maxMonths es defensa, no camino normal.
func closeWithFixedPayment(balance decimal.Decimal, annualRate decimal.Decimal, payment decimal.Decimal, maxMonths int) ([]Row, error) {
	i := monthlyRateRat(annualRate)
	rows := make([]Row, 0, maxMonths)
	for month := 1; balance.Sign() > 0; month++ {
		if month > maxMonths {
			return nil, fmt.Errorf("finance: interno: la cuota mantenida no cierra el préstamo en %d meses", maxMonths)
		}
		interest := centsIntToDecimal(roundHalfUpCents(new(big.Rat).Mul(balance.Rat(), i)))
		var pay, principalRepaid decimal.Decimal
		if scheduled := payment.Sub(interest); scheduled.GreaterThanOrEqual(balance) {
			// Última cuota: paga exactamente capital + interés y el saldo
			// queda en 0.00.
			pay = balance.Add(interest)
			principalRepaid = balance
		} else {
			pay = payment
			principalRepaid = scheduled
		}
		balance = balance.Sub(principalRepaid)
		rows = append(rows, Row{
			Month:     month,
			Payment:   pay,
			Interest:  interest,
			Principal: principalRepaid,
			Balance:   balance,
		})
	}
	return rows, nil
}
