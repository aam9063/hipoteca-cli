package finance

import (
	"math/big"

	"github.com/shopspring/decimal"
)

// Loan son los datos de entrada del cuadro de amortización.
type Loan struct {
	Principal  decimal.Decimal
	AnnualRate decimal.Decimal // tipo nominal anual en tanto por ciento, p. ej. "3.10"
	Months     int
}

// Row es una mensualidad del cuadro (Month es 1-based).
type Row struct {
	Month     int
	Payment   decimal.Decimal
	Interest  decimal.Decimal
	Principal decimal.Decimal
	Balance   decimal.Decimal
}

// Schedule es el cuadro completo con sus totales.
type Schedule struct {
	Rows          []Row
	TotalInterest decimal.Decimal
	TotalPaid     decimal.Decimal
}

// Amortization genera el cuadro completo del sistema francés con las
// convenciones de docs/calculos.md: interés mensual redondeado half-up a
// céntimos antes de amortizar (convención 1.4) y última cuota que absorbe el
// residuo para que el capital final sea exactamente 0.00 (convención 1.3).
//
// El cuadro devuelto puede tener menos filas que loan.Months: la cuota se
// redondea half-up a céntimos y puede quedar hasta medio céntimo por encima
// de la cuota exacta. Con combinaciones extremas de tipo y plazo (tipos de dos
// dígitos a cientos de meses) ese excedente acumulado liquidaría el préstamo
// antes del mes Months. En la mensualidad en la que la cuota redondeada
// superaría capital + interés, esa mensualidad pasa a ser la última: paga
// exactamente capital + interés, el capital queda en 0.00 y el bucle se
// detiene, de modo que nunca se emiten filas con capital o cuota negativos.
func Amortization(loan Loan) (Schedule, error) {
	payment, err := Payment(loan.Principal, loan.AnnualRate, loan.Months)
	if err != nil {
		return Schedule{}, err
	}

	i := monthlyRateRat(loan.AnnualRate)
	balance := loan.Principal
	totalInterest := decimal.Zero
	totalPaid := decimal.Zero
	rows := make([]Row, 0, loan.Months)

	for month := 1; month <= loan.Months; month++ {
		interest := centsIntToDecimal(roundHalfUpCents(new(big.Rat).Mul(balance.Rat(), i)))

		var pay, principalRepaid decimal.Decimal
		scheduledPrincipal := payment.Sub(interest)
		switch {
		case month == loan.Months || scheduledPrincipal.GreaterThanOrEqual(balance):
			// La última cuota absorbe el residuo: paga el capital pendiente
			// más el interés del mes, de modo que el capital final sea 0.00.
			// La misma rama cierra el préstamo antes de plazo si la cuota
			// redondeada superaría capital + interés (excedente de redondeo
			// half-up acumulado): esa mensualidad es la última real y no se
			// emiten filas fantasma después.
			pay = balance.Add(interest)
			principalRepaid = balance
		default:
			pay = payment
			principalRepaid = scheduledPrincipal
		}

		balance = balance.Sub(principalRepaid)
		rows = append(rows, Row{
			Month:     month,
			Payment:   pay,
			Interest:  interest,
			Principal: principalRepaid,
			Balance:   balance,
		})
		totalInterest = totalInterest.Add(interest)
		totalPaid = totalPaid.Add(pay)

		if balance.IsZero() {
			// El préstamo se ha liquidado (mes Months o cierre anticipado por
			// el excedente de redondeo): el cuadro termina aquí.
			break
		}
	}

	return Schedule{Rows: rows, TotalInterest: totalInterest, TotalPaid: totalPaid}, nil
}
