package finance

import (
	"errors"
	"fmt"

	"github.com/shopspring/decimal"
)

// Errores de validación de los tramos de una hipoteca mixta (FIN-02, decisión 5).
var (
	ErrEmptyPeriods            = errors.New("finance: la hipoteca mixta necesita al menos un tramo")
	ErrNonPositivePeriodMonths = errors.New("finance: cada tramo debe durar al menos un mes")
	// ErrPeriodsDoNotCoverTerm: la suma de los tramos no cubre exactamente el
	// plazo del préstamo, ni por defecto ni por exceso.
	ErrPeriodsDoNotCoverTerm = errors.New("finance: los tramos no cubren exactamente el plazo del préstamo")
)

// Period es un tramo de una hipoteca mixta: su TIN (en tanto por ciento, con
// suelo y redondeo de escritura ya aplicados vía NewRate) y los meses que
// dura. Loan.AnnualRate no participa en CompoundSchedule: el tipo de cada
// tramo va aquí.
type Period struct {
	Rate   decimal.Decimal
	Months int
}

// CompoundSchedule genera el cuadro de una hipoteca mixta como una lista de
// tramos. No duplica las matemáticas del sistema francés: para cada tramo
// llama a Amortization con el capital pendiente al empezar el tramo, el TIN
// del tramo y los meses que quedan hasta el fin del plazo, y toma sus primeras
// period.Months filas. Así la cuota de cada tramo es la del núcleo
// (Payment con el capital pendiente y los meses restantes, convención 1.5.4
// de docs/calculos.md) y el interés, la amortización y el cierre del cuadro
// son exactamente los del núcleo, sin recalcular nada aquí. Los meses se
// renumeran de 1 a lo largo de todo el cuadro; el interés total y el pagado
// se acumulan fila a fila.
//
// La suma de los Months de los tramos debe ser exactamente loan.Months; si no
// es así devuelve ErrPeriodsDoNotCoverTerm. Los tipos negativos están soportados:
// un interés mensual negativo amortiza de más y el cuadro se cierra antes, igual
// que en Amortization (es el caso de una hipoteca sin suelo con Euríbor negativo).
func CompoundSchedule(loan Loan, periods []Period) (Schedule, error) {
	if len(periods) == 0 {
		return Schedule{}, ErrEmptyPeriods
	}
	total := 0
	for _, period := range periods {
		if period.Months <= 0 {
			return Schedule{}, fmt.Errorf("%w: tramo con %d meses", ErrNonPositivePeriodMonths, period.Months)
		}
		total += period.Months
	}
	if total != loan.Months {
		return Schedule{}, fmt.Errorf("%w: tramos suman %d, plazo %d",
			ErrPeriodsDoNotCoverTerm, total, loan.Months)
	}

	rows := make([]Row, 0, loan.Months)
	balance := loan.Principal
	totalInterest := decimal.Zero
	totalPaid := decimal.Zero
	month := 0

	for _, period := range periods {
		remainingToTerm := loan.Months - month
		segment, err := Amortization(Loan{
			Principal:  balance,
			AnnualRate: period.Rate,
			Months:     remainingToTerm,
		})
		if err != nil {
			return Schedule{}, err
		}

		// Del cuadro que el núcleo calcula hasta el fin del plazo sólo
		// pertenecen a este tramo sus primeras period.Months filas; la
		// siguiente revisión recalcula la cuota con el capital que queda.
		taken := 0
		for _, row := range segment.Rows {
			month++
			taken++
			rows = append(rows, Row{
				Month:     month,
				Payment:   row.Payment,
				Interest:  row.Interest,
				Principal: row.Principal,
				Balance:   row.Balance,
			})
			totalInterest = totalInterest.Add(row.Interest)
			totalPaid = totalPaid.Add(row.Payment)
			balance = row.Balance
			if taken == period.Months || balance.IsZero() {
				break
			}
		}
		if balance.IsZero() {
			// El préstamo se ha liquidado antes de agotar los tramos (cierre
			// anticipado por el excedente de redondeo, igual que en el cuadro
			// simple): el cuadro termina aquí.
			break
		}
	}

	return Schedule{Rows: rows, TotalInterest: totalInterest, TotalPaid: totalPaid}, nil
}
