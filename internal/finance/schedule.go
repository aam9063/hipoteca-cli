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
		if month == loan.Months {
			// La última cuota absorbe el residuo: paga el capital pendiente
			// más el interés del mes, de modo que el capital final sea 0.00.
			pay = balance.Add(interest)
			principalRepaid = balance
		} else {
			pay = payment
			principalRepaid = pay.Sub(interest)
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
	}

	return Schedule{Rows: rows, TotalInterest: totalInterest, TotalPaid: totalPaid}, nil
}
