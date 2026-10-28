package finance

import (
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

// mixedPeriods construye los tramos de un caso mixto desde el JSON: tramo
// inicial más una revisión por cada entrada, con la duración deducida de los
// meses restantes de cada revisión (nunca a mano).
func mixedPeriods(t *testing.T, c refMixedCase) []Period {
	t.Helper()
	periods := []Period{{
		Rate:   mustDecimal(t, c.Inputs.InitialRate),
		Months: c.Inputs.InitialPeriodMonths,
	}}
	revs := c.Expected.Revisions
	for i, rev := range revs {
		months := rev.RemainingMonths
		if i+1 < len(revs) {
			months = rev.RemainingMonths - revs[i+1].RemainingMonths
		}
		periods = append(periods, Period{
			Rate:   mustDecimal(t, rev.TinAfterRounding),
			Months: months,
		})
	}
	return periods
}

// assertRefRow compara una fila del cuadro con la expectativa del JSON.
func assertRefRow(t *testing.T, label string, got Row, want refRow) {
	t.Helper()
	if got.Month != want.Month {
		t.Errorf("%s: mes = %d, se esperaba %d", label, got.Month, want.Month)
	}
	if !got.Payment.Equal(mustDecimal(t, want.Payment)) {
		t.Errorf("%s: cuota = %s, se esperaba %s", label, got.Payment.String(), want.Payment)
	}
	if !got.Interest.Equal(mustDecimal(t, want.Interest)) {
		t.Errorf("%s: interés = %s, se esperaba %s", label, got.Interest.String(), want.Interest)
	}
	if !got.Principal.Equal(mustDecimal(t, want.PrincipalRepaid)) {
		t.Errorf("%s: amortización = %s, se esperaba %s", label, got.Principal.String(), want.PrincipalRepaid)
	}
	if !got.Balance.Equal(mustDecimal(t, want.OutstandingAfter)) {
		t.Errorf("%s: capital pendiente = %s, se esperaba %s", label, got.Balance.String(), want.OutstandingAfter)
	}
}

// TestCompoundScheduleReferenceCases reproduce el cuadro completo de los
// casos 6, 7 y 8: cuotas de cada tramo, interés total, primera y última fila y
// cierre a exactamente 0.00, leyendo las expectativas del JSON.
func TestCompoundScheduleReferenceCases(t *testing.T) {
	for _, name := range mixedCaseFiles {
		c := loadMixedCase(t, name)
		t.Run(c.ID, func(t *testing.T) {
			loan := Loan{Principal: mustDecimal(t, c.Inputs.Principal), Months: c.Inputs.TermMonths}
			sched, err := CompoundSchedule(loan, mixedPeriods(t, c))
			if err != nil {
				t.Fatalf("CompoundSchedule(%s): %v", c.ID, err)
			}

			rows := sched.Rows
			if len(rows) == 0 {
				t.Fatal("el cuadro no tiene filas")
			}
			assertRefRow(t, "primera fila", rows[0], c.Expected.FirstRow)
			assertRefRow(t, "última fila", rows[len(rows)-1], c.Expected.LastRow)

			if !sched.TotalInterest.Equal(mustDecimal(t, c.Expected.TotalInterest)) {
				t.Errorf("interés total = %s, se esperaba %s",
					sched.TotalInterest.String(), c.Expected.TotalInterest)
			}
			if !sched.TotalPaid.Equal(mustDecimal(t, c.Expected.TotalPaid)) {
				t.Errorf("total pagado = %s, se esperaba %s",
					sched.TotalPaid.String(), c.Expected.TotalPaid)
			}

			sumPrincipal := decimal.Zero
			for _, row := range rows {
				sumPrincipal = sumPrincipal.Add(row.Principal)
			}
			if !sumPrincipal.Equal(mustDecimal(t, c.Expected.SumPrincipalRepaid)) {
				t.Errorf("suma de amortizaciones = %s, se esperaba %s",
					sumPrincipal.String(), c.Expected.SumPrincipalRepaid)
			}
			if !rows[len(rows)-1].Balance.Equal(mustDecimal(t, c.Expected.FinalOutstanding)) {
				t.Errorf("capital final = %s, se esperaba %s",
					rows[len(rows)-1].Balance.String(), c.Expected.FinalOutstanding)
			}

			// La cuota de cada revisión es la primera fila de su tramo.
			for _, rev := range c.Expected.Revisions {
				if rev.FirstMonth < 1 || rev.FirstMonth > len(rows) {
					t.Fatalf("revisión %d: first_month %d fuera del cuadro de %d filas",
						rev.RevisionNumber, rev.FirstMonth, len(rows))
				}
				row := rows[rev.FirstMonth-1]
				if !row.Payment.Equal(mustDecimal(t, rev.Payment)) {
					t.Errorf("revisión %d: cuota = %s, se esperaba %s",
						rev.RevisionNumber, row.Payment.String(), rev.Payment)
				}
			}

			// La primera fila de la primera revisión, con interés y capital.
			assertRefRow(t, "primera fila tras la revisión 1",
				rows[c.Expected.FirstRowAfterFirstRevision.Month-1],
				c.Expected.FirstRowAfterFirstRevision)

			// El mes de referencia de cada revisión sale de ReferenceMonth
			// con el desfase de la escritura, nunca de una tabla a mano.
			start, err := time.Parse("2006-01-02", c.Inputs.LoanStart)
			if err != nil {
				t.Fatalf("parsear loan_start: %v", err)
			}
			for _, rev := range c.Expected.Revisions {
				revMonth := time.Date(start.Year(), start.Month()+time.Month(rev.FirstMonth-1), 1, 0, 0, 0, 0, time.UTC)
				got := ReferenceMonth(revMonth, c.Inputs.ReferenceOffsetMonths).Format("2006-01")
				if got != rev.ReferenceMonth {
					t.Errorf("revisión %d: mes de referencia = %s, se esperaba %s",
						rev.RevisionNumber, got, rev.ReferenceMonth)
				}
			}
		})
	}
}

// TestCompoundSchedulePeriodCoverage: los tramos deben cubrir exactamente el
// plazo; por defecto o por exceso es error (decisión 5).
func TestCompoundSchedulePeriodCoverage(t *testing.T) {
	loan := Loan{Principal: mustDecimal(t, "150000"), Months: 96}
	rate := mustDecimal(t, "3")

	t.Run("los tramos no llegan al plazo", func(t *testing.T) {
		_, err := CompoundSchedule(loan, []Period{{Rate: rate, Months: 12}})
		if !errors.Is(err, ErrPeriodsDoNotCoverTerm) {
			t.Fatalf("se esperaba ErrPeriodsDoNotCoverTerm, se obtuvo %v", err)
		}
	})
	t.Run("los tramos se pasan del plazo", func(t *testing.T) {
		_, err := CompoundSchedule(loan, []Period{
			{Rate: rate, Months: 50}, {Rate: rate, Months: 50},
		})
		if !errors.Is(err, ErrPeriodsDoNotCoverTerm) {
			t.Fatalf("se esperaba ErrPeriodsDoNotCoverTerm, se obtuvo %v", err)
		}
	})
	t.Run("sin tramos", func(t *testing.T) {
		_, err := CompoundSchedule(loan, nil)
		if !errors.Is(err, ErrEmptyPeriods) {
			t.Fatalf("se esperaba ErrEmptyPeriods, se obtuvo %v", err)
		}
	})
	t.Run("tramo de cero meses", func(t *testing.T) {
		_, err := CompoundSchedule(loan, []Period{
			{Rate: rate, Months: 96}, {Rate: rate, Months: 0},
		})
		if !errors.Is(err, ErrNonPositivePeriodMonths) {
			t.Fatalf("se esperaba ErrNonPositivePeriodMonths, se obtuvo %v", err)
		}
	})
}
